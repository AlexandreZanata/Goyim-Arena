package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var jobsSchemaInstant = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

// TestJobsSchemaVocabularyAndCoherence proves app.jobs refuses an incoherent
// row instead of storing one: the workload and lifecycle vocabularies are
// closed, the payload is a bounded object, the attempt budget is coherent,
// and a lease exists exactly in the leased state.
func TestJobsSchemaVocabularyAndCoherence(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The whole governed workload vocabulary is accepted.
	for _, jobType := range []string{
		"email_delivery", "ink_grant_monthly", "pass_expiry",
		"retention_run", "session_cleanup", "billing_reconciliation",
	} {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO app.jobs (type, version, parameters, available_at)
			VALUES ($1, 1, '{"a":1}'::jsonb, $2)`, jobType, jobsSchemaInstant)
		if err != nil {
			t.Fatalf("type %s should be accepted: %v", jobType, err)
		}
	}

	// Unknown workload and lifecycle values are refused.
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at)
		VALUES ('send_email', 1, '{"a":1}'::jsonb, $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'pending', $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	// The version starts at one.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at)
		VALUES ('email_delivery', 0, '{"a":1}'::jsonb, $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	// The payload must be a JSON object; a non-object and an oversized
	// document are refused, and malformed JSON never reaches the column.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at)
		VALUES ('email_delivery', 1, '[1,2]'::jsonb, $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at)
		VALUES ('email_delivery', 1, jsonb_build_object('pad', repeat('x', 4096)), $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at)
		VALUES ('email_delivery', 1, 'not json'::jsonb, $1)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "22P02")

	// The attempt budget is coherent.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, attempts, max_attempts)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, -1, 5)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, attempts, max_attempts)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, 6, 5)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, max_attempts)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, 0)`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	// The lease exists exactly in the leased state.
	leaseCases := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "leased without lease",
			sql: `
				INSERT INTO app.jobs (type, version, parameters, state, available_at)
				VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'leased', $1)`,
			args: []any{jobsSchemaInstant},
		},
		{
			name: "queued with lease",
			sql: `
				INSERT INTO app.jobs (type, version, parameters, state, available_at, lease_owner, leased_until)
				VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'queued', $1, 'worker-1', $2)`,
			args: []any{jobsSchemaInstant, jobsSchemaInstant.Add(time.Minute)},
		},
		{
			name: "terminal with lease",
			sql: `
				INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, lease_owner, leased_until)
				VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'succeeded', $1, 1, 'worker-1', $2)`,
			args: []any{jobsSchemaInstant, jobsSchemaInstant.Add(time.Minute)},
		},
		{
			name: "blank lease owner",
			sql: `
				INSERT INTO app.jobs (type, version, parameters, state, available_at, lease_owner, leased_until)
				VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'leased', $1, '   ', $2)`,
			args: []any{jobsSchemaInstant, jobsSchemaInstant.Add(time.Minute)},
		},
	}
	for _, probe := range leaseCases {
		_, err := db.Pool.Exec(ctx, probe.sql, probe.args...)
		if err == nil {
			t.Errorf("%s: expected an error", probe.name)
			continue
		}
		assertPgErrorCode(t, err, "23514")
	}

	// A coherent leased row is accepted.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, lease_owner, leased_until)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'leased', $1, 1, 'worker-1', $2)`,
		jobsSchemaInstant, jobsSchemaInstant.Add(time.Minute))
	if err != nil {
		t.Fatalf("coherent leased row: %v", err)
	}
}

// TestJobsSchemaRedactedError proves the failure columns only ever hold a
// complete redacted pair: a stable JOB_* code plus a bounded, safe detail.
func TestJobsSchemaRedactedError(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, last_error_code)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'dead', $1, 5, 'JOB_HANDLER_ERROR')`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, last_error_detail)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'dead', $1, 5, 'detail only')`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, last_error_code, last_error_detail)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'dead', $1, 5, 'provider_error', 'x')`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, last_error_code, last_error_detail)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'dead', $1, 5, 'JOB_HANDLER_ERROR', repeat('x', 301))`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, state, available_at, attempts, last_error_code, last_error_detail)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, 'dead', $1, 5, 'JOB_HANDLER_ERROR', 'handler failed once')`, jobsSchemaInstant)
	if err != nil {
		t.Fatalf("coherent redacted failure: %v", err)
	}
}

// TestJobsSchemaIdempotencyAndProvenance proves the caller-chosen key is
// unique when set (while several NULL keys coexist) and that provenance is
// frozen after enqueue: a retry advances the lifecycle, never the identity.
func TestJobsSchemaIdempotencyAndProvenance(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Several jobs without a key coexist.
	for i := 0; i < 3; i++ {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO app.jobs (type, version, parameters, available_at)
			VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1)`, jobsSchemaInstant); err != nil {
			t.Fatalf("keyless insert %d: %v", i, err)
		}
	}

	var jobID pgtype.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, idempotency_key)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, 'email:a1:verify')
		RETURNING id`, jobsSchemaInstant).Scan(&jobID); err != nil {
		t.Fatalf("keyed insert: %v", err)
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, idempotency_key)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, 'email:a1:verify')`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23505")

	// A blank key is not a key.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO app.jobs (type, version, parameters, available_at, idempotency_key)
		VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1, '   ')`, jobsSchemaInstant)
	assertPgErrorCode(t, err, "23514")

	// Lifecycle columns may change...
	if _, err := db.Pool.Exec(ctx, `
		UPDATE app.jobs
		SET state = 'leased', attempts = 1, lease_owner = 'worker-1', leased_until = $2
		WHERE id = $1`, jobID, jobsSchemaInstant.Add(time.Minute)); err != nil {
		t.Fatalf("lifecycle update: %v", err)
	}

	// ...but provenance is frozen.
	for name, stmt := range map[string]string{
		"parameters":      `UPDATE app.jobs SET parameters = '{"a":2}'::jsonb WHERE id = $1`,
		"type":            `UPDATE app.jobs SET type = 'retention_run' WHERE id = $1`,
		"version":         `UPDATE app.jobs SET version = 2 WHERE id = $1`,
		"idempotency key": `UPDATE app.jobs SET idempotency_key = 'other' WHERE id = $1`,
		"max attempts":    `UPDATE app.jobs SET max_attempts = 9 WHERE id = $1`,
		"created at":      `UPDATE app.jobs SET created_at = now() WHERE id = $1`,
	} {
		_, err := db.Pool.Exec(ctx, stmt, jobID)
		if err == nil {
			t.Errorf("%s: expected an immutability error", name)
			continue
		}
		assertPgErrorCode(t, err, "23514")
	}
}

// TestJobsSchemaPrivileges proves the runtime may read, insert and update the
// queue but never delete from it: a terminal job is evidence of what the
// platform did, and only an audited retention policy may remove it.
func TestJobsSchemaPrivileges(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	checks := []struct {
		priv string
		want bool
	}{
		{"SELECT", true},
		{"INSERT", true},
		{"UPDATE", true},
		{"DELETE", false},
	}
	for _, check := range checks {
		var allowed bool
		if err := db.Pool.QueryRow(ctx,
			"SELECT has_table_privilege('arena_app', 'app.jobs', $1)", check.priv,
		).Scan(&allowed); err != nil {
			t.Fatalf("has_table_privilege(app.jobs, %s): %v", check.priv, err)
		}
		if allowed != check.want {
			t.Errorf("arena_app %s on app.jobs = %v, want %v", check.priv, allowed, check.want)
		}
	}

	withAppRole(t, db.Pool.Pool(), func(ctx context.Context, tx pgx.Tx) {
		var id pgtype.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO app.jobs (type, version, parameters, available_at)
			VALUES ('email_delivery', 1, '{"a":1}'::jsonb, $1)
			RETURNING id`, jobsSchemaInstant).Scan(&id); err != nil {
			t.Fatalf("runtime insert: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE app.jobs SET state = 'succeeded' WHERE id = $1`, id); err != nil {
			t.Fatalf("runtime update: %v", err)
		}
		_, err := tx.Exec(ctx, `DELETE FROM app.jobs WHERE id = $1`, id)
		assertPgErrorCode(t, err, "42501")
	})
}
