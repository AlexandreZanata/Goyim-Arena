-- +goose Up
-- 00024 adds the review claim columns to moderation cases (P13-T04):
-- claimed_by, claimed_at and lease_expires_at. A claim moves a case from
-- open to under_review under one moderator with a bounded lease; an expired
-- lease may be reclaimed by another moderator, and a live lease serializes
-- concurrent claims so two moderators never own the same review.
--
-- Invariants:
-- 1. Claim triple coherence: claimed_by, claimed_at and lease_expires_at
--    move together or not at all; a lease always outlives its claim; only
--    cases under review carry a claim, and open/decided/closed cases carry
--    none. Clearing a claim happens only through the decide transition
--    (P13-T04) which resets the triple while moving to decided.
-- 2. Target, priority history and retention rules from 00022 are untouched:
--    this migration only adds nullable columns, a coherence CHECK and
--    comments, so existing rows stay valid and no data moves.
-- 3. Least privilege is unchanged: arena_app already holds SELECT/INSERT/
--    UPDATE on moderation_cases; no new grant is needed and DELETE stays
--    ungranted and trigger-rejected.

ALTER TABLE app.moderation_cases
    ADD COLUMN IF NOT EXISTS claimed_by uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS claimed_at timestamptz,
    ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'moderation_cases_claim_check'
    ) THEN
        ALTER TABLE app.moderation_cases
            ADD CONSTRAINT moderation_cases_claim_check
            CHECK (
                ((claimed_by IS NULL) = (claimed_at IS NULL))
                AND ((claimed_by IS NULL) = (lease_expires_at IS NULL))
                AND (claimed_by IS NOT NULL OR status <> 'under_review')
                AND (lease_expires_at IS NULL OR lease_expires_at > claimed_at)
            );
    END IF;
END
$$;
-- +goose StatementEnd

COMMENT ON COLUMN app.moderation_cases.claimed_by IS 'Reviewing moderator holding the case; NULL unless the case is under review';
COMMENT ON COLUMN app.moderation_cases.claimed_at IS 'Instant the claim was taken; moves with claimed_by';
COMMENT ON COLUMN app.moderation_cases.lease_expires_at IS 'Bounded claim lease: a live lease serializes concurrent claims, an expired lease may be reclaimed by another moderator';

-- +goose Down
ALTER TABLE app.moderation_cases DROP CONSTRAINT IF EXISTS moderation_cases_claim_check;
ALTER TABLE app.moderation_cases DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE app.moderation_cases DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE app.moderation_cases DROP COLUMN IF EXISTS claimed_by;
