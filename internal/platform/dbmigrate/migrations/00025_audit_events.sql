-- +goose Up
-- 00025 establishes the platform administrative audit trail (P14-T01):
-- app.audit_events.
--
-- Invariants (REQ-AUD-01, REQ-INV-09, docs/SECURITY.md §10):
-- 1. One row per administrative fact: stable uuidv7 identifier, caller
--    instant, acting account, namespaced action (module.operation), target
--    type and identifier, stable reason code, minimal metadata, and an
--    optional correlation grouping related events (for example a sanction
--    and its projection effect recorded together).
-- 2. Append-only for every role: the runtime holds SELECT and INSERT only,
--    and triggers reject UPDATE and DELETE with named constraints, so a
--    recorded fact can never be rewritten or erased (no silent history
--    mutation, THR-ADM-02).
-- 3. Metadata allowlist: the JSONB object carries only identifier,
--    reference and transition keys. Payloads, credentials, tokens,
--    sessions, emails, addresses, free prose and antifraud internals are
--    unrepresentable: any other top-level key fails the CHECK. Values are
--    bounded so a key cannot smuggle an unbounded dump either.
-- 4. Idempotency is caller-chosen: a NULL key records unconditionally
--    (several NULLs coexist), while a set key is unique, so retried
--    recorders resolve the original row instead of duplicating it.
-- 5. History is retained: actor and correlation references use ON DELETE
--    RESTRICT, and no cascade from accounts can erase the trail.
-- 6. Least privilege: arena_owner owns the objects; arena_app reads and
--    inserts, never updates or deletes.

CREATE TABLE IF NOT EXISTS app.audit_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    reason_code text NOT NULL,
    metadata jsonb,
    idempotency_key text,
    correlation_id text,
    CONSTRAINT audit_events_action_check CHECK (
        action ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'
        AND char_length(action) <= 100
    ),
    CONSTRAINT audit_events_target_type_check CHECK (
        target_type IN ('account', 'arena', 'argument', 'attribution', 'operation', 'case', 'action', 'appeal', 'intent', 'subscription', 'run')
    ),
    CONSTRAINT audit_events_target_id_check CHECK (
        btrim(target_id) <> '' AND char_length(target_id) <= 200
    ),
    CONSTRAINT audit_events_reason_code_check CHECK (
        btrim(reason_code) <> '' AND char_length(reason_code) <= 200
    ),
    CONSTRAINT audit_events_metadata_check CHECK (
        metadata IS NULL OR (
            jsonb_typeof(metadata) = 'object'
            AND (metadata - ARRAY[
                    'operation_id', 'reference', 'idempotency_key',
                    'arena_id', 'argument_id', 'attribution_id',
                    'target_account_id', 'case_id', 'action_id',
                    'appeal_id', 'intent_id', 'subscription_id',
                    'previous_status', 'new_status', 'outcome',
                    'replayed', 'rule', 'reason'
                ]) = '{}'::jsonb
            AND octet_length(metadata::text) <= 2048
        )
    ),
    CONSTRAINT audit_events_idempotency_key_check CHECK (
        idempotency_key IS NULL OR (btrim(idempotency_key) <> '' AND char_length(idempotency_key) <= 200)
    ),
    CONSTRAINT audit_events_idempotency_unique UNIQUE (idempotency_key),
    CONSTRAINT audit_events_correlation_check CHECK (
        correlation_id IS NULL OR (btrim(correlation_id) <> '' AND char_length(correlation_id) <= 200)
    )
);

CREATE INDEX IF NOT EXISTS audit_events_actor_idx
    ON app.audit_events (actor_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_target_idx
    ON app.audit_events (target_type, target_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_action_idx
    ON app.audit_events (action, occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_correlation_idx
    ON app.audit_events (correlation_id, occurred_at DESC)
    WHERE correlation_id IS NOT NULL;

COMMENT ON TABLE app.audit_events IS 'Append-only administrative audit trail: one immutable row per administrative fact, readable by support and auditors, never rewritten';
COMMENT ON COLUMN app.audit_events.action IS 'Namespaced stable operation code (module.operation); greppable across wallet, moderation, billing and admin sources';
COMMENT ON COLUMN app.audit_events.target_type IS 'Closed target vocabulary; the identifier lives in target_id';
COMMENT ON COLUMN app.audit_events.reason_code IS 'Stable reason or rule code; free prose never enters this column';
COMMENT ON COLUMN app.audit_events.metadata IS 'Minimal allowlisted JSONB object (identifiers, references, transitions); payloads, secrets and PII are unrepresentable';
COMMENT ON COLUMN app.audit_events.idempotency_key IS 'Caller-chosen retry key, unique when set; NULL records unconditionally';
COMMENT ON COLUMN app.audit_events.correlation_id IS 'Optional opaque correlation grouping related events recorded together';

-- +goose StatementBegin
-- The trail is evidence: no row is ever updated or deleted, by any role.
CREATE OR REPLACE FUNCTION app.audit_events_freeze() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit events are append-only and are never updated or deleted (table %)', TG_TABLE_NAME
        USING ERRCODE = '23514', CONSTRAINT = 'audit_events_immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS audit_events_freeze_update ON app.audit_events;
CREATE TRIGGER audit_events_freeze_update
    BEFORE UPDATE ON app.audit_events
    FOR EACH ROW EXECUTE FUNCTION app.audit_events_freeze();

DROP TRIGGER IF EXISTS audit_events_freeze_delete ON app.audit_events;
CREATE TRIGGER audit_events_freeze_delete
    BEFORE DELETE ON app.audit_events
    FOR EACH ROW EXECUTE FUNCTION app.audit_events_freeze();

ALTER TABLE app.audit_events OWNER TO arena_owner;
ALTER FUNCTION app.audit_events_freeze() OWNER TO arena_owner;

-- Runtime surface: the runtime reads and inserts; UPDATE and DELETE are
-- neither granted nor possible through the trigger.
GRANT SELECT, INSERT ON app.audit_events TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_freeze_delete ON app.audit_events;
DROP TRIGGER IF EXISTS audit_events_freeze_update ON app.audit_events;
DROP FUNCTION IF EXISTS app.audit_events_freeze();
DROP TABLE IF EXISTS app.audit_events;
