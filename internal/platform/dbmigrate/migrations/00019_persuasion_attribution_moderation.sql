-- +goose Up
-- 00019 makes attribution invalidation auditable (P11-T04, REQ-PERS-08,
-- BR §5.1): a moderator invalidates or restores one attribution with a
-- reason, and the decision record is preserved on the retained row.
--
-- Invariants:
-- 1. Nothing is deleted. Links, provenance and the creation instant stay
--    immutable and DELETE is rejected for every role (00018); this
--    migration only adds the administrative record of the decision.
-- 2. Every decision writes its record: reason, actor and instant move
--    together or not at all, and an invalid attribution without a decision
--    is unrepresentable. While the attribution is invalid its decision
--    instant is exactly the invalidation instant, so a transition to
--    invalid can never reuse an older decision record. A projection can
--    therefore never observe a validity change that lacks its own
--    administrative record.
-- 3. The record is preserved: once written the triple can never be cleared,
--    so restoring an attribution keeps the moderation history visible on
--    the row instead of silently rewriting it (MODERATION.md). Repeated
--    invalidate/restore cycles keep replacing the record with the newest
--    decision, never erase it.
-- 4. The platform-wide append-only trail (internal/audit, P14-T01) arrives
--    later; the persuasion application records every decision through its
--    audit port and the retained row already carries the auditable facts,
--    exactly like the wallet ledger records through the same port (00010).

ALTER TABLE app.persuasion_attributions
    ADD COLUMN IF NOT EXISTS moderation_reason text,
    ADD COLUMN IF NOT EXISTS moderated_by uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS moderated_at timestamptz;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'persuasion_attributions_decision_check'
    ) THEN
        ALTER TABLE app.persuasion_attributions
            ADD CONSTRAINT persuasion_attributions_decision_check
            CHECK (
                (moderation_reason IS NULL) = (moderated_by IS NULL)
                AND (moderation_reason IS NULL) = (moderated_at IS NULL)
                AND (
                    moderation_reason IS NULL
                    OR (trim(moderation_reason) <> '' AND char_length(moderation_reason) <= 500)
                )
                -- A never-moderated attribution is valid by construction:
                -- validity only ever moves through a recorded decision.
                AND (moderation_reason IS NOT NULL OR status = 'valid')
                -- The invalidation and its decision are the same instant.
                AND (status <> 'invalid' OR moderated_at = invalidated_at)
            );
    END IF;
END
$$;
-- +goose StatementEnd

COMMENT ON COLUMN app.persuasion_attributions.moderation_reason IS 'Mandatory justification of the latest moderation decision; never cleared, so the decision stays recorded without deleting the row';
COMMENT ON COLUMN app.persuasion_attributions.moderated_by IS 'Moderator account that took the latest decision (invalidate or restore)';
COMMENT ON COLUMN app.persuasion_attributions.moderated_at IS 'Instant of the latest moderation decision; coincides with invalidated_at while the attribution is invalid';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.persuasion_attributions_protect_decision() RETURNS trigger AS $$
BEGIN
    IF OLD.moderation_reason IS NOT NULL AND NEW.moderation_reason IS NULL THEN
        RAISE EXCEPTION 'the moderation decision record is preserved and cannot be erased'
            USING ERRCODE = '23514', CONSTRAINT = 'persuasion_attributions_decision_retained';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS persuasion_attributions_decision_retained ON app.persuasion_attributions;
CREATE TRIGGER persuasion_attributions_decision_retained
    BEFORE UPDATE ON app.persuasion_attributions
    FOR EACH ROW EXECUTE FUNCTION app.persuasion_attributions_protect_decision();

ALTER FUNCTION app.persuasion_attributions_protect_decision() OWNER TO arena_owner;

-- The runtime already holds SELECT, INSERT and UPDATE on the table (00018):
-- invalidation and restoration update status, invalidated_at and the
-- decision triple only, and DELETE stays impossible for every role.

-- +goose Down
DROP TRIGGER IF EXISTS persuasion_attributions_decision_retained ON app.persuasion_attributions;
DROP FUNCTION IF EXISTS app.persuasion_attributions_protect_decision();
ALTER TABLE app.persuasion_attributions DROP CONSTRAINT IF EXISTS persuasion_attributions_decision_check;
ALTER TABLE app.persuasion_attributions DROP COLUMN IF EXISTS moderated_at;
ALTER TABLE app.persuasion_attributions DROP COLUMN IF EXISTS moderated_by;
ALTER TABLE app.persuasion_attributions DROP COLUMN IF EXISTS moderation_reason;
