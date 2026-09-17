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
RETURNING id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at;

-- GetArgumentByAuthorAndKey resolves the argument recorded under one
-- attempt key (P10-T04).
-- name: GetArgumentByAuthorAndKey :one
SELECT id, arena_id, author_id, parent_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at
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
    a.content_hash, a.grapheme_cost, a.status, a.created_at, a.updated_at,
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
