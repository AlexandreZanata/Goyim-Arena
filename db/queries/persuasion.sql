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

-- ListAttributionReciprocity loads, for one subject and window, the accounts
-- that credited the subject's arguments while the subject credited theirs
-- (P11-T07; THR-PERS-01): the raw material of the reciprocity signal. Rules
-- encoded here:
--   1. Only valid attributions count, exactly as the public metrics: an
--      invalidated attribution is already a closed case.
--   2. Both directions must exist for the pair to appear; whether the pair
--      crosses the policy threshold is the domain's judgment, not SQL's.
--   3. No eligibility filter on purpose: abuse review must not be blind to
--      suspended or unverified accounts, which are precisely the population
--      coordinated manipulation uses. Eligibility still excludes them from
--      the official results (BR §7); these facts never reach a public
--      projection (CONSTITUTION §Dados pessoais).
-- name: ListAttributionReciprocity :many
WITH attributed_to_subject AS (
    SELECT pa.attributor_id AS account_id, count(*)::bigint AS inbound
    FROM app.persuasion_attributions pa
    JOIN app.arguments a ON a.id = pa.argument_id
    WHERE a.author_id = sqlc.arg(subject_id)::uuid
      AND pa.status = 'valid'
      AND pa.created_at >= sqlc.arg(window_start)::timestamptz
      AND pa.created_at < sqlc.arg(window_end)::timestamptz
    GROUP BY pa.attributor_id
),
attributed_by_subject AS (
    SELECT a.author_id AS account_id, count(*)::bigint AS outbound
    FROM app.persuasion_attributions pa
    JOIN app.arguments a ON a.id = pa.argument_id
    WHERE pa.attributor_id = sqlc.arg(subject_id)::uuid
      AND pa.status = 'valid'
      AND pa.created_at >= sqlc.arg(window_start)::timestamptz
      AND pa.created_at < sqlc.arg(window_end)::timestamptz
    GROUP BY a.author_id
)
SELECT inbound.account_id, inbound.inbound, attributed_by_subject.outbound
FROM attributed_to_subject inbound
JOIN attributed_by_subject ON attributed_by_subject.account_id = inbound.account_id
ORDER BY inbound.account_id;

-- ListAttributionConcentration loads, for one subject and window, how many
-- valid attributions each account made to the subject's arguments (P11-T07;
-- METRICS §4). The share and the threshold are computed by the domain, so the
-- same aggregation serves every policy revision. The eligibility filter is
-- deliberately absent, for the reason documented above.
-- name: ListAttributionConcentration :many
SELECT pa.attributor_id AS account_id, count(*)::bigint AS events
FROM app.persuasion_attributions pa
JOIN app.arguments a ON a.id = pa.argument_id
WHERE a.author_id = sqlc.arg(subject_id)::uuid
  AND pa.status = 'valid'
  AND pa.created_at >= sqlc.arg(window_start)::timestamptz
  AND pa.created_at < sqlc.arg(window_end)::timestamptz
GROUP BY pa.attributor_id
ORDER BY events DESC, account_id;

-- ListAttributionAlternation loads, for one subject and window, the position
-- changes of every account that credited the subject, with how many of those
-- changes were reversals (P11-T07; METRICS §4 "reversões repetidas pela mesma
-- conta"): a change back to the position held before the previous change.
--
-- The chain window function deliberately reads every change of the account,
-- not only the ones inside the window, so the predecessor of an in-window
-- change is its real predecessor; the counters then keep only the in-window
-- events.
-- name: ListAttributionAlternation :many
WITH credited AS (
    SELECT DISTINCT pa.attributor_id AS account_id
    FROM app.persuasion_attributions pa
    JOIN app.arguments a ON a.id = pa.argument_id
    WHERE a.author_id = sqlc.arg(subject_id)::uuid
      AND pa.status = 'valid'
      AND pa.created_at >= sqlc.arg(window_start)::timestamptz
      AND pa.created_at < sqlc.arg(window_end)::timestamptz
),
chain AS (
    SELECT
        pc.account_id,
        pc.changed_at,
        pc.to_position,
        lag(pc.to_position, 1) OVER (PARTITION BY pc.account_id ORDER BY pc.changed_at, pc.id) AS previous_position,
        lag(pc.to_position, 2) OVER (PARTITION BY pc.account_id ORDER BY pc.changed_at, pc.id) AS position_before_previous
    FROM app.position_changes pc
    JOIN credited ON credited.account_id = pc.account_id
)
SELECT
    account_id,
    count(*) FILTER (
        WHERE changed_at >= sqlc.arg(window_start)::timestamptz
          AND changed_at < sqlc.arg(window_end)::timestamptz
    )::bigint AS changes,
    count(*) FILTER (
        WHERE changed_at >= sqlc.arg(window_start)::timestamptz
          AND changed_at < sqlc.arg(window_end)::timestamptz
          AND position_before_previous IS NOT NULL
          AND to_position = position_before_previous
    )::bigint AS reversals
FROM chain
GROUP BY account_id
ORDER BY account_id;

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
