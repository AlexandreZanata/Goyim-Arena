-- +goose Up
-- 00010 makes administrative adjustments auditable directly on the ledger
-- operation (P06-T07): credit_admin and debit_admin rows must carry a
-- non-empty reason and the acting administrator account.
--
-- Invariants:
-- 1. reason and actor_account_id are optional for regular operations, but a
--    CHECK constraint makes them mandatory for the administrative types, so
--    no admin adjustment can exist without justification and actor even if
--    an application path is bypassed (THR-ADM-02).
-- 2. The ledger stays append-only for the runtime: the columns are part of
--    the immutable operation row, so an adjustment cannot be rewritten
--    retroactively (THR-WAL-02).
-- 3. The synchronous audit_events trail (internal/audit) arrives with
--    P14-T01; the wallet application records through its audit port and the
--    ledger row already carries the auditable facts.

ALTER TABLE app.wallet_operations
    ADD COLUMN IF NOT EXISTS reason text,
    ADD COLUMN IF NOT EXISTS actor_account_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'wallet_operations_reason_check'
    ) THEN
        ALTER TABLE app.wallet_operations
            ADD CONSTRAINT wallet_operations_reason_check
            CHECK (
                reason IS NULL
                OR (trim(reason) <> '' AND char_length(reason) <= 500)
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'wallet_operations_admin_audit_check'
    ) THEN
        ALTER TABLE app.wallet_operations
            ADD CONSTRAINT wallet_operations_admin_audit_check
            CHECK (
                operation_type NOT IN ('credit_admin', 'debit_admin')
                OR (
                    reason IS NOT NULL AND trim(reason) <> ''
                    AND actor_account_id IS NOT NULL
                )
            );
    END IF;
END
$$;
-- +goose StatementEnd

COMMENT ON COLUMN app.wallet_operations.reason IS 'Mandatory justification of administrative adjustments; optional for regular operations';
COMMENT ON COLUMN app.wallet_operations.actor_account_id IS 'Acting administrator account for administrative adjustments; never modified after insert';

-- +goose Down
ALTER TABLE app.wallet_operations DROP CONSTRAINT IF EXISTS wallet_operations_admin_audit_check;
ALTER TABLE app.wallet_operations DROP CONSTRAINT IF EXISTS wallet_operations_reason_check;
ALTER TABLE app.wallet_operations DROP COLUMN IF EXISTS actor_account_id;
ALTER TABLE app.wallet_operations DROP COLUMN IF EXISTS reason;
