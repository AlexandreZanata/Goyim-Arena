-- +goose Up
-- 00031 adds rebuildable hot-read projections (P17-T02). The normalized
-- tables remain authoritative; these rows contain counts only, a projection
-- version and the latest source instant observed by the rebuild.

CREATE TABLE IF NOT EXISTS app.arena_public_stat_projections (
    arena_id uuid PRIMARY KEY REFERENCES app.arenas(id) ON DELETE CASCADE,
    projection_version integer NOT NULL DEFAULT 1,
    source_watermark timestamptz NOT NULL,
    stats jsonb NOT NULL,
    rebuilt_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT arena_public_stat_projection_version_check CHECK (projection_version >= 1),
    CONSTRAINT arena_public_stat_projection_stats_check CHECK (
        jsonb_typeof(stats) = 'object'
        AND octet_length(stats::text) <= 4096
    )
);

CREATE TABLE IF NOT EXISTS app.transparency_stat_projections (
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL,
    methodology_version integer NOT NULL DEFAULT 1,
    source_watermark timestamptz NOT NULL,
    stats jsonb NOT NULL,
    rebuilt_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (period_start, period_end),
    CONSTRAINT transparency_stat_projection_period_check CHECK (period_start < period_end),
    CONSTRAINT transparency_stat_projection_version_check CHECK (methodology_version >= 1),
    CONSTRAINT transparency_stat_projection_stats_check CHECK (
        jsonb_typeof(stats) = 'object'
        AND octet_length(stats::text) <= 8192
    )
);

CREATE INDEX IF NOT EXISTS arena_public_stat_projections_watermark_idx
    ON app.arena_public_stat_projections (source_watermark);
CREATE INDEX IF NOT EXISTS transparency_stat_projections_watermark_idx
    ON app.transparency_stat_projections (source_watermark);

COMMENT ON TABLE app.arena_public_stat_projections IS 'Rebuildable public Arena count projection; normalized Arena, position and attribution tables remain authoritative';
COMMENT ON COLUMN app.arena_public_stat_projections.source_watermark IS 'Latest source instant observed by the rebuild; it is not a wall-clock freshness claim';
COMMENT ON TABLE app.transparency_stat_projections IS 'Rebuildable period metrics projection; normalized source tables remain authoritative';
COMMENT ON COLUMN app.transparency_stat_projections.source_watermark IS 'Latest source instant observed by the period rebuild';

ALTER TABLE app.arena_public_stat_projections OWNER TO arena_owner;
ALTER TABLE app.transparency_stat_projections OWNER TO arena_owner;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.arena_public_stat_projections TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.transparency_stat_projections TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.transparency_stat_projections;
DROP TABLE IF EXISTS app.arena_public_stat_projections;
