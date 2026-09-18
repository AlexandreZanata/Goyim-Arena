-- +goose Up
-- 00027 establishes the account deletion workflow (P14-T06,
-- docs/PRIVACY.md §4/§5, BR §10, REQ-PRIV-01).
--
-- Invariants:
-- 1. At most one active (requested) deletion per account: the partial
--    unique index resolves concurrent requests, so the loser re-reads the
--    winner and the cooldown never restarts.
-- 2. Lifecycle requested -> executed | canceled. Requested is the
--    cooling-off window and is the only cancellable state; executed and
--    canceled are terminal per record. A canceled request may be followed
--    by a fresh one, so the terminal history is retained as evidence.
-- 3. Records are retained for the audit trail even after the account is
--    anonymized: the account foreign key is RESTRICT and DELETE is
--    rejected for every role.
-- 4. Execution is a workflow over other schemas, so it lives in the
--    adapter transaction, not in a trigger: profiles and username history
--    are deleted (anonymized authorship), credentials, tokens and sessions
--    are deleted, the account row keeps its stable identifier with the
--    email replaced by an opaque placeholder, and billing/ledger/audit
--    rows are preserved untouched.
-- 5. Least privilege: arena_app reads, inserts and updates the request;
--    DELETE is neither granted nor possible through the trigger.

CREATE TABLE IF NOT EXISTS app.account_deletion_requests (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'requested',
    requested_at timestamptz NOT NULL DEFAULT now(),
    executed_at timestamptz,
    canceled_at timestamptz,
    cancel_reason text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_deletion_requests_status_check CHECK (status IN ('requested', 'executed', 'canceled')),
    CONSTRAINT account_deletion_requests_requested_check CHECK (
        status <> 'requested'
        OR (executed_at IS NULL AND canceled_at IS NULL AND cancel_reason IS NULL)
    ),
    CONSTRAINT account_deletion_requests_executed_check CHECK (
        status <> 'executed'
        OR (executed_at IS NOT NULL AND executed_at >= requested_at AND canceled_at IS NULL AND cancel_reason IS NULL)
    ),
    CONSTRAINT account_deletion_requests_canceled_check CHECK (
        status <> 'canceled'
        OR (
            canceled_at IS NOT NULL
            AND canceled_at >= requested_at
            AND executed_at IS NULL
            AND cancel_reason IS NOT NULL
            AND btrim(cancel_reason) <> ''
            AND char_length(cancel_reason) <= 500
        )
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS account_deletion_requests_active_unique
    ON app.account_deletion_requests (account_id)
    WHERE status = 'requested';

CREATE INDEX IF NOT EXISTS account_deletion_requests_account_idx
    ON app.account_deletion_requests (account_id, requested_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS account_deletion_requests_due_idx
    ON app.account_deletion_requests (requested_at)
    WHERE status = 'requested';

COMMENT ON TABLE app.account_deletion_requests IS 'Account deletion state machine: requested (cooling-off, cancellable), executed or canceled; the record is retained as evidence after execution';
COMMENT ON COLUMN app.account_deletion_requests.cancel_reason IS 'Holder-provided cancellation reason; restricted evidence, never part of a public projection';
COMMENT ON COLUMN app.account_deletion_requests.executed_at IS 'Instant the anonymization executed; after it the account can never authenticate again';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.account_deletion_requests_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'account deletion records are retained evidence'
            USING ERRCODE = '23514', CONSTRAINT = 'account_deletion_requests_retained';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id IS DISTINCT FROM OLD.id
            OR NEW.account_id IS DISTINCT FROM OLD.account_id
            OR NEW.requested_at IS DISTINCT FROM OLD.requested_at
        THEN
            RAISE EXCEPTION 'deletion request identity and provenance are immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'account_deletion_requests_immutable';
        END IF;
        IF OLD.status <> 'requested' AND NEW.status IS DISTINCT FROM OLD.status THEN
            RAISE EXCEPTION 'executed and canceled deletion requests are terminal'
                USING ERRCODE = '23514', CONSTRAINT = 'account_deletion_requests_terminal';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS account_deletion_requests_guard_update ON app.account_deletion_requests;
CREATE TRIGGER account_deletion_requests_guard_update
    BEFORE UPDATE ON app.account_deletion_requests
    FOR EACH ROW EXECUTE FUNCTION app.account_deletion_requests_guard();

DROP TRIGGER IF EXISTS account_deletion_requests_retained_delete ON app.account_deletion_requests;
CREATE TRIGGER account_deletion_requests_retained_delete
    BEFORE DELETE ON app.account_deletion_requests
    FOR EACH ROW EXECUTE FUNCTION app.account_deletion_requests_guard();

ALTER TABLE app.account_deletion_requests OWNER TO arena_owner;
ALTER FUNCTION app.account_deletion_requests_guard() OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE ON app.account_deletion_requests TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS account_deletion_requests_retained_delete ON app.account_deletion_requests;
DROP TRIGGER IF EXISTS account_deletion_requests_guard_update ON app.account_deletion_requests;
DROP FUNCTION IF EXISTS app.account_deletion_requests_guard();
DROP TABLE IF EXISTS app.account_deletion_requests;
