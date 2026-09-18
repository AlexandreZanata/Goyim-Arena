-- Public Arena export projections (P14-T04, docs/TRANSPARENCY.md §7). These
-- are the approved read-only projections over the arenas, positions,
-- arguments, sources and attribution schemas: every statement only reads,
-- returns public columns and never lets an account identifier leave the
-- database. Aggregates carry integer counts; the argument list is strictly
-- keyset-paginated so a large Arena is read page by page.

-- GetArenaExportHeader resolves the publicly readable Arena (published,
-- closed or restricted) with its derived aggregates in one read: eligible
-- participant distributions, the total accepted position changes and the
-- valid influence counts. Unknown, draft and removed Arenas return no row.
-- The eligibility predicates mirror the public position aggregate
-- (CountEligiblePositionsByArena) and the public attribution metrics
-- (GetArgumentAttributionMetrics) exactly.
-- name: GetArenaExportHeader :one
SELECT
    a.id, a.slug, a.statement, a.context, a.category, a.language, a.status,
    a.published_at, a.closes_at,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id)::bigint AS participants_total,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.initial_position = 'agree')::bigint AS initial_agree,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.initial_position = 'disagree')::bigint AS initial_disagree,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.initial_position = 'undecided')::bigint AS initial_undecided,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.current_position = 'agree')::bigint AS current_agree,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.current_position = 'disagree')::bigint AS current_disagree,
    (SELECT count(*)::bigint
        FROM app.debate_positions dp
        JOIN app.accounts participant ON participant.id = dp.account_id
            AND participant.status = 'active'
        WHERE dp.arena_id = a.id AND dp.current_position = 'undecided')::bigint AS current_undecided,
    (SELECT count(*)::bigint
        FROM app.position_changes pc
        WHERE pc.arena_id = a.id)::bigint AS position_changes,
    (SELECT count(*)::bigint
        FROM app.persuasion_attributions pa
        JOIN app.arguments influence_argument ON influence_argument.id = pa.argument_id
        JOIN app.accounts attributor ON attributor.id = pa.attributor_id
            AND attributor.status = 'active'
            AND attributor.email_verified_at IS NOT NULL
        WHERE influence_argument.arena_id = a.id AND pa.status = 'valid')::bigint AS valid_attributions,
    (SELECT count(DISTINCT influence_argument.author_id)::bigint
        FROM app.persuasion_attributions pa
        JOIN app.arguments influence_argument ON influence_argument.id = pa.argument_id
        JOIN app.accounts attributor ON attributor.id = pa.attributor_id
            AND attributor.status = 'active'
            AND attributor.email_verified_at IS NOT NULL
        WHERE influence_argument.arena_id = a.id AND pa.status = 'valid')::bigint AS influenced_authors
FROM app.arenas a
WHERE a.id = sqlc.arg(arena_id)::uuid
  AND a.status IN ('published', 'closed', 'restricted');

-- ListArenaExportArguments returns one bounded page of published and
-- withdrawn arguments of one Arena, oldest first, strictly after the
-- position. Removed arguments never appear. The adapter withholds the
-- content of withdrawn arguments exactly as the public argument adapter
-- does: the placeholder keeps the public status and dates while the
-- retracted text never reaches the document. Influence counts mirror
-- GetArgumentAttributionMetrics and remain historical facts (withdrawal
-- never rewrites them).
-- name: ListArenaExportArguments :many
SELECT
    arg.id,
    arg.parent_id,
    arg.relation,
    arg.content,
    arg.status,
    arg.created_at,
    arg.withdrawn_at,
    (SELECT count(*)::bigint
        FROM app.persuasion_attributions pa
        WHERE pa.argument_id = arg.id
          AND pa.status = 'valid'
          AND EXISTS (
              SELECT 1
              FROM app.accounts attributor
              WHERE attributor.id = pa.attributor_id
                AND attributor.status = 'active'
                AND attributor.email_verified_at IS NOT NULL
          ))::bigint AS valid_attributions,
    (SELECT count(DISTINCT pa.attributor_id)::bigint
        FROM app.persuasion_attributions pa
        WHERE pa.argument_id = arg.id
          AND pa.status = 'valid'
          AND EXISTS (
              SELECT 1
              FROM app.accounts attributor
              WHERE attributor.id = pa.attributor_id
                AND attributor.status = 'active'
                AND attributor.email_verified_at IS NOT NULL
          ))::bigint AS distinct_people
FROM app.arguments arg
WHERE arg.arena_id = sqlc.arg(arena_id)::uuid
  AND arg.status IN ('published', 'withdrawn')
  AND (
      sqlc.arg(after_created_at)::timestamptz IS NULL
      OR (arg.created_at, arg.id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY arg.created_at, arg.id
LIMIT sqlc.arg(page_limit);

-- ListExportArgumentSources returns the structured sources of the given
-- arguments in deterministic order. The adapter only asks for published
-- arguments: withdrawn content withholds its sources too.
-- name: ListExportArgumentSources :many
SELECT s.argument_id, s.url, s.description
FROM app.argument_sources s
WHERE s.argument_id = ANY(sqlc.arg(argument_ids)::uuid[])
ORDER BY s.created_at, s.id;
