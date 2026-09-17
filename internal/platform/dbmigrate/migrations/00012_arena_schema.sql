-- +goose Up
-- 00012 establishes the Arena schema (P08-T01): app.categories, app.arenas
-- and app.arena_relations.
--
-- Invariants (docs/BUSINESS_RULES.md §2, docs/MVP.md §3):
-- 1. An Arena carries creator, slug, statement, optional context, category,
--    fixed language, status, created/published/closes instants and an
--    optimistic version.
-- 2. The language is fixed per Arena and restricted to the MVP content
--    languages: pt-BR and en-US. Interface locale stays independent.
-- 3. Status is a closed vocabulary in text with CHECK: draft, published,
--    closed, restricted, removed. Drafts are the only rows without slug and
--    published_at; only non-drafts can have closes_at, and it must be after
--    publication.
-- 4. Statement and language are immutable after publication, enforced by a
--    defensive trigger rather than convention. Provenance (creator, slug,
--    published_at, created_at) is equally protected against retroactive
--    rewrites; status, context, category, closes_at and version remain
--    mutable for lifecycle transitions.
-- 5. Published arenas are never deleted: the trigger only allows deleting
--    drafts. Content retention is a product rule.
-- 6. Continuations/substitutions live in app.arena_relations, linking two
--    arenas with a stable kind and never to themselves.
-- 7. Categories are an editorial reference table: seeded conservatively in
--    this migration and extensible by new migrations, with no runtime DDL.
-- 8. Least privilege: arena_owner owns the objects; arena_app reads
--    categories, fully manages arenas through the trigger guard and may add
--    or remove relations.

CREATE TABLE IF NOT EXISTS app.categories (
    slug text PRIMARY KEY,
    display_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT categories_slug_format_check CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])$'),
    CONSTRAINT categories_display_order_check CHECK (display_order >= 0)
);

COMMENT ON TABLE app.categories IS 'Editorial Arena categories; seeded by migrations, display names resolve through the versioned i18n catalogs';

INSERT INTO app.categories (slug, display_order) VALUES
    ('technology', 1),
    ('science', 2),
    ('philosophy', 3),
    ('politics', 4),
    ('economics', 5),
    ('health', 6),
    ('culture', 7),
    ('society', 8)
ON CONFLICT (slug) DO NOTHING;

CREATE TABLE IF NOT EXISTS app.arenas (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    creator_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    slug text,
    statement text NOT NULL,
    context text,
    category text NOT NULL REFERENCES app.categories(slug) ON DELETE RESTRICT,
    language text NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    closes_at timestamptz,
    CONSTRAINT arenas_slug_unique UNIQUE (slug),
    CONSTRAINT arenas_slug_format_check CHECK (slug IS NULL OR slug ~ '^[a-z0-9]([a-z0-9-]{1,78}[a-z0-9])$'),
    CONSTRAINT arenas_statement_check CHECK (trim(statement) <> '' AND char_length(statement) <= 2000),
    CONSTRAINT arenas_context_check CHECK (context IS NULL OR (trim(context) <> '' AND char_length(context) <= 10000)),
    CONSTRAINT arenas_language_check CHECK (language IN ('pt-BR', 'en-US')),
    CONSTRAINT arenas_status_check CHECK (status IN ('draft', 'published', 'closed', 'restricted', 'removed')),
    CONSTRAINT arenas_version_check CHECK (version > 0),
    CONSTRAINT arenas_status_dates_check CHECK (
        (status = 'draft') = (published_at IS NULL)
        AND (status = 'draft') = (slug IS NULL)
        AND (closes_at IS NULL OR published_at IS NOT NULL)
        AND (closes_at IS NULL OR closes_at > published_at)
    )
);

CREATE INDEX IF NOT EXISTS arenas_feed_idx ON app.arenas (language, category, status, published_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS arenas_creator_idx ON app.arenas (creator_id, status, created_at DESC);

COMMENT ON TABLE app.arenas IS 'Arena aggregation root: immutable published statement and fixed content language, mutable lifecycle status';
COMMENT ON COLUMN app.arenas.slug IS 'Stable public address assigned at publication; never an identity substitute';
COMMENT ON COLUMN app.arenas.language IS 'Immutable content language of the Arena (pt-BR or en-US); independent from the interface locale';
COMMENT ON COLUMN app.arenas.version IS 'Optimistic concurrency version, incremented on every accepted mutation';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.arenas_protect_published() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'draft' THEN
            RAISE EXCEPTION 'only draft arenas can be deleted'
                USING ERRCODE = '23514', CONSTRAINT = 'arenas_published_not_deletable';
        END IF;
        RETURN OLD;
    END IF;

    IF OLD.status <> 'draft' AND (
        NEW.statement IS DISTINCT FROM OLD.statement
        OR NEW.language IS DISTINCT FROM OLD.language
        OR NEW.creator_id IS DISTINCT FROM OLD.creator_id
        OR NEW.slug IS DISTINCT FROM OLD.slug
        OR NEW.published_at IS DISTINCT FROM OLD.published_at
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    ) THEN
        RAISE EXCEPTION 'published arena statement, language and provenance are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'arenas_published_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS arenas_protect_published_update ON app.arenas;
CREATE TRIGGER arenas_protect_published_update
    BEFORE UPDATE ON app.arenas
    FOR EACH ROW EXECUTE FUNCTION app.arenas_protect_published();

DROP TRIGGER IF EXISTS arenas_protect_published_delete ON app.arenas;
CREATE TRIGGER arenas_protect_published_delete
    BEFORE DELETE ON app.arenas
    FOR EACH ROW EXECUTE FUNCTION app.arenas_protect_published();

CREATE TABLE IF NOT EXISTS app.arena_relations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    arena_id uuid NOT NULL REFERENCES app.arenas(id) ON DELETE RESTRICT,
    related_arena_id uuid NOT NULL REFERENCES app.arenas(id) ON DELETE RESTRICT,
    relation text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT arena_relations_kind_check CHECK (relation IN ('SUPERSEDES', 'CONTINUES')),
    CONSTRAINT arena_relations_not_self_check CHECK (arena_id <> related_arena_id),
    CONSTRAINT arena_relations_unique UNIQUE (arena_id, related_arena_id)
);

CREATE INDEX IF NOT EXISTS arena_relations_related_idx ON app.arena_relations (related_arena_id);

COMMENT ON TABLE app.arena_relations IS 'Substitution/continuation notes linking one Arena to another (never to itself)';

ALTER TABLE app.categories OWNER TO arena_owner;
ALTER TABLE app.arenas OWNER TO arena_owner;
ALTER TABLE app.arena_relations OWNER TO arena_owner;
ALTER FUNCTION app.arenas_protect_published() OWNER TO arena_owner;

-- Runtime surface: categories are read-only; arenas are managed through the
-- publication immutability trigger; relations are added or removed whole.
GRANT SELECT ON app.categories TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.arenas TO arena_app;
GRANT SELECT, INSERT, DELETE ON app.arena_relations TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.arena_relations;
DROP TRIGGER IF EXISTS arenas_protect_published_delete ON app.arenas;
DROP TRIGGER IF EXISTS arenas_protect_published_update ON app.arenas;
DROP FUNCTION IF EXISTS app.arenas_protect_published();
DROP TABLE IF EXISTS app.arenas;
DROP TABLE IF EXISTS app.categories;
