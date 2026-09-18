-- +goose Up
-- 00022 establishes the moderation case schema (P13-T01):
-- app.admin_roles, app.moderation_reports, app.moderation_cases,
-- app.moderation_actions and app.moderation_appeals.
--
-- Invariants (docs/MODERATION.md, REQ-MOD-01/02/03/04, REQ-AUD-01,
-- REQ-INV-09):
-- 1. Reports are restricted evidence: the reporter, the structured reason
--    and the optional context live only here, never in a public projection.
--    A report names exactly one target (arena, argument or profile) through
--    three nullable references guarded by a CHECK, so a report can never
--    point at two targets or at none, and the type always matches the
--    reference that is set.
-- 2. Cases aggregate one target at a time with an explicit lifecycle
--    (open -> under_review -> decided -> closed) enforced by a trigger.
--    Target identity is immutable; only triage fields move. Reports and
--    cases stay independent in this migration: triage linkage arrives with
--    the report use case (P13-T03/T04), so no premature coupling is baked
--    into the schema.
-- 3. Actions are immutable facts of a case: actor, applied rule, restricted
--    justification and optional expiry move together or not at all.
--    Suspensions and interaction limits require a future expiry; every
--    other action forbids one. No UPDATE is accepted after insert, so a
--    sanction can never be rewritten: reversal happens through a new action
--    or an appeal outcome (P13-T05/T06), never by editing history.
-- 4. Appeals are restricted and singular: exactly one appeal per action
--    (UNIQUE on action_id). The appellant, the contested action and the
--    context are immutable; only the review outcome (reviewer, reason and
--    decided instant) may be appended once, through the legal status
--    transitions (open -> under_review -> upheld/modified/reversed).
-- 5. Administrative roles are minimal (moderator, admin) and explicit: one
--    row per account, granted by an existing account, revocable with a
--    dated revocation. Identity (account, grantor, granted instant) is
--    immutable; only the role and the revocation move.
-- 6. Restricted content is separated from public views by construction:
--    reporter context, internal justifications and appeal contexts live in
--    these tables only; no view, export or public projection is created
--    here, and later read models must project explicitly allowed columns.
-- 7. History is retained: every table references accounts and content with
--    ON DELETE RESTRICT and DELETE is rejected for every role by a
--    defensive trigger, not merely by the privilege grant (REQ-INV-09).
-- 8. Least privilege: arena_owner owns every object; arena_app may read
--    and insert everywhere, update only cases, appeals and role rows, and
--    never delete anything.

CREATE TABLE IF NOT EXISTS app.admin_roles (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE RESTRICT,
    role text NOT NULL,
    granted_by uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    granted_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CONSTRAINT admin_roles_role_check CHECK (role IN ('moderator', 'admin')),
    CONSTRAINT admin_roles_revoked_check CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);

COMMENT ON TABLE app.admin_roles IS 'Minimal administrative assignments: one row per account, granted by an existing account, revocable with a dated revocation';
COMMENT ON COLUMN app.admin_roles.role IS 'Administrative capability: moderator or admin; policy enforcement arrives with P13-T02';
COMMENT ON COLUMN app.admin_roles.granted_by IS 'Acting account that granted the role; immutable provenance of the assignment';
COMMENT ON COLUMN app.admin_roles.revoked_at IS 'Instant the role was revoked, when it was; NULL while the assignment is active';

CREATE TABLE IF NOT EXISTS app.moderation_reports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    reporter_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    target_type text NOT NULL,
    target_arena_id uuid REFERENCES app.arenas(id) ON DELETE RESTRICT,
    target_argument_id uuid REFERENCES app.arguments(id) ON DELETE RESTRICT,
    target_account_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    reason text NOT NULL,
    context text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT moderation_reports_target_check CHECK (target_type IN ('arena', 'argument', 'profile')),
    CONSTRAINT moderation_reports_target_ref_check CHECK (
        (target_type = 'arena' AND target_arena_id IS NOT NULL AND target_argument_id IS NULL AND target_account_id IS NULL)
        OR (target_type = 'argument' AND target_arena_id IS NULL AND target_argument_id IS NOT NULL AND target_account_id IS NULL)
        OR (target_type = 'profile' AND target_arena_id IS NULL AND target_argument_id IS NULL AND target_account_id IS NOT NULL)
    ),
    CONSTRAINT moderation_reports_reason_check CHECK (reason IN (
        'violence', 'doxxing', 'harassment', 'sexual', 'fraud',
        'spam', 'illegal', 'evasion', 'multiaccount', 'malware', 'other'
    )),
    CONSTRAINT moderation_reports_context_check CHECK (
        context IS NULL OR (btrim(context) <> '' AND char_length(context) <= 2000)
    )
);

CREATE INDEX IF NOT EXISTS moderation_reports_reporter_idx
    ON app.moderation_reports (reporter_id, created_at DESC);
CREATE INDEX IF NOT EXISTS moderation_reports_target_arena_idx
    ON app.moderation_reports (target_arena_id, created_at DESC)
    WHERE target_arena_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS moderation_reports_target_argument_idx
    ON app.moderation_reports (target_argument_id, created_at DESC)
    WHERE target_argument_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS moderation_reports_target_account_idx
    ON app.moderation_reports (target_account_id, created_at DESC)
    WHERE target_account_id IS NOT NULL;

COMMENT ON TABLE app.moderation_reports IS 'Restricted report evidence: reporter, structured reason and optional context; never part of a public projection';
COMMENT ON COLUMN app.moderation_reports.reason IS 'Closed reason vocabulary from MODERATION §3; free text lives only in context';
COMMENT ON COLUMN app.moderation_reports.context IS 'Optional reporter context up to 2000 chars; restricted evidence, never logged or exported';

CREATE TABLE IF NOT EXISTS app.moderation_cases (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    target_type text NOT NULL,
    target_arena_id uuid REFERENCES app.arenas(id) ON DELETE RESTRICT,
    target_argument_id uuid REFERENCES app.arguments(id) ON DELETE RESTRICT,
    target_account_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'open',
    priority text NOT NULL DEFAULT 'normal',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    CONSTRAINT moderation_cases_target_check CHECK (target_type IN ('arena', 'argument', 'profile')),
    CONSTRAINT moderation_cases_target_ref_check CHECK (
        (target_type = 'arena' AND target_arena_id IS NOT NULL AND target_argument_id IS NULL AND target_account_id IS NULL)
        OR (target_type = 'argument' AND target_arena_id IS NULL AND target_argument_id IS NOT NULL AND target_account_id IS NULL)
        OR (target_type = 'profile' AND target_arena_id IS NULL AND target_argument_id IS NULL AND target_account_id IS NOT NULL)
    ),
    CONSTRAINT moderation_cases_status_check CHECK (
        status IN ('open', 'under_review', 'decided', 'closed')
    ),
    CONSTRAINT moderation_cases_priority_check CHECK (
        priority IN ('low', 'normal', 'high', 'urgent')
    ),
    CONSTRAINT moderation_cases_closed_check CHECK (
        (status = 'closed') = (closed_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS moderation_cases_queue_idx
    ON app.moderation_cases (status, priority, created_at)
    WHERE status IN ('open', 'under_review');
CREATE INDEX IF NOT EXISTS moderation_cases_target_arena_idx
    ON app.moderation_cases (target_arena_id, created_at DESC)
    WHERE target_arena_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS moderation_cases_target_argument_idx
    ON app.moderation_cases (target_argument_id, created_at DESC)
    WHERE target_argument_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS moderation_cases_target_account_idx
    ON app.moderation_cases (target_account_id, created_at DESC)
    WHERE target_account_id IS NOT NULL;

COMMENT ON TABLE app.moderation_cases IS 'Moderation case per target with an explicit triage lifecycle; reporter evidence stays in reports, decisions stay in actions';
COMMENT ON COLUMN app.moderation_cases.status IS 'Triage lifecycle: open -> under_review -> decided -> closed; enforced by trigger';
COMMENT ON COLUMN app.moderation_cases.priority IS 'Triage priority: low, normal, high or urgent; mutable without moving the lifecycle';

CREATE TABLE IF NOT EXISTS app.moderation_actions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    case_id uuid NOT NULL REFERENCES app.moderation_cases(id) ON DELETE RESTRICT,
    action_type text NOT NULL,
    actor_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    rule_applied text NOT NULL,
    justification text NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT moderation_actions_type_check CHECK (action_type IN (
        'no_action', 'warning', 'link_hide', 'interaction_limit',
        'argument_remove', 'arena_close', 'attribution_invalidate',
        'position_invalidate', 'suspension', 'ban', 'preserve_legal'
    )),
    CONSTRAINT moderation_actions_rule_check CHECK (
        btrim(rule_applied) <> '' AND char_length(rule_applied) <= 200
    ),
    CONSTRAINT moderation_actions_justification_check CHECK (
        btrim(justification) <> '' AND char_length(justification) <= 2000
    ),
    CONSTRAINT moderation_actions_expiry_check CHECK (
        ((action_type IN ('suspension', 'interaction_limit')) = (expires_at IS NOT NULL))
        AND (expires_at IS NULL OR expires_at > created_at)
    )
);

CREATE INDEX IF NOT EXISTS moderation_actions_case_idx
    ON app.moderation_actions (case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS moderation_actions_actor_idx
    ON app.moderation_actions (actor_id, created_at DESC);

COMMENT ON TABLE app.moderation_actions IS 'Immutable sanction facts of a case: actor, applied rule, restricted justification and optional expiry; reversals are new rows, never edits';
COMMENT ON COLUMN app.moderation_actions.rule_applied IS 'Stable rule reference applied by the moderator (for example MOD-3:spam); policy neutrality is tested in P13-T08';
COMMENT ON COLUMN app.moderation_actions.justification IS 'Restricted internal justification up to 2000 chars; never part of a public projection';
COMMENT ON COLUMN app.moderation_actions.expires_at IS 'Expiry of time-boxed measures (suspension, interaction_limit); required exactly for those, forbidden for the rest';

CREATE TABLE IF NOT EXISTS app.moderation_appeals (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    action_id uuid NOT NULL REFERENCES app.moderation_actions(id) ON DELETE RESTRICT,
    appellant_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'open',
    context text NOT NULL,
    reviewer_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    decision_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    CONSTRAINT moderation_appeals_action_unique UNIQUE (action_id),
    CONSTRAINT moderation_appeals_status_check CHECK (
        status IN ('open', 'under_review', 'upheld', 'modified', 'reversed')
    ),
    CONSTRAINT moderation_appeals_context_check CHECK (
        btrim(context) <> '' AND char_length(context) <= 2000
    ),
    CONSTRAINT moderation_appeals_decision_check CHECK (
        ((status IN ('upheld', 'modified', 'reversed')) = (decided_at IS NOT NULL))
        AND ((status IN ('upheld', 'modified', 'reversed')) = (reviewer_id IS NOT NULL))
        AND ((status IN ('upheld', 'modified', 'reversed')) = (decision_reason IS NOT NULL))
    ),
    CONSTRAINT moderation_appeals_decision_reason_check CHECK (
        decision_reason IS NULL OR (btrim(decision_reason) <> '' AND char_length(decision_reason) <= 2000)
    )
);

CREATE INDEX IF NOT EXISTS moderation_appeals_appellant_idx
    ON app.moderation_appeals (appellant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS moderation_appeals_open_idx
    ON app.moderation_appeals (created_at)
    WHERE status IN ('open', 'under_review');

COMMENT ON TABLE app.moderation_appeals IS 'Restricted appeal per action: exactly one appeal contests one action; review outcome is appended once by a reviewer';
COMMENT ON COLUMN app.moderation_appeals.action_id IS 'Contested action; UNIQUE enforces one appeal per action when applicable';
COMMENT ON COLUMN app.moderation_appeals.context IS 'Appellant context up to 2000 chars; restricted evidence, never part of a public projection';
COMMENT ON COLUMN app.moderation_appeals.decision_reason IS 'Reviewer justification written once when the appeal is decided; the rest of the record is immutable';

-- +goose StatementBegin
-- Retention: moderation history is evidence of administrative power, so no
-- row of this module is ever deleted, by any role.
CREATE OR REPLACE FUNCTION app.moderation_retain_rows() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'moderation rows are retained and are never deleted (table %)', TG_TABLE_NAME
        USING ERRCODE = '23514', CONSTRAINT = 'moderation_rows_retained';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Role identity is provenance: the account, the grantor and the granted
-- instant never change. Only the capability and its revocation move.
CREATE OR REPLACE FUNCTION app.admin_roles_protect_assignment() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.granted_by IS DISTINCT FROM OLD.granted_by
        OR NEW.granted_at IS DISTINCT FROM OLD.granted_at
    THEN
        RAISE EXCEPTION 'the identity of an administrative assignment is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'admin_roles_identity_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Reports are restricted evidence: once written, reporter, target, reason,
-- context and instant never change.
CREATE OR REPLACE FUNCTION app.moderation_reports_protect_evidence() RETURNS trigger AS $$
BEGIN
    IF NEW.reporter_id IS DISTINCT FROM OLD.reporter_id
        OR NEW.target_type IS DISTINCT FROM OLD.target_type
        OR NEW.target_arena_id IS DISTINCT FROM OLD.target_arena_id
        OR NEW.target_argument_id IS DISTINCT FROM OLD.target_argument_id
        OR NEW.target_account_id IS DISTINCT FROM OLD.target_account_id
        OR NEW.reason IS DISTINCT FROM OLD.reason
        OR NEW.context IS DISTINCT FROM OLD.context
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'report evidence is immutable once written'
            USING ERRCODE = '23514', CONSTRAINT = 'moderation_reports_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Cases keep their target forever and move through triage in order. Priority
-- and timestamps move freely; the lifecycle never jumps or revives a closed
-- case.
CREATE OR REPLACE FUNCTION app.moderation_cases_protect_lifecycle() RETURNS trigger AS $$
BEGIN
    IF NEW.target_type IS DISTINCT FROM OLD.target_type
        OR NEW.target_arena_id IS DISTINCT FROM OLD.target_arena_id
        OR NEW.target_argument_id IS DISTINCT FROM OLD.target_argument_id
        OR NEW.target_account_id IS DISTINCT FROM OLD.target_account_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'the target of a moderation case is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'moderation_cases_target_immutable';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NOT (
            (OLD.status = 'open' AND NEW.status = 'under_review')
            OR (OLD.status = 'under_review' AND NEW.status = 'decided')
            OR (OLD.status = 'decided' AND NEW.status = 'closed')
        ) THEN
            RAISE EXCEPTION 'illegal moderation case transition % -> %', OLD.status, NEW.status
                USING ERRCODE = '23514', CONSTRAINT = 'moderation_cases_status_transition';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Actions are facts: nothing about a sanction moves after insert. Reversal
-- is a new action or an appeal outcome, never an edit.
CREATE OR REPLACE FUNCTION app.moderation_actions_freeze() RETURNS trigger AS $$
BEGIN
    IF NEW.case_id IS DISTINCT FROM OLD.case_id
        OR NEW.action_type IS DISTINCT FROM OLD.action_type
        OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
        OR NEW.rule_applied IS DISTINCT FROM OLD.rule_applied
        OR NEW.justification IS DISTINCT FROM OLD.justification
        OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'moderation actions are immutable; reverse with a new action or appeal outcome'
            USING ERRCODE = '23514', CONSTRAINT = 'moderation_actions_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Appeals keep their contested action, appellant and context forever; only
-- the review outcome is appended once, through the legal transitions, and a
-- decided appeal is final.
CREATE OR REPLACE FUNCTION app.moderation_appeals_protect() RETURNS trigger AS $$
BEGIN
    IF NEW.action_id IS DISTINCT FROM OLD.action_id
        OR NEW.appellant_id IS DISTINCT FROM OLD.appellant_id
        OR NEW.context IS DISTINCT FROM OLD.context
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'an appeal contests one action with its context preserved'
            USING ERRCODE = '23514', CONSTRAINT = 'moderation_appeals_immutable';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NOT (
            (OLD.status = 'open' AND NEW.status = 'under_review')
            OR (OLD.status = 'under_review' AND NEW.status IN ('upheld', 'modified', 'reversed'))
        ) THEN
            RAISE EXCEPTION 'illegal appeal transition % -> %', OLD.status, NEW.status
                USING ERRCODE = '23514', CONSTRAINT = 'moderation_appeals_status_transition';
        END IF;
    END IF;

    IF OLD.decided_at IS NOT NULL AND (
        NEW.decided_at IS DISTINCT FROM OLD.decided_at
        OR NEW.reviewer_id IS DISTINCT FROM OLD.reviewer_id
        OR NEW.decision_reason IS DISTINCT FROM OLD.decision_reason
    ) THEN
        RAISE EXCEPTION 'the resolution of an appeal is final'
            USING ERRCODE = '23514', CONSTRAINT = 'moderation_appeals_resolution_final';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS admin_roles_retained_delete ON app.admin_roles;
CREATE TRIGGER admin_roles_retained_delete
    BEFORE DELETE ON app.admin_roles
    FOR EACH ROW EXECUTE FUNCTION app.moderation_retain_rows();

DROP TRIGGER IF EXISTS moderation_reports_retained_delete ON app.moderation_reports;
CREATE TRIGGER moderation_reports_retained_delete
    BEFORE DELETE ON app.moderation_reports
    FOR EACH ROW EXECUTE FUNCTION app.moderation_retain_rows();

DROP TRIGGER IF EXISTS moderation_cases_retained_delete ON app.moderation_cases;
CREATE TRIGGER moderation_cases_retained_delete
    BEFORE DELETE ON app.moderation_cases
    FOR EACH ROW EXECUTE FUNCTION app.moderation_retain_rows();

DROP TRIGGER IF EXISTS moderation_actions_retained_delete ON app.moderation_actions;
CREATE TRIGGER moderation_actions_retained_delete
    BEFORE DELETE ON app.moderation_actions
    FOR EACH ROW EXECUTE FUNCTION app.moderation_retain_rows();

DROP TRIGGER IF EXISTS moderation_appeals_retained_delete ON app.moderation_appeals;
CREATE TRIGGER moderation_appeals_retained_delete
    BEFORE DELETE ON app.moderation_appeals
    FOR EACH ROW EXECUTE FUNCTION app.moderation_retain_rows();

DROP TRIGGER IF EXISTS admin_roles_protect_assignment ON app.admin_roles;
CREATE TRIGGER admin_roles_protect_assignment
    BEFORE UPDATE ON app.admin_roles
    FOR EACH ROW EXECUTE FUNCTION app.admin_roles_protect_assignment();

DROP TRIGGER IF EXISTS moderation_reports_protect_evidence ON app.moderation_reports;
CREATE TRIGGER moderation_reports_protect_evidence
    BEFORE UPDATE ON app.moderation_reports
    FOR EACH ROW EXECUTE FUNCTION app.moderation_reports_protect_evidence();

DROP TRIGGER IF EXISTS moderation_cases_protect_lifecycle ON app.moderation_cases;
CREATE TRIGGER moderation_cases_protect_lifecycle
    BEFORE UPDATE ON app.moderation_cases
    FOR EACH ROW EXECUTE FUNCTION app.moderation_cases_protect_lifecycle();

DROP TRIGGER IF EXISTS moderation_actions_freeze ON app.moderation_actions;
CREATE TRIGGER moderation_actions_freeze
    BEFORE UPDATE ON app.moderation_actions
    FOR EACH ROW EXECUTE FUNCTION app.moderation_actions_freeze();

DROP TRIGGER IF EXISTS moderation_appeals_protect ON app.moderation_appeals;
CREATE TRIGGER moderation_appeals_protect
    BEFORE UPDATE ON app.moderation_appeals
    FOR EACH ROW EXECUTE FUNCTION app.moderation_appeals_protect();

ALTER TABLE app.admin_roles OWNER TO arena_owner;
ALTER TABLE app.moderation_reports OWNER TO arena_owner;
ALTER TABLE app.moderation_cases OWNER TO arena_owner;
ALTER TABLE app.moderation_actions OWNER TO arena_owner;
ALTER TABLE app.moderation_appeals OWNER TO arena_owner;

ALTER FUNCTION app.moderation_retain_rows() OWNER TO arena_owner;
ALTER FUNCTION app.admin_roles_protect_assignment() OWNER TO arena_owner;
ALTER FUNCTION app.moderation_reports_protect_evidence() OWNER TO arena_owner;
ALTER FUNCTION app.moderation_cases_protect_lifecycle() OWNER TO arena_owner;
ALTER FUNCTION app.moderation_actions_freeze() OWNER TO arena_owner;
ALTER FUNCTION app.moderation_appeals_protect() OWNER TO arena_owner;

-- Runtime surface: reports and actions are append-only facts
-- (SELECT/INSERT); cases, appeals and role rows move only through their
-- protected lifecycles (SELECT/INSERT/UPDATE). DELETE is not granted
-- anywhere: retention is enforced by the trigger too.
GRANT SELECT, INSERT, UPDATE ON app.admin_roles TO arena_app;
GRANT SELECT, INSERT ON app.moderation_reports TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.moderation_cases TO arena_app;
GRANT SELECT, INSERT ON app.moderation_actions TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.moderation_appeals TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS moderation_appeals_protect ON app.moderation_appeals;
DROP TRIGGER IF EXISTS moderation_actions_freeze ON app.moderation_actions;
DROP TRIGGER IF EXISTS moderation_cases_protect_lifecycle ON app.moderation_cases;
DROP TRIGGER IF EXISTS moderation_reports_protect_evidence ON app.moderation_reports;
DROP TRIGGER IF EXISTS admin_roles_protect_assignment ON app.admin_roles;
DROP TRIGGER IF EXISTS moderation_appeals_retained_delete ON app.moderation_appeals;
DROP TRIGGER IF EXISTS moderation_actions_retained_delete ON app.moderation_actions;
DROP TRIGGER IF EXISTS moderation_cases_retained_delete ON app.moderation_cases;
DROP TRIGGER IF EXISTS moderation_reports_retained_delete ON app.moderation_reports;
DROP TRIGGER IF EXISTS admin_roles_retained_delete ON app.admin_roles;
DROP FUNCTION IF EXISTS app.moderation_appeals_protect();
DROP FUNCTION IF EXISTS app.moderation_actions_freeze();
DROP FUNCTION IF EXISTS app.moderation_cases_protect_lifecycle();
DROP FUNCTION IF EXISTS app.moderation_reports_protect_evidence();
DROP FUNCTION IF EXISTS app.admin_roles_protect_assignment();
DROP FUNCTION IF EXISTS app.moderation_retain_rows();
DROP TABLE IF EXISTS app.moderation_appeals;
DROP TABLE IF EXISTS app.moderation_actions;
DROP TABLE IF EXISTS app.moderation_cases;
DROP TABLE IF EXISTS app.moderation_reports;
DROP TABLE IF EXISTS app.admin_roles;
