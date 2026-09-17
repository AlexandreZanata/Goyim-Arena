-- +goose Up
-- 00007 adds the optional IANA timezone to app.profiles (P05-T06).
--
-- Invariants:
-- 1. Timezone is an independent preference: it is validated only when
--    informed (domain validates the IANA name against the tz database).
--    NULL means "not informed" and the product falls back to UTC; an empty
--    string is never stored.
-- 2. The interface locale stays owned by app.profiles.interface_locale and
--    this migration does not touch it; content_language belongs to Arena
--    content and remains independent from both.
-- 3. The schema only enforces cheap structural bounds (non-empty, bounded
--    length); semantic IANA validation lives in the profiles domain, which
--    is the only layer allowed to load the timezone database.

ALTER TABLE app.profiles ADD COLUMN IF NOT EXISTS timezone text;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'profiles_timezone_check'
    ) THEN
        ALTER TABLE app.profiles
            ADD CONSTRAINT profiles_timezone_check
            CHECK (
                timezone IS NULL
                OR (trim(timezone) <> '' AND char_length(timezone) <= 64)
            );
    END IF;
END
$$;
-- +goose StatementEnd

COMMENT ON COLUMN app.profiles.timezone IS 'Optional IANA timezone preference of the account; NULL means not informed (UTC fallback)';

-- +goose Down
ALTER TABLE app.profiles DROP CONSTRAINT IF EXISTS profiles_timezone_check;
ALTER TABLE app.profiles DROP COLUMN IF EXISTS timezone;
