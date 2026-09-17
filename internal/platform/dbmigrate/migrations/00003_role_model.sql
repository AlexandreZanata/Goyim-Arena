-- +goose Up
-- 00003 establishes the least-privilege role model of Goyim Arena
-- (P03-T03) inside a single forward migration, so the database ships its
-- security policy atomically with the schema it protects. Full role
-- documentation lives in docs/ROLE_MODEL.md.
--
-- Role model:
--   arena_owner     owns every object; DDL authority; NOLOGIN. The
--                   bootstrap superuser relinquishes ownership after the
--                   schema is provisioned and never serves application
--                   traffic.
--   arena_migrator  runs the forward-only migration workflow; membership
--                   in arena_owner (SET role) is transient and exits with
--                   the migration session.
--   arena_app       the runtime of the application: data DML inside the
--                   app schema only. It cannot create or alter schema,
--                   cannot touch the migration history and is never a
--                   superuser.
--
-- Credentials are managed by the operator (ALTER ROLE ... PASSWORD), or
-- through scripts/dev-role-cycle.sh; passwords are never hardcoded here.
--
-- Idempotency: DO blocks guard the CREATE ROLE statements so a database
-- restored from a snapshot that already has the roles re-runs safely.
-- goose needs StatementBegin/StatementEnd around the multi-line DO blocks
-- so the dollar-quoted bodies parse as single statements.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_owner') THEN
        CREATE ROLE arena_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
    -- arena_app is the ONLY login role the migration creates: the runtime
    -- authenticates with credentials the operator manages (scripts/
    -- dev-role-cycle.sh in development, the secret store in production).
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_app') THEN
        CREATE ROLE arena_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_migrator') THEN
        CREATE ROLE arena_migrator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION
            IN ROLE arena_owner;
    END IF;
END
$$;
-- +goose StatementEnd

-- Ownership of the existing objects moves from the bootstrap superuser to
-- the NOLOGIN owner; membership grants migrator its transient DDL powers.
ALTER SCHEMA app OWNER TO arena_owner;
ALTER TABLE app.schema_metadata OWNER TO arena_owner;

GRANT USAGE ON SCHEMA app TO arena_app;
GRANT SELECT ON app.schema_metadata TO arena_app;

-- +goose Down
-- The Down section restores the pre-00003 shape: ownership back to the
-- bootstrap role and the role model removed. It is not part of the
-- production path (migrations are forward-only); it exists so a migration
-- can be tested cleanly on a scratch database.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_migrator') THEN
        DROP ROLE arena_migrator;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_app') THEN
        DROP ROLE arena_app;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'arena_owner') THEN
        DROP ROLE arena_owner;
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE app.schema_metadata OWNER TO current_user;
ALTER SCHEMA app OWNER TO current_user;
