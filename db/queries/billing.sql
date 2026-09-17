-- Arena Pass entitlement queries for the PostgreSQL platform adapter.
--
-- Consumption is append-only: no query updates or deletes a consumption and
-- no query mutates a lot's quantity or expiration. The runtime grants
-- enforce the same boundary in the database (P07-T01).

-- name: CreateArenaPassLot :one
INSERT INTO app.arena_pass_lots (account_id, origin, quantity, remaining_quantity, expires_at, reference)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at;

-- CreateArenaPassLotIfAbsent inserts the grant exactly once per
-- (account, origin, reference). On a conflict it returns no row, which tells
-- the adapter to resolve the original lot (P07-T02).
-- name: CreateArenaPassLotIfAbsent :one
INSERT INTO app.arena_pass_lots (account_id, origin, quantity, remaining_quantity, expires_at, reference)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (account_id, origin, reference) DO NOTHING
RETURNING id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at;

-- name: GetArenaPassLotByGrant :one
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1 AND origin = $2 AND reference = $3;

-- name: GetArenaPassLot :one
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE id = $1;

-- name: ListArenaPassLotsByAccount :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1
ORDER BY expires_at NULLS LAST, created_at;

-- ConsumeArenaPassLot atomically decrements a lot that still has passes. The
-- conditional predicate and the remaining_quantity CHECK together make
-- over-consumption impossible, even under concurrent consumers (P07-T03).
-- name: ConsumeArenaPassLot :execrows
UPDATE app.arena_pass_lots
SET remaining_quantity = remaining_quantity - 1
WHERE id = $1 AND remaining_quantity > 0;

-- ListAvailablePassLotsForUpdate locks the consumable lots of an account in
-- consumption order: nearest expiration first, then lots that never expire.
-- Expired lots are never candidates, so they can never be consumed (P07-T03).
-- name: ListAvailablePassLotsForUpdate :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1
  AND remaining_quantity > 0
  AND (expires_at IS NULL OR expires_at > sqlc.arg(at)::timestamptz)
ORDER BY expires_at ASC NULLS LAST, created_at ASC
FOR UPDATE;

-- name: GetArenaPassConsumptionByArena :one
SELECT id, lot_id, arena_id, consumed_at
FROM app.arena_pass_consumptions
WHERE arena_id = $1;

-- ListExpiredArenaPassLots derives the expired lots that still hold passes.
-- Expiration is never written back: the predicate is evaluated at read time,
-- so the sweep is a pure derivation and repeated runs are identical (P07-T04).
-- name: ListExpiredArenaPassLots :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE expires_at IS NOT NULL
  AND expires_at <= sqlc.arg(at)::timestamptz
  AND remaining_quantity > 0
ORDER BY expires_at ASC, id ASC
LIMIT sqlc.arg(page_limit);

-- name: CreateArenaPassConsumption :one
INSERT INTO app.arena_pass_consumptions (lot_id, arena_id)
VALUES ($1, $2)
RETURNING id, lot_id, arena_id, consumed_at;

-- name: ListArenaPassConsumptionsByAccount :many
SELECT c.id, c.lot_id, c.arena_id, c.consumed_at, l.origin, l.reference
FROM app.arena_pass_consumptions c
JOIN app.arena_pass_lots l ON l.id = c.lot_id
WHERE l.account_id = $1
ORDER BY c.consumed_at DESC, c.id DESC;

-- ListArenaPassConsumptionsPage returns one keyset-paginated page of the
-- owner's consumption history, newest first. NULL after_* parameters select
-- the first page; the (consumed_at, id) tuple comparison never duplicates or
-- skips rows (P07-T06).
-- name: ListArenaPassConsumptionsPage :many
SELECT c.id, c.lot_id, c.arena_id, c.consumed_at, l.origin, l.reference
FROM app.arena_pass_consumptions c
JOIN app.arena_pass_lots l ON l.id = c.lot_id
WHERE l.account_id = sqlc.arg(account_id)
  AND (
      sqlc.arg(after_consumed_at)::timestamptz IS NULL
      OR (c.consumed_at, c.id) < (sqlc.arg(after_consumed_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY c.consumed_at DESC, c.id DESC
LIMIT sqlc.arg(page_limit);
