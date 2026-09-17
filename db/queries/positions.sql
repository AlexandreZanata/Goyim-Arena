-- ConfirmInitialPosition inserts the initial projection of one account in
-- one Arena. The primary key (arena_id, account_id) resolves concurrent
-- confirmations: the loser inserts nothing and re-reads the winner (P09-T03).
-- name: ConfirmInitialPosition :one
INSERT INTO app.debate_positions (arena_id, account_id, initial_position, current_position, created_at, updated_at)
VALUES (
    sqlc.arg(arena_id)::uuid,
    sqlc.arg(account_id)::uuid,
    sqlc.arg(position)::text,
    sqlc.arg(position)::text,
    sqlc.arg(at)::timestamptz,
    sqlc.arg(at)::timestamptz
)
ON CONFLICT (arena_id, account_id) DO NOTHING
RETURNING arena_id, account_id, initial_position, current_position, version, created_at, updated_at;

-- GetDebatePosition returns the private projection of one account in one
-- Arena.
-- name: GetDebatePosition :one
SELECT arena_id, account_id, initial_position, current_position, version, created_at, updated_at
FROM app.debate_positions
WHERE arena_id = $1 AND account_id = $2;

-- CreatePositionChange appends one change to the immutable history chain.
-- The unique (arena_id, account_id, version) constraint resolves concurrent
-- changes: only the chain tip advances (P09-T04).
-- name: CreatePositionChange :one
INSERT INTO app.position_changes (arena_id, account_id, from_position, to_position, version, changed_at)
VALUES (
    sqlc.arg(arena_id)::uuid,
    sqlc.arg(account_id)::uuid,
    sqlc.arg(from_position)::text,
    sqlc.arg(to_position)::text,
    sqlc.arg(version)::integer,
    sqlc.arg(changed_at)::timestamptz
)
RETURNING id;

-- UpdateCurrentPosition moves the projection to the change target under the
-- optimistic version check: zero rows mean the chain moved since it was read
-- and the whole change transaction must roll back (P09-T04).
-- name: UpdateCurrentPosition :execrows
UPDATE app.debate_positions
SET current_position = sqlc.arg(current_position)::text,
    version = sqlc.arg(version)::integer,
    updated_at = sqlc.arg(at)::timestamptz
WHERE arena_id = sqlc.arg(arena_id)::uuid
  AND account_id = sqlc.arg(account_id)::uuid
  AND version = sqlc.arg(expected_version)::integer;
