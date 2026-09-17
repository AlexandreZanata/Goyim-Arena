-- CreateArgument inserts one argument under the author idempotency key. The
-- partial unique index resolves concurrent attempts with the same key: the
-- loser gets no row back and resolves the replay (P10-T04).
-- name: CreateArgument :one
INSERT INTO app.arguments (
    arena_id, author_id, parent_id, relation, content, content_hash,
    grapheme_cost, idempotency_key, created_at, updated_at
)
VALUES (
    sqlc.arg(arena_id)::uuid,
    sqlc.arg(author_id)::uuid,
    sqlc.narg(parent_id)::uuid,
    sqlc.arg(relation)::text,
    sqlc.arg(content)::text,
    sqlc.arg(content_hash)::text,
    sqlc.arg(grapheme_cost)::integer,
    sqlc.arg(idempotency_key)::text,
    sqlc.arg(created_at)::timestamptz,
    sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (author_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at, withdrawn_at;

-- GetArgumentByAuthorAndKey resolves the argument recorded under one
-- attempt key (P10-T04).
-- name: GetArgumentByAuthorAndKey :one
SELECT id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at, withdrawn_at
FROM app.arguments
WHERE author_id = sqlc.arg(author_id)::uuid
  AND idempotency_key = sqlc.arg(idempotency_key)::text;

-- GetParentArgument returns one argument together with its derived depth
-- (0 for a top-level argument). Replies walk the chain through the
-- recursive CTE, so depth is never denormalized; the depth guard bounds a
-- corrupted chain defensively (P10-T05).
-- name: GetParentArgument :one
WITH RECURSIVE chain AS (
    SELECT id, parent_id, 0::integer AS depth
    FROM app.arguments
    WHERE id = sqlc.arg(argument_id)::uuid
    UNION ALL
    SELECT parent.id, parent.parent_id, chain.depth + 1
    FROM app.arguments parent
    JOIN chain ON parent.id = chain.parent_id
    WHERE chain.depth < 1000
)
SELECT
    a.id, a.arena_id, a.author_id, a.parent_id, a.relation, a.content,
    a.content_hash, a.grapheme_cost, a.status, a.created_at, a.updated_at, a.withdrawn_at,
    (SELECT max(chain.depth) FROM chain)::integer AS depth
FROM app.arguments a
WHERE a.id = sqlc.arg(argument_id)::uuid;

-- CreateArgumentSource attaches one structured source to an argument.
-- name: CreateArgumentSource :one
INSERT INTO app.argument_sources (argument_id, url, description, created_at)
VALUES (
    sqlc.arg(argument_id)::uuid,
    sqlc.arg(url)::text,
    sqlc.narg(description)::text,
    sqlc.arg(created_at)::timestamptz
)
RETURNING id;

-- GetArgumentForAuthor returns one argument scoped to its author. A foreign
-- argument is deliberately indistinguishable from a missing one (P10-T06).
-- name: GetArgumentForAuthor :one
SELECT id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at, withdrawn_at
FROM app.arguments
WHERE id = sqlc.arg(argument_id)::uuid
  AND author_id = sqlc.arg(author_id)::uuid;

-- WithdrawArgument moves a published argument out of the display under the
-- author scope, recording the withdrawal instant once. Zero rows mean the
-- status moved concurrently: the caller re-reads and resolves (P10-T06).
-- name: WithdrawArgument :one
UPDATE app.arguments
SET status = 'withdrawn',
    withdrawn_at = COALESCE(withdrawn_at, sqlc.arg(withdrawn_at)::timestamptz),
    updated_at = sqlc.arg(withdrawn_at)::timestamptz
WHERE id = sqlc.arg(argument_id)::uuid
  AND author_id = sqlc.arg(author_id)::uuid
  AND status = 'published'
RETURNING id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at, withdrawn_at;

-- ListArenaArgumentsPage returns one keyset page of published top-level
-- arguments of one relation in one Arena, newest first, with the derived
-- published-reply count computed in the same statement (no N+1). Withdrawn
-- and removed arguments never appear in public lists (P10-T07).
-- name: ListArenaArgumentsPage :many
SELECT
    a.id, a.arena_id, a.author_id, a.parent_id, a.relation, a.content,
    a.content_hash, a.grapheme_cost, a.status, a.created_at, a.updated_at, a.withdrawn_at,
    (SELECT count(*) FROM app.arguments reply WHERE reply.parent_id = a.id AND reply.status = 'published')::bigint AS reply_count
FROM app.arguments a
WHERE a.arena_id = sqlc.arg(arena_id)::uuid
  AND a.relation = sqlc.arg(relation)::text
  AND a.parent_id IS NULL
  AND a.status = 'published'
  AND (
      sqlc.arg(after_created_at)::timestamptz IS NULL
      OR (a.created_at, a.id) < (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(page_limit);

-- ListRepliesPage returns one keyset page of published replies of one
-- parent, newest first. Replies carry no derived reply count: the depth
-- policy forbids grandchildren (P10-T05), so the count is always zero and
-- the statement stays cheaper.
-- name: ListRepliesPage :many
SELECT
    a.id, a.arena_id, a.author_id, a.parent_id, a.relation, a.content,
    a.content_hash, a.grapheme_cost, a.status, a.created_at, a.updated_at, a.withdrawn_at
FROM app.arguments a
WHERE a.parent_id = sqlc.arg(parent_id)::uuid
  AND a.status = 'published'
  AND (
      sqlc.arg(after_created_at)::timestamptz IS NULL
      OR (a.created_at, a.id) < (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(page_limit);

-- GetPublicArgument resolves one argument for the public surface: published
-- and withdrawn (retracted) arguments resolve; moderation-removed arguments
-- are not found (P10-T07).
-- name: GetPublicArgument :one
SELECT
    a.id, a.arena_id, a.author_id, a.parent_id, a.relation, a.content,
    a.content_hash, a.grapheme_cost, a.status, a.created_at, a.updated_at, a.withdrawn_at,
    (SELECT count(*) FROM app.arguments reply WHERE reply.parent_id = a.id AND reply.status = 'published')::bigint AS reply_count
FROM app.arguments a
WHERE a.id = sqlc.arg(argument_id)::uuid
  AND a.status IN ('published', 'withdrawn');
