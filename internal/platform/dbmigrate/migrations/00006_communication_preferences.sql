-- +goose Up
-- 00006 establishes explicit communication preferences (P05-T05):
-- app.communication_preferences for the current opt-in state and
-- app.communication_preference_history as the append-only audit trail.
--
-- Invariants:
-- 1. One preferences row per account. Absence of a row means the
--    conservative defaults: marketing opt-in is false. Opt-in is never
--    implicit — only an explicit owner action can set it to true.
-- 2. Marketing communication is strictly opt-in; the column defaults to
--    false and every change is auditable.
-- 3. Essential/transactional messages (security, account, billing) are not
--    optional in the MVP and therefore are not represented here.
-- 4. Preferences never carry email, payment identifiers, antifraud flags or
--    administrative notes; the locale stays owned by app.profiles and is
--    joined read-only by the preferences query.
-- 5. Least privilege: arena_owner owns the objects; arena_app gets DML.

CREATE TABLE IF NOT EXISTS app.communication_preferences (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE CASCADE,
    marketing_opt_in boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE app.communication_preferences IS 'Explicit communication opt-ins per account; marketing defaults to false and is never opted in implicitly';
COMMENT ON COLUMN app.communication_preferences.marketing_opt_in IS 'Explicit marketing consent; false by default and only the account owner may change it';

CREATE TABLE IF NOT EXISTS app.communication_preference_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    marketing_opt_in boolean NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS communication_preference_history_account_id_idx ON app.communication_preference_history (account_id, changed_at DESC);

COMMENT ON TABLE app.communication_preference_history IS 'Append-only audit trail of every explicit communication preference change';

ALTER TABLE app.communication_preferences OWNER TO arena_owner;
ALTER TABLE app.communication_preference_history OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE, DELETE ON app.communication_preferences TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.communication_preference_history TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.communication_preference_history;
DROP TABLE IF EXISTS app.communication_preferences;
