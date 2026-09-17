-- +goose Up
-- 00017 records the author withdrawal instant (P10-T06): withdrawing moves
-- the argument out of the public display without erasing the historical
-- content, and the fact must stay auditable even if moderation later turns
-- the status into removed. The immutability trigger already allows this
-- column because it only protects content, cost and provenance.
ALTER TABLE app.arguments ADD COLUMN IF NOT EXISTS withdrawn_at timestamptz;

ALTER TABLE app.arguments ADD CONSTRAINT arguments_withdrawn_at_check CHECK (
    withdrawn_at IS NULL OR status IN ('withdrawn', 'removed')
);

COMMENT ON COLUMN app.arguments.withdrawn_at IS 'Instant of the author withdrawal; kept as the audit fact of the retraction, never a deletion';

-- +goose Down
ALTER TABLE app.arguments DROP CONSTRAINT IF EXISTS arguments_withdrawn_at_check;
ALTER TABLE app.arguments DROP COLUMN IF EXISTS withdrawn_at;
