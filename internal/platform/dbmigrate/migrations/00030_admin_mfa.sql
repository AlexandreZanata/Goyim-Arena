-- +goose Up
-- 00030 adds the second factor of the administrative surface (P16-T05).
--
-- Three facts live here and nowhere else:
--
--   * the sealed shared secret of one account, with the step that was last
--     accepted. Keeping the step in the row is what makes a code single-use in
--     time: the check and the advance happen in one statement, so two
--     concurrent verifications of the same code cannot both win;
--   * the one-time backup codes, stored as hashes. A code is shown once, at
--     enrollment, and never again: the row is the ability to spend it, and
--     spending it is an UPDATE of used_at that only succeeds while it is NULL;
--   * whether a session presented the factor, which is what the step-up rule
--     of the administrative gate reads. It lives on the session because it is
--     a property of the session, not of the account: a session that has not
--     presented it is not administrative, whatever the account holds.
--
-- The secret is sealed with AES-256-GCM under a key that is not in this
-- database, and the account identifier is the additional authenticated data:
-- a sealed value copied from another row does not open. The schema stores
-- ciphertext, so a dump, a replica or a backup does not contain a usable
-- second factor. The mechanism is in internal/platform/mfa (ADR-014).

CREATE TABLE IF NOT EXISTS app.account_mfa (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE CASCADE,
    secret_sealed bytea NOT NULL,
    confirmed_at timestamptz,
    last_accepted_step bigint NOT NULL DEFAULT -1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_mfa_sealed_not_empty CHECK (octet_length(secret_sealed) > 0),
    -- -1 means "no step accepted yet", which is the state of a pending
    -- enrollment. Anything below it would mean a step before the epoch.
    CONSTRAINT account_mfa_step_check CHECK (last_accepted_step >= -1),
    -- A confirmed enrollment always has an accepted step: the code that
    -- confirmed it is spent by definition.
    CONSTRAINT account_mfa_confirmed_step_check CHECK (confirmed_at IS NULL OR last_accepted_step >= 0)
);

COMMENT ON TABLE app.account_mfa IS 'Second factor of one account: sealed TOTP secret and the last accepted time step (P16-T05)';
COMMENT ON COLUMN app.account_mfa.secret_sealed IS 'AES-256-GCM ciphertext (nonce || sealed secret) bound to the account identifier as AAD; no plaintext secret is ever stored';
COMMENT ON COLUMN app.account_mfa.last_accepted_step IS 'Highest accepted TOTP time step; a code at or below it is refused as a replay';

CREATE TABLE IF NOT EXISTS app.mfa_backup_codes (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
    code_hash text NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mfa_backup_codes_hash_check CHECK (btrim(code_hash) <> '' AND char_length(code_hash) <= 300)
);

COMMENT ON TABLE app.mfa_backup_codes IS 'One-time recovery codes of an MFA enrollment, stored hashed and spent exactly once (P16-T05)';
COMMENT ON COLUMN app.mfa_backup_codes.used_at IS 'Set by the single UPDATE that spends the code; NULL means the code is still usable';

CREATE INDEX IF NOT EXISTS mfa_backup_codes_account_idx
    ON app.mfa_backup_codes (account_id)
    WHERE used_at IS NULL;

ALTER TABLE app.sessions ADD COLUMN IF NOT EXISTS mfa_verified_at timestamptz;

COMMENT ON COLUMN app.sessions.mfa_verified_at IS 'Instant the session presented a second factor; NULL means the session is not MFA-elevated (P16-T05)';

ALTER TABLE app.account_mfa OWNER TO arena_owner;
ALTER TABLE app.mfa_backup_codes OWNER TO arena_owner;

-- Runtime surface: the runtime reads and writes both tables. DELETE is granted
-- on the backup codes because replacing a set is how a re-enrollment and a
-- spent code are cleaned up; the enrollment row itself is never deleted by the
-- runtime, it is cascaded with the account.
GRANT SELECT, INSERT, UPDATE ON app.account_mfa TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.mfa_backup_codes TO arena_app;

-- +goose Down
ALTER TABLE app.sessions DROP COLUMN IF EXISTS mfa_verified_at;
DROP INDEX IF EXISTS app.mfa_backup_codes_account_idx;
DROP TABLE IF EXISTS app.mfa_backup_codes;
DROP TABLE IF EXISTS app.account_mfa;
