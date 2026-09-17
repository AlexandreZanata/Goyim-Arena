-- Arena draft queries for the PostgreSQL platform adapter.
--
-- Drafts are private: every query is scoped by the creator and public
-- projections never select a draft. Creating or editing a draft never
-- touches the Arena Pass ledger (P08-T03).

-- name: CreateArena :one
INSERT INTO app.arenas (creator_id, statement, context, category, language)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- name: GetArenaForCreator :one
SELECT id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at
FROM app.arenas
WHERE id = $1 AND creator_id = $2;

-- name: ListArenaDraftsForCreator :many
SELECT id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at
FROM app.arenas
WHERE creator_id = $1 AND status = 'draft'
ORDER BY created_at DESC, id DESC;

-- UpdateArenaDraft replaces the mutable draft fields under an optimistic
-- version check: a stale expected version affects no row (P08-T03).
-- name: UpdateArenaDraft :one
UPDATE app.arenas
SET statement = $3,
    context = $4,
    category = $5,
    language = $6,
    version = version + 1
WHERE id = $1 AND creator_id = $2 AND version = $7 AND status = 'draft'
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- name: DeleteArenaDraft :execrows
DELETE FROM app.arenas
WHERE id = $1 AND creator_id = $2 AND status = 'draft';

-- PublishArenaDraft performs the draft→published transition under the
-- optimistic version check inside the publication transaction, so the Arena
-- row and the consumed Arena Pass commit together (P08-T04).
-- name: PublishArenaDraft :one
UPDATE app.arenas
SET status = 'published',
    slug = $3,
    published_at = $4,
    version = version + 1
WHERE id = $1 AND creator_id = $2 AND status = 'draft' AND version = $5
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- GetArenaStateForCreator diagnoses why a scoped draft write affected no
-- row: missing or foreign arena, non-draft status, or stale version.
-- name: GetArenaStateForCreator :one
SELECT status, version
FROM app.arenas
WHERE id = $1 AND creator_id = $2;
