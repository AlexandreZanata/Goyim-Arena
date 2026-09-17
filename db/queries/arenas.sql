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

-- name: GetArenaByID :one
SELECT id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at
FROM app.arenas
WHERE id = $1;

-- CloseArena performs the published→closed transition requested by the
-- creator under the optimistic version check. Reopening does not exist in
-- the MVP (P08-T05).
-- name: CloseArena :one
UPDATE app.arenas
SET status = 'closed',
    version = version + 1
WHERE id = $1 AND creator_id = $2 AND status = 'published' AND version = $3
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- RestrictArena applies the moderation restriction to a published or closed
-- Arena under the optimistic version check (P08-T05).
-- name: RestrictArena :one
UPDATE app.arenas
SET status = 'restricted',
    version = version + 1
WHERE id = $1 AND status IN ('published', 'closed') AND version = $2
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- RemoveArena applies the moderation removal to a published, closed or
-- restricted Arena under the optimistic version check; removed is terminal.
-- name: RemoveArena :one
UPDATE app.arenas
SET status = 'removed',
    version = version + 1
WHERE id = $1 AND status IN ('published', 'closed', 'restricted') AND version = $2
RETURNING id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at;

-- name: GetArenaStateByID :one
SELECT status, version
FROM app.arenas
WHERE id = $1;

-- ListPublicArenasPage returns one keyset page of the public feed, newest
-- first, with optional language, category and status filters. Only publicly
-- visible statuses are ever candidates: drafts and removed Arenas can never
-- appear, and the (published_at, id) tuple comparison never duplicates or
-- skips rows (P08-T06).
-- name: ListPublicArenasPage :many
SELECT id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at
FROM app.arenas
WHERE status IN ('published', 'closed', 'restricted')
  AND (sqlc.narg(language_filter)::text IS NULL OR language = sqlc.narg(language_filter)::text)
  AND (sqlc.narg(category_filter)::text IS NULL OR category = sqlc.narg(category_filter)::text)
  AND (sqlc.narg(status_filter)::text IS NULL OR status = sqlc.narg(status_filter)::text)
  AND (
      sqlc.arg(after_published_at)::timestamptz IS NULL
      OR (published_at, id) < (sqlc.arg(after_published_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY published_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- GetPublicArenaBySlug resolves a public Arena address. Drafts are never
-- addressable and removed Arenas are not found for the public (P08-T07).
-- name: GetPublicArenaBySlug :one
SELECT id, creator_id, slug, statement, context, category, language, status, version, created_at, published_at, closes_at
FROM app.arenas
WHERE slug = $1 AND status IN ('published', 'closed', 'restricted');

-- GetArenaStatusBySlug reports the stored status of the Arena holding the
-- slug, including removed, so the SEO document endpoint can answer 410 for
-- removed Arenas instead of 404 (P08-T08). Drafts never hold a slug.
-- name: GetArenaStatusBySlug :one
SELECT status
FROM app.arenas
WHERE slug = $1;
