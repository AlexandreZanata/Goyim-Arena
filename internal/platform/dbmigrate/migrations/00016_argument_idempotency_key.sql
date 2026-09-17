-- +goose Up
-- 00016 adds the per-attempt idempotency key to app.arguments (P10-T04):
-- publishing debits INK and inserts the argument in the same transaction,
-- so a retried attempt must resolve the recorded argument instead of
-- debiting again. The key is unique per author; old rows stay NULL because
-- the column is added after the fact (expand/contract).
ALTER TABLE app.arguments ADD COLUMN IF NOT EXISTS idempotency_key text;

CREATE UNIQUE INDEX IF NOT EXISTS arguments_author_idempotency_unique
    ON app.arguments (author_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE app.arguments ADD CONSTRAINT arguments_idempotency_key_check CHECK (
    idempotency_key IS NULL
    OR (char_length(idempotency_key) BETWEEN 1 AND 128 AND idempotency_key ~ '^[!-~]+$')
);

COMMENT ON COLUMN app.arguments.idempotency_key IS 'Client attempt key, unique per author: a retry resolves the recorded argument instead of debiting INK again';

-- +goose Down
ALTER TABLE app.arguments DROP CONSTRAINT IF EXISTS arguments_idempotency_key_check;
DROP INDEX IF EXISTS arguments_author_idempotency_unique;
ALTER TABLE app.arguments DROP COLUMN IF EXISTS idempotency_key;
