-- Arena Pass entitlement queries for the PostgreSQL platform adapter.
--
-- Consumption is append-only: no query updates or deletes a consumption and
-- no query mutates a lot's quantity or expiration. The runtime grants
-- enforce the same boundary in the database (P07-T01).

-- name: CreateArenaPassLot :one
INSERT INTO app.arena_pass_lots (account_id, origin, quantity, remaining_quantity, expires_at, reference)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at;

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
