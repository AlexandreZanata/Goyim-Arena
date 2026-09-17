-- +goose Up
-- 00013 adds the partial index of the unfiltered public Arena feed
-- (P08-T06). arenas_feed_idx (migration 00012) serves feeds filtered by
-- language (and optionally category and status) because those columns lead
-- the index; the unfiltered newest-first feed needs its own partial index
-- over the publicly visible statuses, so keyset pagination never falls back
-- to a full scan plus sort on volume.
--
-- Drafts are not in the partial predicate: they are never published and can
-- never appear in a public feed. Removed Arenas are excluded as well.

CREATE INDEX IF NOT EXISTS arenas_public_feed_idx
    ON app.arenas (published_at DESC, id DESC)
    WHERE status IN ('published', 'closed', 'restricted');

COMMENT ON INDEX app.arenas_public_feed_idx IS 'Keyset order of the unfiltered public feed (public statuses only)';

-- +goose Down
DROP INDEX IF EXISTS app.arenas_public_feed_idx;
