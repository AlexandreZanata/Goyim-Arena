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

-- ListAuthorArenaReputation derives the reputation projection of one
-- author (P11-T05; BR §5.1, §6, §7): one row per Arena where the author
-- received at least one valid attribution, with the eligible people the
-- author influenced there and the valid attribution events received there.
--
-- Rules encoded here:
--   1. Only valid attributions count: invalidated ones never integrate the
--      valid totals (BR §6) and the row is retained for the administrative
--      trail (P11-T04).
--   2. DistinctPeople counts eligible attributors once per author and
--      Arena (BR §6), so repeated attributions by the same person never
--      inflate the headline.
--   3. Eligible attributor means an active account with a verified email
--      (BR §7), the same predicate the professional aggregates use.
--   4. Attribution events are facts: withdrawing or removing an argument
--      does not rewrite historical counts (BR §10); the attribution itself
--      is the exclusion unit.
-- The result carries counts only — attributor identifiers never leave the
-- database.
-- name: ListAuthorArenaReputation :many
SELECT
    ar.id AS arena_id,
    ar.category,
    ar.language,
    count(DISTINCT pa.attributor_id)::bigint AS distinct_people,
    count(*)::bigint AS valid_attributions
FROM app.persuasion_attributions pa
JOIN app.arguments a ON a.id = pa.argument_id
JOIN app.arenas ar ON ar.id = a.arena_id
JOIN app.accounts attributor ON attributor.id = pa.attributor_id
    AND attributor.status = 'active'
    AND attributor.email_verified_at IS NOT NULL
WHERE a.author_id = sqlc.arg(author_id)::uuid
  AND pa.status = 'valid'
GROUP BY ar.id, ar.category, ar.language
ORDER BY ar.id;

-- ResolveAuthorByUsername resolves a public username to the author identity
-- used by the reputation projection (P11-T06). Resolution is read-only over
-- the profiles projection and matches the canonical normalized username,
-- the only authority key of profile lookups (P05-T01): no profile field
-- crosses the port, only the resolved identity does. A username that owns no
-- profile returns no rows, which the adapter reports as ErrProfileNotFound.
-- name: ResolveAuthorByUsername :one
SELECT account_id, username
FROM app.profiles
WHERE username_normalized = lower(sqlc.arg(username)::text);

-- GetArgumentAttributionMetrics derives the public count facts of one
-- argument (P11-T06; BR §5.1, §6, §7): the valid attribution events it
-- received and the eligible people who credited it, each person counted
-- once per argument. Rules encoded here:
--   1. Only valid attributions count, exactly as the reputation projection
--      (BR §6): an invalidated attribution never integrates a valid total.
--   2. Eligible attributor means an active account with a verified email
--      (BR §7), the same predicate the professional aggregates use.
--   3. The LEFT JOIN keeps an argument with no eligible attribution
--      representable (zeros) while a missing argument still returns no row,
--      so the adapter can distinguish "no counts" from "no argument".
--   4. Counts are facts, not state: withdrawing or removing the argument
--      does not rewrite them (BR §10).
-- The result carries counts only — attributor identities never leave the
-- database.
-- name: GetArgumentAttributionMetrics :one
SELECT
    count(pa.argument_id)::bigint AS valid_attributions,
    count(DISTINCT pa.attributor_id)::bigint AS distinct_people
FROM app.arguments a
LEFT JOIN app.persuasion_attributions pa
    ON pa.argument_id = a.id
   AND pa.status = 'valid'
   AND EXISTS (
       SELECT 1
       FROM app.accounts attributor
       WHERE attributor.id = pa.attributor_id
         AND attributor.status = 'active'
         AND attributor.email_verified_at IS NOT NULL
   )
WHERE a.id = sqlc.arg(argument_id)::uuid
GROUP BY a.id;

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
