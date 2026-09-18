-- +goose Up
-- 00026 establishes the personal data export records (P14-T05,
-- docs/PRIVACY.md §4/§6, REQ-PRIV-01).
--
-- Invariants:
-- 1. One export belongs to exactly one account and is addressed by a
--    stable id; the owner is the only account that can download it.
-- 2. At most one active export exists per account (requested or ready):
--    a partial unique index serializes re-requests, which rotate the
--    download token instead of spawning parallel jobs.
-- 3. The download capability is an opaque 256-bit token; only its SHA-256
--    hash is stored, exactly like sessions and recovery tokens.
-- 4. A ready document is immutable and carries its content hash; the
--    document is the machine-readable personal export and is validated as
--    JSON by a trigger (a CHECK cannot cast safely).
-- 5. Downloads are bounded: download_count never exceeds max_downloads
--    and every consumption is dated.
-- 6. Exports are retained (no DELETE for any role); expiry moves the row
--    to expired and the retention workflow (P14-T07) may purge only the
--    document bytes, never the record.
-- 7. Least privilege: arena_app reads, inserts and updates; DELETE is
--    rejected by a trigger for every role.

CREATE TABLE IF NOT EXISTS app.data_exports (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'requested',
    download_token_hash bytea NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    generated_at timestamptz,
    expires_at timestamptz,
    document text,
    document_sha256 text,
    download_count integer NOT NULL DEFAULT 0,
    max_downloads integer NOT NULL DEFAULT 1,
    last_downloaded_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT data_exports_status_check CHECK (status IN ('requested', 'ready', 'expired')),
    CONSTRAINT data_exports_token_hash_check CHECK (octet_length(download_token_hash) = 32),
    CONSTRAINT data_exports_requested_check CHECK (
        status <> 'requested'
        OR (generated_at IS NULL AND expires_at IS NULL AND document IS NULL AND document_sha256 IS NULL)
    ),
    CONSTRAINT data_exports_ready_check CHECK (
        status <> 'ready'
        OR (
            generated_at IS NOT NULL
            AND expires_at IS NOT NULL
            AND expires_at > generated_at
            AND document IS NOT NULL
            AND document_sha256 IS NOT NULL
        )
    ),
    CONSTRAINT data_exports_expiry_check CHECK (expires_at IS NULL OR generated_at IS NOT NULL),
    CONSTRAINT data_exports_document_hash_check CHECK (
        document_sha256 IS NULL OR document_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT data_exports_downloads_check CHECK (
        download_count >= 0 AND max_downloads > 0 AND download_count <= max_downloads
    ),
    CONSTRAINT data_exports_downloaded_check CHECK (
        (download_count = 0) = (last_downloaded_at IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS data_exports_account_active_unique
    ON app.data_exports (account_id)
    WHERE status IN ('requested', 'ready');

CREATE INDEX IF NOT EXISTS data_exports_account_requested_idx
    ON app.data_exports (account_id, requested_at DESC);

COMMENT ON TABLE app.data_exports IS 'Personal data export jobs: owner-scoped, token-protected, expiring and download-limited; the document never contains restricted antifraud or provider secret data';
COMMENT ON COLUMN app.data_exports.download_token_hash IS 'SHA-256 of the opaque download token; the raw capability is returned exactly once to the owner';
COMMENT ON COLUMN app.data_exports.document IS 'Versioned machine-readable export document (JSON text), immutable once written';
COMMENT ON COLUMN app.data_exports.document_sha256 IS 'SHA-256 of the exact document bytes served to the owner';
COMMENT ON COLUMN app.data_exports.max_downloads IS 'Download budget frozen per export: policy changes never retrofit existing records';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.data_exports_protect() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'personal export records are retained; retention purges the document, never the record'
            USING ERRCODE = '23514', CONSTRAINT = 'data_exports_retained';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.account_id IS DISTINCT FROM OLD.account_id
            OR NEW.requested_at IS DISTINCT FROM OLD.requested_at
        THEN
            RAISE EXCEPTION 'export identity and provenance are immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'data_exports_immutable';
        END IF;

        IF OLD.document IS NOT NULL AND NEW.document IS NOT NULL
            AND NEW.document IS DISTINCT FROM OLD.document
        THEN
            RAISE EXCEPTION 'a generated export document is immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'data_exports_document_immutable';
        END IF;
    END IF;

    IF NEW.document IS NOT NULL THEN
        BEGIN
            PERFORM NEW.document::jsonb;
        EXCEPTION WHEN others THEN
            RAISE EXCEPTION 'export document must be valid JSON'
                USING ERRCODE = '23514', CONSTRAINT = 'data_exports_document_json';
        END;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS data_exports_protect_insert ON app.data_exports;
CREATE TRIGGER data_exports_protect_insert
    BEFORE INSERT ON app.data_exports
    FOR EACH ROW EXECUTE FUNCTION app.data_exports_protect();

DROP TRIGGER IF EXISTS data_exports_protect_update ON app.data_exports;
CREATE TRIGGER data_exports_protect_update
    BEFORE UPDATE ON app.data_exports
    FOR EACH ROW EXECUTE FUNCTION app.data_exports_protect();

DROP TRIGGER IF EXISTS data_exports_retained_delete ON app.data_exports;
CREATE TRIGGER data_exports_retained_delete
    BEFORE DELETE ON app.data_exports
    FOR EACH ROW EXECUTE FUNCTION app.data_exports_protect();

ALTER TABLE app.data_exports OWNER TO arena_owner;
ALTER FUNCTION app.data_exports_protect() OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE ON app.data_exports TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS data_exports_retained_delete ON app.data_exports;
DROP TRIGGER IF EXISTS data_exports_protect_update ON app.data_exports;
DROP TRIGGER IF EXISTS data_exports_protect_insert ON app.data_exports;
DROP FUNCTION IF EXISTS app.data_exports_protect();
DROP TABLE IF EXISTS app.data_exports;
