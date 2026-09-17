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
