-- Wallet ledger queries for the PostgreSQL platform adapter.
--
-- The ledger is append-only: these queries only create wallet rows, insert
-- operations/transactions and read them back. No query updates or deletes a
-- transaction, and the runtime grants enforce the same boundary in the
-- database (THR-WAL-02).

-- name: CreateWalletAccount :one
INSERT INTO app.wallet_accounts (account_id)
VALUES ($1)
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at;

-- name: GetWalletAccount :one
SELECT account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at
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
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at;

-- name: CreditPurchasedBalance :one
UPDATE app.wallet_accounts
SET balance_purchased = balance_purchased + $2,
    updated_at = now()
WHERE account_id = $1
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at;

-- GetWalletAccountForUpdate locks the balance projection row of an account
-- for the duration of the transaction, serializing concurrent debits so no
-- double spend can pass the balance check (THR-WAL-01).
-- name: GetWalletAccountForUpdate :one
SELECT account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at
FROM app.wallet_accounts
WHERE account_id = $1
FOR UPDATE;

-- ApplyWalletDebit subtracts the planned bucket consumptions in a single
-- statement; the CHECK (balance >= 0) guards the invariant even if a caller
-- gets the plan wrong.
-- name: ApplyWalletDebit :one
UPDATE app.wallet_accounts
SET balance_free = balance_free - $2,
    balance_purchased = balance_purchased - $3,
    updated_at = now()
WHERE account_id = $1
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at;

-- name: ListWalletTransactionsByOperationID :many
SELECT id, operation_id, bucket, amount, created_at
FROM app.wallet_transactions
WHERE operation_id = $1
ORDER BY bucket;

-- name: GetWalletFreeCycleAnchor :one
SELECT free_cycle_anchor_at
FROM app.wallet_accounts
WHERE account_id = $1;

-- ApplyFreeBalanceDelta applies a signed net delta to the FREE_INK balance:
-- the monthly renewal expires the remaining franchise and grants the next
-- one in a single statement. The CHECK (balance_free >= 0) still guards the
-- invariant.
-- name: ApplyFreeBalanceDelta :one
UPDATE app.wallet_accounts
SET balance_free = balance_free + $2,
    updated_at = now()
WHERE account_id = $1
RETURNING account_id, balance_free, balance_purchased, created_at, updated_at, free_cycle_anchor_at;

-- GetDerivedWalletBalance recomputes both bucket balances exclusively from
-- the append-only ledger: the source of truth for the cached projection
-- (P06-T05, REQ-WAL-01).
-- name: GetDerivedWalletBalance :one
SELECT
    COALESCE(sum(t.amount) FILTER (WHERE t.bucket = 'FREE_INK'), 0)::bigint AS balance_free,
    COALESCE(sum(t.amount) FILTER (WHERE t.bucket = 'PURCHASED_INK'), 0)::bigint AS balance_purchased
FROM app.wallet_transactions t
JOIN app.wallet_operations o ON o.id = t.operation_id
WHERE o.account_id = $1;

-- ListWalletStatementPage returns one keyset-paginated page of the account
-- statement, newest first. NULL after_* parameters select the first page;
-- the (created_at, id) tuple comparison never duplicates or skips rows.
-- name: ListWalletStatementPage :many
SELECT t.id, t.operation_id, t.bucket, t.amount, t.created_at,
       o.operation_type, o.reference
FROM app.wallet_transactions t
JOIN app.wallet_operations o ON o.id = t.operation_id
WHERE o.account_id = sqlc.arg(account_id)
  AND (
      sqlc.arg(after_created_at)::timestamptz IS NULL
      OR (t.created_at, t.id) < (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(page_limit);

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
