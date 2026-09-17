-- GetPositionChangeForAttributor loads one position change scoped to its
-- account and locks it FOR UPDATE: attribution recording serializes per
-- change, so the cumulative three-argument limit cannot be bypassed by
-- concurrent requests (P11-T03). Reading the change row is the approved
-- read-only projection over the positions schema.
-- name: GetPositionChangeForAttributor :one
SELECT id, arena_id, account_id, changed_at
FROM app.position_changes
WHERE id = sqlc.arg(change_id)::uuid
  AND account_id = sqlc.arg(attributor_id)::uuid
FOR UPDATE;

-- ListAttributionArgumentIDs returns the argument identifiers already
-- credited by one change (P11-T03).
-- name: ListAttributionArgumentIDs :many
SELECT argument_id
FROM app.persuasion_attributions
WHERE position_change_id = sqlc.arg(change_id)::uuid
ORDER BY created_at, argument_id;

-- ListAttributionCandidates loads the eligibility inputs of the proposed
-- arguments: arena, author, creation instant and status. Relation is
-- deliberately not read: it never restricts eligibility (P11-T02).
-- name: ListAttributionCandidates :many
SELECT id, arena_id, author_id, created_at, status
FROM app.arguments
WHERE id = ANY(sqlc.arg(argument_ids)::uuid[]);

-- CreateAttribution records one attribution under the unique
-- (change, argument) pair: a retry inserts nothing and resolves the replay
-- (P11-T03).
-- name: CreateAttribution :one
INSERT INTO app.persuasion_attributions (position_change_id, attributor_id, argument_id)
VALUES (
    sqlc.arg(change_id)::uuid,
    sqlc.arg(attributor_id)::uuid,
    sqlc.arg(argument_id)::uuid
)
ON CONFLICT (position_change_id, argument_id) DO NOTHING
RETURNING id;
