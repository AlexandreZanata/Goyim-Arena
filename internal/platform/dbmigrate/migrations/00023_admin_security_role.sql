-- +goose Up
-- 00023 expands the administrative vocabulary with the security role
-- (P13-T02): moderator, admin and security share the same assignment table,
-- provenance and revocation rules established in 00022. No data moves: the
-- CHECK replacement only widens the accepted role set, and existing rows
-- stay valid.
--
-- The authorization policy itself lives in the moderation domain and
-- application layers (internal/moderation/{domain,application}): the schema
-- stores who holds which capability and when it was revoked, never who may
-- do what. Frontend or email based authorization is impossible by
-- construction: assignments key on account identifiers only.

ALTER TABLE app.admin_roles
    DROP CONSTRAINT IF EXISTS admin_roles_role_check;
ALTER TABLE app.admin_roles
    ADD CONSTRAINT admin_roles_role_check CHECK (role IN ('moderator', 'admin', 'security'));

COMMENT ON COLUMN app.admin_roles.role IS 'Administrative capability: moderator, admin or security; policy enforcement lives in the moderation application layer (P13-T02)';

-- +goose Down
ALTER TABLE app.admin_roles
    DROP CONSTRAINT IF EXISTS admin_roles_role_check;
ALTER TABLE app.admin_roles
    ADD CONSTRAINT admin_roles_role_check CHECK (role IN ('moderator', 'admin'));
