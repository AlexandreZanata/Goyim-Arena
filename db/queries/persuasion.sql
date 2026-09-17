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

-- GetAttributionForModeration loads one attribution with its current
-- validity and its latest moderation decision and locks it FOR UPDATE, so
-- concurrent decisions on the same row serialize instead of overwriting
-- each other (P11-T04). It is the moderation read projection: a consumer
-- reads validity from the retained row that the schema keeps coherent with
-- the decision record, so no projection can observe an unrecorded
-- invalidation.
-- name: GetAttributionForModeration :one
SELECT id, position_change_id, attributor_id, argument_id, status, created_at,
       moderation_reason, moderated_by, moderated_at
FROM app.persuasion_attributions
WHERE id = sqlc.arg(attribution_id)::uuid
FOR UPDATE;

-- InvalidateAttribution moves a valid attribution to invalid, recording the
-- actor, the mandatory reason and the instant on the retained row; nothing
-- is deleted. The status guard loses the race instead of overwriting a
-- concurrent decision.
-- name: InvalidateAttribution :one
UPDATE app.persuasion_attributions
SET status = 'invalid',
    invalidated_at = sqlc.arg(decided_at)::timestamptz,
    moderation_reason = sqlc.arg(reason)::text,
    moderated_by = sqlc.arg(moderator_id)::uuid,
    moderated_at = sqlc.arg(decided_at)::timestamptz
WHERE id = sqlc.arg(attribution_id)::uuid
  AND status = 'valid'
RETURNING id, position_change_id, attributor_id, argument_id, status, created_at,
          moderation_reason, moderated_by, moderated_at;

-- RestoreAttribution reverses one invalidation on the same retained row,
-- recording the restore decision: the row moves back to valid and the
-- decision record is replaced by the newest one, never erased.
-- name: RestoreAttribution :one
UPDATE app.persuasion_attributions
SET status = 'valid',
    invalidated_at = NULL,
    moderation_reason = sqlc.arg(reason)::text,
    moderated_by = sqlc.arg(moderator_id)::uuid,
    moderated_at = sqlc.arg(decided_at)::timestamptz
WHERE id = sqlc.arg(attribution_id)::uuid
  AND status = 'invalid'
RETURNING id, position_change_id, attributor_id, argument_id, status, created_at,
          moderation_reason, moderated_by, moderated_at;
