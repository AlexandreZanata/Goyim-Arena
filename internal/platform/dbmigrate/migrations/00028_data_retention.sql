-- +goose Up
-- 00028 establishes the data retention policy (P14-T07, docs/PRIVACY.md
-- §1/§5, REQ-PRIV-01).
--
-- The policy itself is executable code (internal/profiles/domain and
-- internal/profiles/application): one schedule per governed class decides
-- whether the job purges, anonymizes or retains, and after how long. This
-- migration only stores the two facts the job needs: which holds are in
-- force and what each run did.
--
-- Invariants:
-- 1. The retention ledger records one row per governed class and
--    execution instant: how many records were purged, how many had their
--    restricted references anonymized, how many were retained under an
--    obligation and how many were preserved by an active legal hold. It
--    carries counts, the class and instants only: never content, never a
--    subject identifier, never a payload.
-- 2. The ledger is append-only evidence: the runtime inserts and reads,
--    UPDATE and DELETE are rejected by trigger for every role, and the
--    same (class, instant) is recorded once, so a replayed run cannot
--    duplicate or rewrite history.
-- 3. Cutoff semantics: a purge or anonymize run always dates the boundary
--    it applied; a retained class has no boundary (nothing is ever
--    removed), so cutoff_at is NULL exactly for the classes the policy
--    marks as retained.
-- 4. A legal hold names one class and either one account or the whole
--    class. At most one active hold exists per (class, account) and per
--    (class, whole class); while it is active the class purges and
--    anonymizes nothing for the held rows.
-- 5. Holds are provenance: the class, the account, the reason, the
--    grantor and the placement instant never change; releasing is
--    write-once and never erases the hold.
-- 6. Least privilege: arena_app reads, inserts and updates holds (to
--    release them) and reads and inserts the ledger.

CREATE TABLE IF NOT EXISTS app.retention_holds (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    data_class text NOT NULL,
    account_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    reason_code text NOT NULL,
    placed_by uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    placed_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    release_reason_code text,
    CONSTRAINT retention_holds_class_check CHECK (data_class IN (
        'tokens', 'sessions', 'referential_logs', 'exports', 'abuse_signals', 'billing'
    )),
    CONSTRAINT retention_holds_reason_check CHECK (
        btrim(reason_code) <> '' AND char_length(reason_code) <= 100
    ),
    CONSTRAINT retention_holds_release_check CHECK (
        (released_at IS NULL) = (release_reason_code IS NULL)
    ),
    CONSTRAINT retention_holds_release_reason_check CHECK (
        release_reason_code IS NULL OR (
            btrim(release_reason_code) <> '' AND char_length(release_reason_code) <= 100
        )
    ),
    CONSTRAINT retention_holds_release_order_check CHECK (
        released_at IS NULL OR released_at >= placed_at
    )
);

-- One active hold per class and subject: the class-wide hold maps the
-- absent account to the nil identifier, because unique indexes treat NULLs
-- as distinct and would otherwise allow parallel whole-class holds.
CREATE UNIQUE INDEX IF NOT EXISTS retention_holds_active_unique
    ON app.retention_holds (
        data_class,
        COALESCE(account_id, '00000000-0000-0000-0000-000000000000'::uuid)
    )
    WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS retention_holds_class_idx
    ON app.retention_holds (data_class, placed_at DESC);

COMMENT ON TABLE app.retention_holds IS 'Legal and contractual holds: while a hold is active the retention job purges and anonymizes nothing for the held class or account; holds are never deleted';
COMMENT ON COLUMN app.retention_holds.data_class IS 'Governed retention class, the same closed vocabulary the executable policy declares';
COMMENT ON COLUMN app.retention_holds.account_id IS 'Held account; NULL holds the whole class';
COMMENT ON COLUMN app.retention_holds.reason_code IS 'Stable reason code of the hold (never free prose)';
COMMENT ON COLUMN app.retention_holds.placed_by IS 'Administrative account that placed the hold; retained as provenance';
COMMENT ON COLUMN app.retention_holds.release_reason_code IS 'Stable reason code written once when the hold is released';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.retention_holds_protect() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'retention holds are evidence and are never deleted (table %)', TG_TABLE_NAME
            USING ERRCODE = '23514', CONSTRAINT = 'retention_holds_retained';
    END IF;

    IF NEW.data_class IS DISTINCT FROM OLD.data_class
        OR NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.reason_code IS DISTINCT FROM OLD.reason_code
        OR NEW.placed_by IS DISTINCT FROM OLD.placed_by
        OR NEW.placed_at IS DISTINCT FROM OLD.placed_at
    THEN
        RAISE EXCEPTION 'the identity and provenance of a retention hold are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'retention_holds_immutable';
    END IF;

    IF OLD.released_at IS NOT NULL
        AND NEW.released_at IS DISTINCT FROM OLD.released_at
    THEN
        RAISE EXCEPTION 'releasing a retention hold happens once'
            USING ERRCODE = '23514', CONSTRAINT = 'retention_holds_release_once';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS retention_holds_retained_delete ON app.retention_holds;
CREATE TRIGGER retention_holds_retained_delete
    BEFORE DELETE ON app.retention_holds
    FOR EACH ROW EXECUTE FUNCTION app.retention_holds_protect();

DROP TRIGGER IF EXISTS retention_holds_protect_update ON app.retention_holds;
CREATE TRIGGER retention_holds_protect_update
    BEFORE UPDATE ON app.retention_holds
    FOR EACH ROW EXECUTE FUNCTION app.retention_holds_protect();

CREATE TABLE IF NOT EXISTS app.retention_runs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    data_class text NOT NULL,
    executed_at timestamptz NOT NULL,
    cutoff_at timestamptz,
    purged_count integer NOT NULL DEFAULT 0,
    anonymized_count integer NOT NULL DEFAULT 0,
    retained_count integer NOT NULL DEFAULT 0,
    held_count integer NOT NULL DEFAULT 0,
    CONSTRAINT retention_runs_class_check CHECK (data_class IN (
        'tokens', 'sessions', 'referential_logs', 'exports', 'abuse_signals', 'billing'
    )),
    CONSTRAINT retention_runs_counts_check CHECK (
        purged_count >= 0
        AND anonymized_count >= 0
        AND retained_count >= 0
        AND held_count >= 0
    ),
    CONSTRAINT retention_runs_cutoff_check CHECK (
        (data_class IN ('referential_logs', 'billing')) = (cutoff_at IS NULL)
    )
);

-- One row per class and execution instant: a replayed run resolves the
-- original record instead of duplicating it.
CREATE UNIQUE INDEX IF NOT EXISTS retention_runs_class_executed_unique
    ON app.retention_runs (data_class, executed_at);

CREATE INDEX IF NOT EXISTS retention_runs_executed_idx
    ON app.retention_runs (executed_at DESC);

COMMENT ON TABLE app.retention_runs IS 'Append-only retention ledger: one row per governed class and execution with counts only, so enforcement is auditable without storing any content';
COMMENT ON COLUMN app.retention_runs.data_class IS 'Governed retention class the run enforced';
COMMENT ON COLUMN app.retention_runs.executed_at IS 'Instant the run enforced the schedule, from the injected clock';
COMMENT ON COLUMN app.retention_runs.cutoff_at IS 'Terminal boundary the run applied (now minus the class window); NULL for classes retained without a purge horizon';
COMMENT ON COLUMN app.retention_runs.purged_count IS 'Records removed by the run';
COMMENT ON COLUMN app.retention_runs.anonymized_count IS 'Records whose restricted references were stripped by the run';
COMMENT ON COLUMN app.retention_runs.retained_count IS 'Records kept under a retention obligation by the run';
COMMENT ON COLUMN app.retention_runs.held_count IS 'Records the run preserved because an active legal hold covered them';

-- +goose StatementBegin
-- The ledger is evidence of enforcement: no row is ever rewritten or
-- erased, by any role.
CREATE OR REPLACE FUNCTION app.retention_runs_freeze() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'retention runs are append-only and are never updated or deleted (table %)', TG_TABLE_NAME
        USING ERRCODE = '23514', CONSTRAINT = 'retention_runs_immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS retention_runs_freeze_update ON app.retention_runs;
CREATE TRIGGER retention_runs_freeze_update
    BEFORE UPDATE ON app.retention_runs
    FOR EACH ROW EXECUTE FUNCTION app.retention_runs_freeze();

DROP TRIGGER IF EXISTS retention_runs_freeze_delete ON app.retention_runs;
CREATE TRIGGER retention_runs_freeze_delete
    BEFORE DELETE ON app.retention_runs
    FOR EACH ROW EXECUTE FUNCTION app.retention_runs_freeze();

ALTER TABLE app.retention_holds OWNER TO arena_owner;
ALTER TABLE app.retention_runs OWNER TO arena_owner;

ALTER FUNCTION app.retention_holds_protect() OWNER TO arena_owner;
ALTER FUNCTION app.retention_runs_freeze() OWNER TO arena_owner;

-- Runtime surface: holds are read, inserted and released (never deleted);
-- the ledger is read and appended.
GRANT SELECT, INSERT, UPDATE ON app.retention_holds TO arena_app;
GRANT SELECT, INSERT ON app.retention_runs TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS retention_runs_freeze_delete ON app.retention_runs;
DROP TRIGGER IF EXISTS retention_runs_freeze_update ON app.retention_runs;
DROP FUNCTION IF EXISTS app.retention_runs_freeze();
DROP TABLE IF EXISTS app.retention_runs;

DROP TRIGGER IF EXISTS retention_holds_protect_update ON app.retention_holds;
DROP TRIGGER IF EXISTS retention_holds_retained_delete ON app.retention_holds;
DROP FUNCTION IF EXISTS app.retention_holds_protect();
DROP TABLE IF EXISTS app.retention_holds;
