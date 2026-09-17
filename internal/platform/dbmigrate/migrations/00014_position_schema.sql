-- +goose Up
-- 00014 establishes the private position history (P09-T01):
-- app.debate_positions and app.position_changes.
--
-- Invariants (docs/BUSINESS_RULES.md §3, docs/MVP.md §3):
-- 1. Each participant has at most one initial position per Arena: the
--    primary key is (arena_id, account_id).
-- 2. The position vocabulary is closed in text with CHECK: agree,
--    disagree, undecided. No plan, payment or reputation weight exists in
--    the schema.
-- 3. debate_positions is the projection: initial_position is immutable as
--    a historical fact, current_position starts equal to it and moves only
--    by accepted changes, and version increments with each accepted change.
-- 4. position_changes is strictly append-only: arena_app receives SELECT
--    and INSERT only, and a defensive trigger rejects UPDATE and DELETE for
--    every role. from_position and to_position always differ.
-- 5. The change chain is derivable: it starts at initial_position (version
--    1) and each row records from_position, to_position and the resulting
--    version, unique per (Arena, account, version).
-- 6. Individual positions and their history are private data: the public
--    surface only ever sees aggregates derived without account identifiers
--    (P09-T05).
-- 7. Arena eligibility (published and open, eligible account) is a
--    use-case rule (P09-T03/T04), not a schema trigger: the schema keeps
--    the history complete and the use case refuses new entries.

CREATE TABLE IF NOT EXISTS app.debate_positions (
    arena_id uuid NOT NULL REFERENCES app.arenas(id) ON DELETE RESTRICT,
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    initial_position text NOT NULL,
    current_position text NOT NULL,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (arena_id, account_id),
    CONSTRAINT debate_positions_initial_check CHECK (initial_position IN ('agree', 'disagree', 'undecided')),
    CONSTRAINT debate_positions_current_check CHECK (current_position IN ('agree', 'disagree', 'undecided')),
    CONSTRAINT debate_positions_version_check CHECK (version >= 1)
);

CREATE INDEX IF NOT EXISTS debate_positions_account_idx ON app.debate_positions (account_id, created_at DESC);

COMMENT ON TABLE app.debate_positions IS 'Private projection of one account position in one Arena: immutable initial choice plus current choice and optimistic version';
COMMENT ON COLUMN app.debate_positions.initial_position IS 'Immutable first confirmed position; the history chain starts here at version 1';
COMMENT ON COLUMN app.debate_positions.current_position IS 'Derived projection of the chain tip; must equal the last accepted change target';
COMMENT ON COLUMN app.debate_positions.version IS 'Optimistic concurrency version: 1 at confirmation, +1 per accepted change';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.debate_positions_protect_initial() RETURNS trigger AS $$
BEGIN
    IF NEW.arena_id IS DISTINCT FROM OLD.arena_id
        OR NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.initial_position IS DISTINCT FROM OLD.initial_position
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'initial position and its provenance are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'debate_positions_initial_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS debate_positions_protect_initial_update ON app.debate_positions;
CREATE TRIGGER debate_positions_protect_initial_update
    BEFORE UPDATE ON app.debate_positions
    FOR EACH ROW EXECUTE FUNCTION app.debate_positions_protect_initial();

CREATE TABLE IF NOT EXISTS app.position_changes (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    arena_id uuid NOT NULL,
    account_id uuid NOT NULL,
    from_position text NOT NULL,
    to_position text NOT NULL,
    version integer NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT position_changes_from_check CHECK (from_position IN ('agree', 'disagree', 'undecided')),
    CONSTRAINT position_changes_to_check CHECK (to_position IN ('agree', 'disagree', 'undecided')),
    CONSTRAINT position_changes_distinct_check CHECK (from_position <> to_position),
    CONSTRAINT position_changes_version_check CHECK (version >= 2),
    CONSTRAINT position_changes_position_fk FOREIGN KEY (arena_id, account_id)
        REFERENCES app.debate_positions (arena_id, account_id) ON DELETE RESTRICT,
    CONSTRAINT position_changes_chain_unique UNIQUE (arena_id, account_id, version)
);

CREATE INDEX IF NOT EXISTS position_changes_account_idx ON app.position_changes (account_id, changed_at DESC);

COMMENT ON TABLE app.position_changes IS 'Append-only chain of accepted position changes; from_position and to_position always differ';
COMMENT ON COLUMN app.position_changes.version IS 'Resulting projection version after the change; the chain starts at 2';
COMMENT ON COLUMN app.position_changes.changed_at IS 'Instant of the accepted change; ordering authority is version';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.position_changes_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'position changes are append-only'
        USING ERRCODE = '23514', CONSTRAINT = 'position_changes_append_only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS position_changes_append_only_guard ON app.position_changes;
CREATE TRIGGER position_changes_append_only_guard
    BEFORE UPDATE OR DELETE ON app.position_changes
    FOR EACH ROW EXECUTE FUNCTION app.position_changes_append_only();

ALTER TABLE app.debate_positions OWNER TO arena_owner;
ALTER TABLE app.position_changes OWNER TO arena_owner;
ALTER FUNCTION app.debate_positions_protect_initial() OWNER TO arena_owner;
ALTER FUNCTION app.position_changes_append_only() OWNER TO arena_owner;

-- Runtime surface: positions are managed through the initial-immutability
-- trigger; changes are append-only at the privilege level.
GRANT SELECT, INSERT, UPDATE ON app.debate_positions TO arena_app;
GRANT SELECT, INSERT ON app.position_changes TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS position_changes_append_only_guard ON app.position_changes;
DROP TRIGGER IF EXISTS debate_positions_protect_initial_update ON app.debate_positions;
DROP FUNCTION IF EXISTS app.position_changes_append_only();
DROP FUNCTION IF EXISTS app.debate_positions_protect_initial();
DROP TABLE IF EXISTS app.position_changes;
DROP TABLE IF EXISTS app.debate_positions;
