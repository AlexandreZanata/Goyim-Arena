-- +goose Up
-- 00008 establishes the append-only INK ledger (P06-T01):
-- app.wallet_accounts, app.wallet_operations and app.wallet_transactions.
--
-- Invariants (docs/MONETIZATION.md §2, docs/THREAT_MODEL.md THR-WAL-01/02):
-- 1. Every INK quantity is bigint: balances and amounts are never float,
--    numeric or money.
-- 2. wallet_accounts holds the cached balance projection per bucket
--    (FREE_INK, PURCHASED_INK) with strict CHECK (balance >= 0); the
--    append-only transaction ledger stays the source of truth and the
--    projection can always be rebuilt from it (P06-T05).
-- 3. wallet_operations is the idempotency registry of logical operations:
--    operation type, reference and a globally unique idempotency key. The
--    same key is accepted exactly once, so retries return the original
--    operation instead of duplicating it (P06-T03).
-- 4. wallet_transactions is strictly append-only: arena_app receives only
--    SELECT and INSERT. UPDATE and DELETE are denied at the privilege level
--    (THR-WAL-02), not merely by convention.
-- 5. Buckets are text with CHECK: FREE_INK is the plan franchise (consumed
--    first) and PURCHASED_INK is bought INK (does not expire initially).
-- 6. amount <> 0; positive values credit a bucket and negative values debit
--    it. One operation touches each bucket at most once, so a split debit
--    produces one or two transaction rows, never duplicates.
-- 7. Financial history is retained: wallet rows never cascade from
--    accounts (ON DELETE RESTRICT). Account deletion is anonymization, not
--    silent removal of ledger rows.
-- 8. Least privilege: arena_owner owns the objects; arena_app gets the
--    minimum DML (no DELETE anywhere; no UPDATE on the append-only tables).

CREATE TABLE IF NOT EXISTS app.wallet_accounts (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE RESTRICT,
    balance_free bigint NOT NULL DEFAULT 0,
    balance_purchased bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wallet_accounts_balance_free_check CHECK (balance_free >= 0),
    CONSTRAINT wallet_accounts_balance_purchased_check CHECK (balance_purchased >= 0)
);

COMMENT ON TABLE app.wallet_accounts IS 'Cached INK balance projection per account and bucket; the append-only transaction ledger is the source of truth';
COMMENT ON COLUMN app.wallet_accounts.balance_free IS 'FREE_INK balance (plan franchise, consumed before purchased INK); never negative';
COMMENT ON COLUMN app.wallet_accounts.balance_purchased IS 'PURCHASED_INK balance (bought INK, consumed after the franchise); never negative';

CREATE TABLE IF NOT EXISTS app.wallet_operations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.wallet_accounts(account_id) ON DELETE RESTRICT,
    operation_type text NOT NULL,
    idempotency_key text NOT NULL,
    reference text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wallet_operations_type_check CHECK (operation_type IN (
        'credit_free',
        'credit_member',
        'credit_purchase',
        'credit_refund',
        'credit_admin',
        'debit_argument',
        'debit_admin',
        'expire_free'
    )),
    CONSTRAINT wallet_operations_idempotency_key_check CHECK (trim(idempotency_key) <> ''),
    CONSTRAINT wallet_operations_reference_check CHECK (trim(reference) <> ''),
    CONSTRAINT wallet_operations_idempotency_key_unique UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS wallet_operations_account_id_idx ON app.wallet_operations (account_id, created_at DESC);

COMMENT ON TABLE app.wallet_operations IS 'Append-only idempotency registry of logical INK operations (credit, debit, expiry)';
COMMENT ON COLUMN app.wallet_operations.idempotency_key IS 'Globally unique retry key: the same key is accepted exactly once';
COMMENT ON COLUMN app.wallet_operations.reference IS 'Stable identifier of the cause (argument, Stripe event, moderation case, billing period)';

CREATE TABLE IF NOT EXISTS app.wallet_transactions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    operation_id uuid NOT NULL REFERENCES app.wallet_operations(id) ON DELETE RESTRICT,
    bucket text NOT NULL,
    amount bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wallet_transactions_bucket_check CHECK (bucket IN ('FREE_INK', 'PURCHASED_INK')),
    CONSTRAINT wallet_transactions_amount_check CHECK (amount <> 0),
    CONSTRAINT wallet_transactions_operation_bucket_unique UNIQUE (operation_id, bucket)
);

CREATE INDEX IF NOT EXISTS wallet_transactions_operation_id_idx ON app.wallet_transactions (operation_id);

COMMENT ON TABLE app.wallet_transactions IS 'Append-only INK ledger: signed bucket deltas (positive credit, negative debit); never updated or deleted at runtime';
COMMENT ON COLUMN app.wallet_transactions.amount IS 'Signed INK delta in bigint: positive credits the bucket, negative debits it; zero is forbidden';

ALTER TABLE app.wallet_accounts OWNER TO arena_owner;
ALTER TABLE app.wallet_operations OWNER TO arena_owner;
ALTER TABLE app.wallet_transactions OWNER TO arena_owner;

-- Runtime surface. wallet_accounts is a mutable projection (no DELETE: the
-- wallet is retained with its history); the ledger tables are append-only:
-- arena_app can read and insert, never update or delete (THR-WAL-02).
GRANT SELECT, INSERT, UPDATE ON app.wallet_accounts TO arena_app;
GRANT SELECT, INSERT ON app.wallet_operations TO arena_app;
GRANT SELECT, INSERT ON app.wallet_transactions TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.wallet_transactions;
DROP TABLE IF EXISTS app.wallet_operations;
DROP TABLE IF EXISTS app.wallet_accounts;
