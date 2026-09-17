-- Wallet ledger queries for the PostgreSQL platform adapter.
--
-- The ledger is append-only: these queries only create wallet rows, insert
-- operations/transactions and read them back. No query updates or deletes a
-- transaction, and the runtime grants enforce the same boundary in the
-- database (THR-WAL-02).

-- name: CreateWalletAccount :one
INSERT INTO app.wallet_accounts (account_id)
VALUES ($1)
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at;

-- name: GetWalletAccount :one
SELECT account_id, balance_free, balance_purchased, created_at, updated_at
FROM app.wallet_accounts
WHERE account_id = $1;

-- EnsureWalletAccount materializes the balance projection row for an
-- account; a pre-existing row is left untouched, including its balances.
-- name: EnsureWalletAccount :exec
INSERT INTO app.wallet_accounts (account_id)
VALUES ($1)
ON CONFLICT (account_id) DO NOTHING;

-- CreateWalletOperationIfAbsent inserts the operation exactly once per
-- idempotency key. On a conflict it returns no row, which tells the adapter
-- to replay the original operation (P06-T03).
-- name: CreateWalletOperationIfAbsent :one
INSERT INTO app.wallet_operations (account_id, operation_type, idempotency_key, reference)
VALUES ($1, $2, $3, $4)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, account_id, operation_type, idempotency_key, reference, created_at;

-- CreditFreeBalance adds the delta to the FREE_INK balance projection. The
-- CHECK (balance_free >= 0) guards the invariant even here.
-- name: CreditFreeBalance :one
UPDATE app.wallet_accounts
SET balance_free = balance_free + $2,
    updated_at = now()
WHERE account_id = $1
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at;

-- name: CreditPurchasedBalance :one
UPDATE app.wallet_accounts
SET balance_purchased = balance_purchased + $2,
    updated_at = now()
WHERE account_id = $1
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at;

-- name: CreateWalletOperation :one
INSERT INTO app.wallet_operations (account_id, operation_type, idempotency_key, reference)
VALUES ($1, $2, $3, $4)
RETURNING id, account_id, operation_type, idempotency_key, reference, created_at;

-- name: GetWalletOperationByIdempotencyKey :one
SELECT id, account_id, operation_type, idempotency_key, reference, created_at
FROM app.wallet_operations
WHERE idempotency_key = $1;

-- name: CreateWalletTransaction :one
INSERT INTO app.wallet_transactions (operation_id, bucket, amount)
VALUES ($1, $2, $3)
RETURNING id, operation_id, bucket, amount, created_at;

-- name: ListWalletTransactionsByAccount :many
SELECT t.id, t.operation_id, t.bucket, t.amount, t.created_at,
       o.operation_type, o.reference
FROM app.wallet_transactions t
JOIN app.wallet_operations o ON o.id = t.operation_id
WHERE o.account_id = $1
ORDER BY t.created_at DESC, t.id DESC;
