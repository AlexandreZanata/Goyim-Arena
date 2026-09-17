-- +goose Up
-- 00004 establishes the core identity and authentication schema (P04-T01):
-- accounts, password_credentials, email_verification_tokens,
-- password_reset_tokens, and sessions.
--
-- Security invariants:
-- 1. Normalized email has case-insensitive uniqueness (lower(email)).
-- 2. Credentials live in password_credentials, separated from accounts,
--    so profile and account queries never read or leak password hashes.
-- 3. Tokens and sessions store exclusively cryptographic hashes (SHA-256 bytea).
-- 4. Sessions track created, expires, last_seen and revoked timestamps,
--    with indexes dedicated to active lookup and cleanup queries.
-- 5. Least-privilege role model: objects are owned by arena_owner and
--    arena_app is granted DML (SELECT, INSERT, UPDATE, DELETE).

CREATE TABLE IF NOT EXISTS app.accounts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_email_not_empty CHECK (trim(email) <> ''),
    CONSTRAINT accounts_status_check CHECK (status IN ('pending', 'active', 'suspended', 'deleted'))
);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_email_lower_idx ON app.accounts (lower(email));
CREATE INDEX IF NOT EXISTS accounts_status_idx ON app.accounts (status);

COMMENT ON TABLE app.accounts IS 'User account records and account status lifecycle';
COMMENT ON COLUMN app.accounts.email IS 'User email address; uniqueness enforced case-insensitively via lower(email)';

CREATE TABLE IF NOT EXISTS app.password_credentials (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE CASCADE,
    password_hash text NOT NULL,
    algorithm text NOT NULL DEFAULT 'argon2id',
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT password_credentials_hash_not_empty CHECK (trim(password_hash) <> '')
);

COMMENT ON TABLE app.password_credentials IS 'Hashed password credentials isolated from public account and profile data';

CREATE TABLE IF NOT EXISTS app.email_verification_tokens (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT email_verification_tokens_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT email_verification_tokens_used_at_check CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX IF NOT EXISTS email_verification_tokens_account_id_idx ON app.email_verification_tokens (account_id);
CREATE INDEX IF NOT EXISTS email_verification_tokens_expires_at_idx ON app.email_verification_tokens (expires_at);

COMMENT ON TABLE app.email_verification_tokens IS 'Single-use cryptographic token hashes for email verification';

CREATE TABLE IF NOT EXISTS app.password_reset_tokens (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT password_reset_tokens_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT password_reset_tokens_used_at_check CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX IF NOT EXISTS password_reset_tokens_account_id_idx ON app.password_reset_tokens (account_id);
CREATE INDEX IF NOT EXISTS password_reset_tokens_expires_at_idx ON app.password_reset_tokens (expires_at);

COMMENT ON TABLE app.password_reset_tokens IS 'Single-use cryptographic token hashes for password recovery';

CREATE TABLE IF NOT EXISTS app.sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    ip_address text,
    user_agent text,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT sessions_revoked_at_check CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX IF NOT EXISTS sessions_account_id_idx ON app.sessions (account_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON app.sessions (expires_at);
CREATE INDEX IF NOT EXISTS sessions_cleanup_idx ON app.sessions (expires_at, revoked_at);

COMMENT ON TABLE app.sessions IS 'Opaque server-side session store with revocation and inactivity tracking';

ALTER TABLE app.accounts OWNER TO arena_owner;
ALTER TABLE app.password_credentials OWNER TO arena_owner;
ALTER TABLE app.email_verification_tokens OWNER TO arena_owner;
ALTER TABLE app.password_reset_tokens OWNER TO arena_owner;
ALTER TABLE app.sessions OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE, DELETE ON app.accounts TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.password_credentials TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.email_verification_tokens TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.password_reset_tokens TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.sessions TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.sessions;
DROP TABLE IF EXISTS app.password_reset_tokens;
DROP TABLE IF EXISTS app.email_verification_tokens;
DROP TABLE IF EXISTS app.password_credentials;
DROP TABLE IF EXISTS app.accounts;
