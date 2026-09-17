-- +goose Up
-- 00015 establishes the argument schema (P10-T02): app.arguments and
-- app.argument_sources.
--
-- Invariants (docs/BUSINESS_RULES.md §4, docs/MVP.md §3):
-- 1. An argument belongs to exactly one Arena and one author, and declares
--    its relation to the statement: support, oppose or context (a favor,
--    contra ou contextual).
-- 2. A reply is an argument with parent_id. The composite foreign key
--    (parent_id, arena_id) makes a cross-Arena parent impossible, and an
--    argument can never be its own parent.
-- 3. Published content is immutable: content, content_hash, grapheme_cost,
--    relation, provenance (Arena, author, parent, created_at) are protected
--    by a defensive trigger. Only the withdrawal/moderation status and
--    updated_at move after publication (BUSINESS_RULES §4.1: no silent
--    edits, retention of the historical fact).
-- 4. Arguments are never deleted in the MVP: withdrawal removes them from
--    display without erasing the internal history. arena_app has no DELETE
--    grant and a trigger rejects DELETE for every role.
-- 5. The grapheme cost is the billing unit (1 INK per cluster,
--    REQ-WAL-02): it is a positive integer up to the 3.000-cluster limit.
--    The exact Unicode count is computed by the domain (ADR-013); the
--    schema enforces the range and a non-empty plaintext.
-- 6. Sources support a claim without certifying it: each source carries a
--    URL and an optional short description, unique per argument.
-- 7. Least privilege: arena_owner owns the objects; arena_app inserts and
--    reads arguments, moves only the status, and inserts/reads sources.

CREATE TABLE IF NOT EXISTS app.arguments (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    arena_id uuid NOT NULL REFERENCES app.arenas(id) ON DELETE RESTRICT,
    author_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    parent_id uuid,
    relation text NOT NULL,
    content text NOT NULL,
    content_hash text NOT NULL,
    grapheme_cost integer NOT NULL,
    status text NOT NULL DEFAULT 'published',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT arguments_id_arena_unique UNIQUE (id, arena_id),
    CONSTRAINT arguments_parent_same_arena_fk FOREIGN KEY (parent_id, arena_id)
        REFERENCES app.arguments (id, arena_id) ON DELETE RESTRICT,
    CONSTRAINT arguments_not_self_parent_check CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT arguments_relation_check CHECK (relation IN ('support', 'oppose', 'context')),
    CONSTRAINT arguments_content_check CHECK (btrim(content) <> '' AND octet_length(content) <= 131072),
    CONSTRAINT arguments_content_hash_check CHECK (char_length(content_hash) BETWEEN 8 AND 128),
    CONSTRAINT arguments_grapheme_cost_check CHECK (grapheme_cost >= 1 AND grapheme_cost <= 3000),
    CONSTRAINT arguments_status_check CHECK (status IN ('published', 'withdrawn', 'removed'))
);

CREATE INDEX IF NOT EXISTS arguments_arena_relation_idx ON app.arguments (arena_id, relation, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS arguments_parent_idx ON app.arguments (parent_id, created_at DESC, id DESC) WHERE parent_id IS NOT NULL;

COMMENT ON TABLE app.arguments IS 'Public immutable argument or reply of one Arena: plaintext, canonical content hash, grapheme cost and withdrawal/moderation status';
COMMENT ON COLUMN app.arguments.relation IS 'Declared relation to the Arena statement: support, oppose or context';
COMMENT ON COLUMN app.arguments.content IS 'Plaintext content as published; never edited after publication';
COMMENT ON COLUMN app.arguments.content_hash IS 'Versioned canonical hash of the content (format owned by the domain)';
COMMENT ON COLUMN app.arguments.grapheme_cost IS 'Grapheme cluster count: the 1 INK per cluster billing unit, at most 3000';
COMMENT ON COLUMN app.arguments.status IS 'published, withdrawn (author) or removed (moderation); transitions are use-case rules';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.arguments_protect_published() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'arguments are retained; withdrawal never erases the historical fact'
            USING ERRCODE = '23514', CONSTRAINT = 'arguments_retained';
    END IF;

    IF NEW.arena_id IS DISTINCT FROM OLD.arena_id
        OR NEW.author_id IS DISTINCT FROM OLD.author_id
        OR NEW.parent_id IS DISTINCT FROM OLD.parent_id
        OR NEW.relation IS DISTINCT FROM OLD.relation
        OR NEW.content IS DISTINCT FROM OLD.content
        OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
        OR NEW.grapheme_cost IS DISTINCT FROM OLD.grapheme_cost
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'published argument content, cost and provenance are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'arguments_published_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS arguments_protect_published_update ON app.arguments;
CREATE TRIGGER arguments_protect_published_update
    BEFORE UPDATE ON app.arguments
    FOR EACH ROW EXECUTE FUNCTION app.arguments_protect_published();

DROP TRIGGER IF EXISTS arguments_retained_delete ON app.arguments;
CREATE TRIGGER arguments_retained_delete
    BEFORE DELETE ON app.arguments
    FOR EACH ROW EXECUTE FUNCTION app.arguments_protect_published();

CREATE TABLE IF NOT EXISTS app.argument_sources (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    argument_id uuid NOT NULL REFERENCES app.arguments(id) ON DELETE RESTRICT,
    url text NOT NULL,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT argument_sources_unique UNIQUE (argument_id, url),
    CONSTRAINT argument_sources_url_check CHECK (
        char_length(url) BETWEEN 8 AND 2048
        AND url ~ '^https?://[^[:space:]]+$'
    ),
    CONSTRAINT argument_sources_description_check CHECK (description IS NULL OR char_length(description) <= 500)
);

CREATE INDEX IF NOT EXISTS argument_sources_argument_idx ON app.argument_sources (argument_id);

COMMENT ON TABLE app.argument_sources IS 'Structured sources supporting an argument: URL plus optional short description; a source never certifies truth';
COMMENT ON COLUMN app.argument_sources.url IS 'Absolute http(s) URL, bounded and free of whitespace';

ALTER TABLE app.arguments OWNER TO arena_owner;
ALTER TABLE app.argument_sources OWNER TO arena_owner;
ALTER FUNCTION app.arguments_protect_published() OWNER TO arena_owner;

-- Runtime surface: arguments are inserted and read; only the status moves.
-- Sources are inserted and read whole.
GRANT SELECT, INSERT, UPDATE ON app.arguments TO arena_app;
GRANT SELECT, INSERT ON app.argument_sources TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS arguments_retained_delete ON app.arguments;
DROP TRIGGER IF EXISTS arguments_protect_published_update ON app.arguments;
DROP FUNCTION IF EXISTS app.arguments_protect_published();
DROP TABLE IF EXISTS app.argument_sources;
DROP TABLE IF EXISTS app.arguments;
