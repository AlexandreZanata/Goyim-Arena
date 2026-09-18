-- +goose Up
-- 00029 establishes the durable PostgreSQL job queue (P15-T01): app.jobs.
--
-- Invariants (REQ-JOB-01, docs/BACKEND.md, docs/ARCHITECTURE.md §jobs):
-- 1. Durable work lives in the database, not in memory or in a goroutine:
--    one row per unit of work, with the type, the schema version of its
--    payload, the payload itself, the earliest instant it may run
--    (available_at), the attempt budget and the lifecycle state.
-- 2. The job's parameters are minimal and self-describing: a bounded JSON
--    object carrying identifiers, references and arguments, never rendered
--    HTML, credentials, tokens or free prose. The CHECK rejects a
--    non-object, an oversized document and a malformed one, so an invalid
--    payload can never be stored as if it were runnable.
--    The column is named `parameters` and not `payload` on purpose: the
--    schema-wide privacy rule (billing_schema_test) reserves every `%payload%`
--    column for the digest and size of the verified provider body, so a raw
--    provider payload can never be persisted anywhere. The domain and
--    application still speak of the job payload; this is the same object.
-- 3. Last error is redacted by construction: only a stable machine code
--    (JOB_*) and a bounded detail that the adapter has already stripped of
--    secrets. There is no column that can hold an arbitrary stack trace or
--    provider message.
-- 4. Leasing is exclusive and recoverable: a leased row always names its
--    holder and the instant its lease ends; nothing else may carry a lease.
--    A lease that is not renewed expires, and the row becomes claimable
--    again, so a crashed worker never strands work.
-- 5. Idempotency is caller-chosen: a NULL key enqueues unconditionally
--    (several NULLs coexist), while a set key is unique, so a retried
--    enqueue resolves the original row instead of duplicating work.
-- 6. Provenance is frozen after enqueue: type, version, payload,
--    idempotency key, attempt budget and creation instant are immutable; a
--    retry mutates only lifecycle columns (state, lease, attempts, error,
--    availability, updated_at).
-- 7. Least privilege: arena_owner owns the objects; arena_app selects,
--    inserts and updates. DELETE is not granted — terminal rows are
--    evidence of what the platform did and are removed only by a future,
--    audited retention policy, never by the runtime.

CREATE TABLE IF NOT EXISTS app.jobs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    type text NOT NULL,
    version integer NOT NULL,
    parameters jsonb NOT NULL,
    idempotency_key text,
    state text NOT NULL DEFAULT 'queued',
    available_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    lease_owner text,
    leased_until timestamptz,
    last_error_code text,
    last_error_detail text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT jobs_type_check CHECK (
        type IN (
            'email_delivery',
            'ink_grant_monthly',
            'pass_expiry',
            'retention_run',
            'session_cleanup',
            'billing_reconciliation'
        )
    ),
    CONSTRAINT jobs_version_check CHECK (version >= 1),
    CONSTRAINT jobs_parameters_check CHECK (
        jsonb_typeof(parameters) = 'object'
        AND octet_length(parameters::text) <= 4096
    ),
    CONSTRAINT jobs_state_check CHECK (
        state IN ('queued', 'leased', 'succeeded', 'dead')
    ),
    CONSTRAINT jobs_attempts_check CHECK (
        attempts >= 0 AND attempts <= max_attempts
    ),
    CONSTRAINT jobs_max_attempts_check CHECK (max_attempts >= 1),
    CONSTRAINT jobs_idempotency_key_check CHECK (
        idempotency_key IS NULL
        OR (btrim(idempotency_key) <> '' AND char_length(idempotency_key) <= 200)
    ),
    -- A lease is all-or-nothing, and it belongs to exactly one lifecycle
    -- state: leased rows name a holder and an expiry; every other row has
    -- none. Terminal rows can never re-acquire one.
    CONSTRAINT jobs_lease_coherence_check CHECK (
        (state = 'leased')
        = (lease_owner IS NOT NULL AND leased_until IS NOT NULL)
    ),
    CONSTRAINT jobs_lease_owner_check CHECK (
        lease_owner IS NULL
        OR (btrim(lease_owner) <> '' AND char_length(lease_owner) <= 128)
    ),
    -- The redacted error is either absent or a complete pair: a stable code
    -- plus a bounded, already-sanitized detail. A code without a detail (or
    -- the reverse) is incoherent and rejected.
    CONSTRAINT jobs_last_error_check CHECK (
        (last_error_code IS NULL) = (last_error_detail IS NULL)
        AND (
            last_error_code IS NULL
            OR (
                last_error_code ~ '^JOB_[A-Z0-9_]{2,61}$'
                AND char_length(last_error_detail) <= 300
            )
        )
    ),
    -- A terminal row is a recorded outcome: it carries no lease.
    CONSTRAINT jobs_terminal_check CHECK (
        state NOT IN ('succeeded', 'dead')
        OR (lease_owner IS NULL AND leased_until IS NULL)
    ),
    CONSTRAINT jobs_idempotency_unique UNIQUE (idempotency_key)
);

-- Claim path: only queued rows that are due. The lease-expiry path has its
-- own partial index so an expired lease is found without scanning the queue.
CREATE INDEX IF NOT EXISTS jobs_claimable_idx
    ON app.jobs (available_at, id)
    WHERE state = 'queued';
CREATE INDEX IF NOT EXISTS jobs_lease_expiry_idx
    ON app.jobs (leased_until, id)
    WHERE state = 'leased';
CREATE INDEX IF NOT EXISTS jobs_state_idx
    ON app.jobs (state, updated_at DESC);

COMMENT ON TABLE app.jobs IS 'Durable job queue: one row per unit of work, leased with SKIP LOCKED and recoverable after a crashed holder';
COMMENT ON COLUMN app.jobs.type IS 'Closed workload vocabulary; handlers per type arrive with the worker';
COMMENT ON COLUMN app.jobs.version IS 'Schema version of the payload; a bump is an explicit, reviewable change';
COMMENT ON COLUMN app.jobs.parameters IS 'Bounded JSON object with identifiers, references and arguments (the job payload); rendered output, credentials and free prose are unrepresentable';
COMMENT ON COLUMN app.jobs.state IS 'Lifecycle: queued, leased, succeeded or dead';
COMMENT ON COLUMN app.jobs.available_at IS 'Earliest instant the job may run (also the retry instant after a failure)';
COMMENT ON COLUMN app.jobs.attempts IS 'Attempts already consumed; bounded by max_attempts';
COMMENT ON COLUMN app.jobs.lease_owner IS 'Worker that holds the lease; NULL for every non-leased row';
COMMENT ON COLUMN app.jobs.leased_until IS 'Instant the lease expires and the job becomes claimable again';
COMMENT ON COLUMN app.jobs.last_error_code IS 'Stable redacted code (JOB_*) of the last failure; never a provider message';
COMMENT ON COLUMN app.jobs.last_error_detail IS 'Bounded, already-sanitized failure detail; never a stack trace or payload echo';
COMMENT ON COLUMN app.jobs.idempotency_key IS 'Caller-chosen retry key, unique when set; NULL enqueues unconditionally';

-- +goose StatementBegin
-- Frozen provenance: a retry advances the lifecycle, never the identity of
-- the work. Rewriting type, version, payload, key, budget or creation
-- instant would silently change what was requested.
CREATE OR REPLACE FUNCTION app.jobs_freeze_provenance() RETURNS trigger AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.type IS DISTINCT FROM OLD.type
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.parameters IS DISTINCT FROM OLD.parameters
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.max_attempts IS DISTINCT FROM OLD.max_attempts
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'job provenance is immutable after enqueue (table %)', TG_TABLE_NAME
            USING ERRCODE = '23514', CONSTRAINT = 'jobs_provenance_immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS jobs_freeze_provenance_update ON app.jobs;
CREATE TRIGGER jobs_freeze_provenance_update
    BEFORE UPDATE ON app.jobs
    FOR EACH ROW EXECUTE FUNCTION app.jobs_freeze_provenance();

ALTER TABLE app.jobs OWNER TO arena_owner;
ALTER FUNCTION app.jobs_freeze_provenance() OWNER TO arena_owner;

-- Runtime surface: enqueue, claim, complete, fail and release all mutate the
-- row; DELETE is deliberately not granted.
GRANT SELECT, INSERT, UPDATE ON app.jobs TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS jobs_freeze_provenance_update ON app.jobs;
DROP FUNCTION IF EXISTS app.jobs_freeze_provenance();
DROP TABLE IF EXISTS app.jobs;
