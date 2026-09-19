-- +goose Up
-- 00032 adds public language-aware full-text indexes for P17-T03.
-- PostgreSQL remains the only search engine: source rows stay authoritative,
-- and moderation status is enforced by partial-index predicates and queries.

CREATE INDEX IF NOT EXISTS arenas_public_search_pt_fts_idx
    ON app.arenas USING gin (
        to_tsvector('portuguese'::regconfig, statement || ' ' || COALESCE(context, ''))
    )
    WHERE language = 'pt-BR' AND status IN ('published', 'closed', 'restricted');

CREATE INDEX IF NOT EXISTS arenas_public_search_en_fts_idx
    ON app.arenas USING gin (
        to_tsvector('english'::regconfig, statement || ' ' || COALESCE(context, ''))
    )
    WHERE language = 'en-US' AND status IN ('published', 'closed', 'restricted');

CREATE INDEX IF NOT EXISTS arguments_public_search_pt_fts_idx
    ON app.arguments USING gin (to_tsvector('portuguese'::regconfig, content))
    WHERE status = 'published';

CREATE INDEX IF NOT EXISTS arguments_public_search_en_fts_idx
    ON app.arguments USING gin (to_tsvector('english'::regconfig, content))
    WHERE status = 'published';

COMMENT ON INDEX app.arenas_public_search_pt_fts_idx IS 'Portuguese public Arena search; drafts and removed Arenas are excluded';
COMMENT ON INDEX app.arenas_public_search_en_fts_idx IS 'English public Arena search; drafts and removed Arenas are excluded';
COMMENT ON INDEX app.arguments_public_search_pt_fts_idx IS 'Portuguese published-argument search; withdrawn and removed arguments are excluded';
COMMENT ON INDEX app.arguments_public_search_en_fts_idx IS 'English published-argument search; withdrawn and removed arguments are excluded';

-- +goose Down
DROP INDEX IF EXISTS app.arguments_public_search_en_fts_idx;
DROP INDEX IF EXISTS app.arguments_public_search_pt_fts_idx;
DROP INDEX IF EXISTS app.arenas_public_search_en_fts_idx;
DROP INDEX IF EXISTS app.arenas_public_search_pt_fts_idx;
