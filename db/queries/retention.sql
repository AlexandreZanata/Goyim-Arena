-- Data retention statements (P14-T07, docs/PRIVACY.md §1/§5,
-- REQ-PRIV-01). One schedule per governed class decides the action; these
-- statements execute it and count what happened.
--
-- Every purge and anonymize statement:
--   * is scoped by the class cutoff (now minus the class window) applied to
--     the terminal instant of the record (used, revoked or expired), so a
--     record inside the window is never touched;
--   * excludes the accounts covered by an active hold of that class, and
--     the whole class when a class-wide hold is in force;
--   * returns the affected count and the count preserved by holds in one
--     statement, so the ledger cannot disagree with what happened.
--
-- Retained classes purge nothing by design: their statements only count the
-- records kept under obligation.

-- ListActiveRetentionHolds resolves the holds in force for the job.
-- name: ListActiveRetentionHolds :many
SELECT id, data_class, account_id, reason_code, placed_at
FROM app.retention_holds
WHERE released_at IS NULL
ORDER BY placed_at, id;

-- PurgeTerminalVerificationTokens removes verification tokens that were
-- used or expired at or before the cutoff.
-- name: PurgeTerminalVerificationTokens :one
WITH scope AS (
    SELECT token.id, token.account_id
    FROM app.email_verification_tokens token
    WHERE (token.used_at IS NOT NULL AND token.used_at <= sqlc.arg(cutoff)::timestamptz)
       OR token.expires_at <= sqlc.arg(cutoff)::timestamptz
),
purged AS (
    DELETE FROM app.email_verification_tokens target
    USING scope
    WHERE target.id = scope.id
      AND NOT sqlc.arg(class_held)::boolean
      AND NOT (scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[])))
    RETURNING 1 AS row_count
),
preserved AS (
    SELECT 1
    FROM scope
    WHERE sqlc.arg(class_held)::boolean
       OR scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[]))
)
SELECT
    (SELECT count(*) FROM purged)::integer AS purged,
    (SELECT count(*) FROM preserved)::integer AS held;

-- PurgeTerminalRecoveryTokens removes password recovery tokens that were
-- used or expired at or before the cutoff.
-- name: PurgeTerminalRecoveryTokens :one
WITH scope AS (
    SELECT token.id, token.account_id
    FROM app.password_reset_tokens token
    WHERE (token.used_at IS NOT NULL AND token.used_at <= sqlc.arg(cutoff)::timestamptz)
       OR token.expires_at <= sqlc.arg(cutoff)::timestamptz
),
purged AS (
    DELETE FROM app.password_reset_tokens target
    USING scope
    WHERE target.id = scope.id
      AND NOT sqlc.arg(class_held)::boolean
      AND NOT (scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[])))
    RETURNING 1 AS row_count
),
preserved AS (
    SELECT 1
    FROM scope
    WHERE sqlc.arg(class_held)::boolean
       OR scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[]))
)
SELECT
    (SELECT count(*) FROM purged)::integer AS purged,
    (SELECT count(*) FROM preserved)::integer AS held;

-- PurgeTerminalSessions removes sessions revoked or expired at or before
-- the cutoff; the restricted client references travel with the row.
-- name: PurgeTerminalSessions :one
WITH scope AS (
    SELECT session.id, session.account_id
    FROM app.sessions session
    WHERE (session.revoked_at IS NOT NULL AND session.revoked_at <= sqlc.arg(cutoff)::timestamptz)
       OR session.expires_at <= sqlc.arg(cutoff)::timestamptz
),
purged AS (
    DELETE FROM app.sessions target
    USING scope
    WHERE target.id = scope.id
      AND NOT sqlc.arg(class_held)::boolean
      AND NOT (scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[])))
    RETURNING 1 AS row_count
),
preserved AS (
    SELECT 1
    FROM scope
    WHERE sqlc.arg(class_held)::boolean
       OR scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[]))
)
SELECT
    (SELECT count(*) FROM purged)::integer AS purged,
    (SELECT count(*) FROM preserved)::integer AS held;

-- AnonymizeTerminalSessionReferentials strips the client IP and user agent
-- of terminal sessions past the prevention window. The session row itself
-- survives until its own, longer retention window elapses.
-- name: AnonymizeTerminalSessionReferentials :one
WITH scope AS (
    SELECT session.id, session.account_id
    FROM app.sessions session
    WHERE (
            (session.revoked_at IS NOT NULL AND session.revoked_at <= sqlc.arg(cutoff)::timestamptz)
            OR session.expires_at <= sqlc.arg(cutoff)::timestamptz
        )
      AND (session.ip_address IS NOT NULL OR session.user_agent IS NOT NULL)
),
anonymized AS (
    UPDATE app.sessions target
    SET ip_address = NULL,
        user_agent = NULL
    FROM scope
    WHERE target.id = scope.id
      AND NOT sqlc.arg(class_held)::boolean
      AND NOT (scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[])))
    RETURNING 1 AS row_count
),
preserved AS (
    SELECT 1
    FROM scope
    WHERE sqlc.arg(class_held)::boolean
       OR scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[]))
)
SELECT
    (SELECT count(*) FROM anonymized)::integer AS anonymized,
    (SELECT count(*) FROM preserved)::integer AS held;

-- PurgeExpiredExportDocuments purges the document bytes of exports whose
-- link expired at or before the cutoff and expires requests that were never
-- generated by the cutoff. The record itself is retained: migration 00026
-- forbids deleting export rows, and the download capability is already
-- worthless once the record is expired.
-- name: PurgeExpiredExportDocuments :one
WITH scope AS (
    SELECT export.id, export.account_id
    FROM app.data_exports export
    WHERE (export.status = 'ready' AND export.expires_at <= sqlc.arg(cutoff)::timestamptz)
       OR (export.status = 'requested' AND export.requested_at <= sqlc.arg(cutoff)::timestamptz)
),
purged AS (
    UPDATE app.data_exports target
    SET status = 'expired',
        document = NULL,
        document_sha256 = NULL,
        updated_at = sqlc.arg(executed_at)::timestamptz
    FROM scope
    WHERE target.id = scope.id
      AND NOT sqlc.arg(class_held)::boolean
      AND NOT (scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[])))
    RETURNING 1 AS row_count
),
preserved AS (
    SELECT 1
    FROM scope
    WHERE sqlc.arg(class_held)::boolean
       OR scope.account_id = ANY (COALESCE(sqlc.arg(held_accounts)::uuid[], '{}'::uuid[]))
)
SELECT
    (SELECT count(*) FROM purged)::integer AS purged,
    (SELECT count(*) FROM preserved)::integer AS held;

-- CountRetainedAuditEvents counts the administrative trail kept as
-- evidence. The trail is append-only for every role, so the retention
-- policy never schedules a purge for this class.
-- name: CountRetainedAuditEvents :one
SELECT count(*)::integer AS retained
FROM app.audit_events;

-- CountRetainedBillingRows counts the payment, subscription, refund and
-- reconciliation records kept as evidence of money and of the
-- reconciliation duty. No row of the billing module is ever deleted.
-- name: CountRetainedBillingRows :one
SELECT (
    (SELECT count(*) FROM app.stripe_customers)
    + (SELECT count(*) FROM app.stripe_events)
    + (SELECT count(*) FROM app.checkout_intents)
    + (SELECT count(*) FROM app.subscriptions)
    + (SELECT count(*) FROM app.billing_refunds)
    + (SELECT count(*) FROM app.billing_reconciliation_runs)
    + (SELECT count(*) FROM app.billing_reconciliation_findings)
)::integer AS retained;

-- CreateRetentionRun appends one ledger row per class and execution
-- instant. A replayed run resolves the original record instead of
-- duplicating or rewriting it.
-- name: CreateRetentionRun :one
INSERT INTO app.retention_runs (
    data_class, executed_at, cutoff_at, purged_count, anonymized_count, retained_count, held_count
)
VALUES (
    sqlc.arg(data_class)::text,
    sqlc.arg(executed_at)::timestamptz,
    sqlc.narg(cutoff_at)::timestamptz,
    sqlc.arg(purged_count)::integer,
    sqlc.arg(anonymized_count)::integer,
    sqlc.arg(retained_count)::integer,
    sqlc.arg(held_count)::integer
)
ON CONFLICT (data_class, executed_at) DO NOTHING
RETURNING id, data_class, executed_at, cutoff_at, purged_count, anonymized_count, retained_count, held_count;

-- GetRetentionRunForClassAt resolves the recorded run of one class and
-- instant, so a replay reports what the ledger holds instead of inventing a
-- second outcome.
-- name: GetRetentionRunForClassAt :one
SELECT id, data_class, executed_at, cutoff_at, purged_count, anonymized_count, retained_count, held_count
FROM app.retention_runs
WHERE data_class = sqlc.arg(data_class)::text
  AND executed_at = sqlc.arg(executed_at)::timestamptz;
