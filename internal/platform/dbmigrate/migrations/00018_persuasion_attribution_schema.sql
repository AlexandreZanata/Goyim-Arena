-- +goose Up
-- 00018 establishes the private attribution schema (P11-T01):
-- app.persuasion_attributions.
--
-- Invariants (docs/BUSINESS_RULES.md §5, §6):
-- 1. One attribution links exactly one position change to exactly one
--    argument; the pair is unique, so the same argument is credited at most
--    once per change.
-- 2. The attributor is the account that made the change: the composite
--    foreign key (position_change_id, attributor_id) makes a mismatched
--    attributor impossible, so the identity is coherent by construction.
-- 3. Status is the closed vocabulary valid|invalid. Moderation invalidates
--    and restores without ever deleting the row (historical fact), and
--    invalidated_at is coherent with the status: set on invalidation,
--    cleared on restoration (P11-T04).
-- 4. Links and provenance are immutable after insert; only status and
--    invalidated_at move. arena_app has no DELETE grant and a trigger
--    rejects DELETE for every role.
-- 5. The attributor identity is private: the table is never exposed through
--    a database view or a public projection; public metrics count without
--    identifiers (P11-T05/T06).

-- The composite foreign key below needs a unique target on the change.
ALTER TABLE app.position_changes
    ADD CONSTRAINT position_changes_id_account_unique UNIQUE (id, account_id);

CREATE TABLE IF NOT EXISTS app.persuasion_attributions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    position_change_id uuid NOT NULL,
    attributor_id uuid NOT NULL,
    argument_id uuid NOT NULL REFERENCES app.arguments(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'valid',
    created_at timestamptz NOT NULL DEFAULT now(),
    invalidated_at timestamptz,
    CONSTRAINT persuasion_attributions_change_fk FOREIGN KEY (position_change_id, attributor_id)
        REFERENCES app.position_changes (id, account_id) ON DELETE RESTRICT,
    CONSTRAINT persuasion_attributions_unique UNIQUE (position_change_id, argument_id),
    CONSTRAINT persuasion_attributions_status_check CHECK (status IN ('valid', 'invalid')),
    CONSTRAINT persuasion_attributions_invalidated_check CHECK (
        (status = 'invalid') = (invalidated_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS persuasion_attributions_argument_idx ON app.persuasion_attributions (argument_id, status);
CREATE INDEX IF NOT EXISTS persuasion_attributions_attributor_idx ON app.persuasion_attributions (attributor_id, created_at DESC);

COMMENT ON TABLE app.persuasion_attributions IS 'Private influence attributions: one position change credits one argument; the attributor identity is never part of a public projection';
COMMENT ON COLUMN app.persuasion_attributions.attributor_id IS 'Account that made the position change; coherent with the change through the composite foreign key and private by default';
COMMENT ON COLUMN app.persuasion_attributions.status IS 'valid or invalid (moderation/fraud); invalidation never deletes the historical row';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.persuasion_attributions_protect_links() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'attributions are historical facts and are never deleted'
            USING ERRCODE = '23514', CONSTRAINT = 'persuasion_attributions_retained';
    END IF;

    IF NEW.position_change_id IS DISTINCT FROM OLD.position_change_id
        OR NEW.attributor_id IS DISTINCT FROM OLD.attributor_id
        OR NEW.argument_id IS DISTINCT FROM OLD.argument_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'attribution links and provenance are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'persuasion_attributions_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS persuasion_attributions_protect_update ON app.persuasion_attributions;
CREATE TRIGGER persuasion_attributions_protect_update
    BEFORE UPDATE ON app.persuasion_attributions
    FOR EACH ROW EXECUTE FUNCTION app.persuasion_attributions_protect_links();

DROP TRIGGER IF EXISTS persuasion_attributions_retained_delete ON app.persuasion_attributions;
CREATE TRIGGER persuasion_attributions_retained_delete
    BEFORE DELETE ON app.persuasion_attributions
    FOR EACH ROW EXECUTE FUNCTION app.persuasion_attributions_protect_links();

ALTER TABLE app.persuasion_attributions OWNER TO arena_owner;
ALTER FUNCTION app.persuasion_attributions_protect_links() OWNER TO arena_owner;

-- Runtime surface: attributions are inserted and read; only the status
-- (invalidation/restoration) moves.
GRANT SELECT, INSERT, UPDATE ON app.persuasion_attributions TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS persuasion_attributions_retained_delete ON app.persuasion_attributions;
DROP TRIGGER IF EXISTS persuasion_attributions_protect_update ON app.persuasion_attributions;
DROP FUNCTION IF EXISTS app.persuasion_attributions_protect_links();
DROP TABLE IF EXISTS app.persuasion_attributions;
ALTER TABLE app.position_changes DROP CONSTRAINT IF EXISTS position_changes_id_account_unique;
