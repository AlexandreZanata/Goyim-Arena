-- +goose Up
-- 00002 documents the schema metadata table in the catalog itself: the
-- version table is part of the public surface of the database and every
-- operator reading the catalog should see what it tracks.
COMMENT ON TABLE app.schema_metadata IS 'goose forward-only migration history for the app schema (schema_metadata version table required by the master plan)';

-- +goose Down
COMMENT ON TABLE app.schema_metadata IS NULL;
