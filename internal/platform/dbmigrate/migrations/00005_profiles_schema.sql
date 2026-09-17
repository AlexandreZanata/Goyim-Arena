-- +goose Up
-- 00005 establishes the public profile schema (P05-T01): app.profiles and
-- app.username_history, with the minimal locale preference required by
-- docs/MVP.md ("username público" and "locale").
--
-- Invariants:
-- 1. One profile per account: account_id is the primary key, so an account
--    can never accumulate duplicate profiles.
-- 2. Usernames are unique case-insensitively through the canonical
--    username_normalized column (ASCII lowercase), which must always equal
--    lower(username); username itself is presentation-only.
-- 3. Format: 3 to 30 characters, ASCII letters/digits plus inner '-'/'_',
--    starting and ending alphanumeric. Reserved-term policy is not schema:
--    it is enforced by the profiles domain (P05-T02).
-- 4. interface_locale only accepts the MVP BCP 47 locales: pt-BR and en-US
--    (docs/I18N_STANDARD is the product-level source; the default matches
--    the product default locale).
-- 5. Email, credentials and payment-provider identifiers never live in or
--    are selected from profile tables (docs/PRIVACY.md): they stay in
--    app.accounts and in the billing module respectively.
-- 6. Every username creation or change is auditable in
--    app.username_history.
-- 7. Least privilege: arena_owner owns the objects; arena_app gets DML.

CREATE TABLE IF NOT EXISTS app.profiles (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE CASCADE,
    username text NOT NULL,
    username_normalized text NOT NULL,
    interface_locale text NOT NULL DEFAULT 'pt-BR',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT profiles_username_format_check CHECK (username ~ '^[A-Za-z0-9]([A-Za-z0-9_-]{1,28}[A-Za-z0-9])$'),
    CONSTRAINT profiles_username_normalized_format_check CHECK (username_normalized ~ '^[a-z0-9]([a-z0-9_-]{1,28}[a-z0-9])$'),
    CONSTRAINT profiles_username_normalized_matches_check CHECK (username_normalized = lower(username)),
    CONSTRAINT profiles_username_normalized_unique UNIQUE (username_normalized),
    CONSTRAINT profiles_interface_locale_check CHECK (interface_locale IN ('pt-BR', 'en-US'))
);

COMMENT ON TABLE app.profiles IS 'Account profiles: public username and interface locale, without email or payment identifiers';
COMMENT ON COLUMN app.profiles.username_normalized IS 'Canonical ASCII-lowercase username; sole uniqueness/authority key for lookups';

CREATE TABLE IF NOT EXISTS app.username_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    username text NOT NULL,
    username_normalized text NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT username_history_username_format_check CHECK (username ~ '^[A-Za-z0-9]([A-Za-z0-9_-]{1,28}[A-Za-z0-9])$'),
    CONSTRAINT username_history_username_normalized_format_check CHECK (username_normalized ~ '^[a-z0-9]([a-z0-9_-]{1,28}[a-z0-9])$'),
    CONSTRAINT username_history_username_normalized_matches_check CHECK (username_normalized = lower(username))
);

CREATE INDEX IF NOT EXISTS username_history_account_id_idx ON app.username_history (account_id, changed_at DESC);

COMMENT ON TABLE app.username_history IS 'Audit trail of every username set or changed by an account (P05-T02 requires auditable history)';

ALTER TABLE app.profiles OWNER TO arena_owner;
ALTER TABLE app.username_history OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE, DELETE ON app.profiles TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.username_history TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.username_history;
DROP TABLE IF EXISTS app.profiles;
