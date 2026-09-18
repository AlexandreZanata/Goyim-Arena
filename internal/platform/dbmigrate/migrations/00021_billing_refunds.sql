-- +goose Up
-- 00021 establishes the explicit refund and chargeback state (P12-T09) and
-- the compensating ledger operation.
--
-- Invariants (docs/MONETIZATION.md §2.3, docs/SECURITY.md §8, REQ-BIL-04,
-- THR-STRIPE-02):
-- 1. Refunds and chargebacks are explicit rows, never silent mutations: one
--    row per provider refund/dispute object, anchored on its unique provider
--    identifier (re_... or dp_...), so a redelivered webhook resolves the
--    same row instead of compensating twice.
-- 2. The ledger stays append-only: the compensation is a new debit_refund
--    transaction on PURCHASED_INK (for INK packs) and a zeroing of the
--    remaining projection of the purchased pass lot (for passes). History
--    rows are never updated or deleted; consumption rows are never removed.
-- 3. Good-faith consumption never becomes a negative balance: the debit is
--    capped at the available PURCHASED_INK balance and any shortfall —
--    like any dispute, any partial pass refund or any fully consumed benefit
--    — becomes needs_review for support/fraud instead of an overdraft.
-- 4. The included franchise has no refund value (MONETIZATION §2.2): Member
--    refunds record the fact and wait for review without automatic ledger
--    movement.
-- 5. Provider identifiers are private and shape-checked; the commercial facts
--    (intent, amounts, revoked quantities) are immutable once written and
--    only the human resolution may be appended.
-- 6. Financial history is retained: no cascade from accounts or intents
--    (ON DELETE RESTRICT) and DELETE is rejected for every role by a
--    defensive trigger.
-- 7. Least privilege: arena_owner owns every object; arena_app may read and
--    insert, resolve the human review once, and never delete anything.

-- The compensating operation joins the ledger vocabulary. The old CHECK is
-- replaced so debit_refund is accepted alongside the eight existing types;
-- every other value stays rejected.
ALTER TABLE app.wallet_operations
    DROP CONSTRAINT IF EXISTS wallet_operations_type_check;
ALTER TABLE app.wallet_operations
    ADD CONSTRAINT wallet_operations_type_check CHECK (operation_type IN (
        'credit_free',
        'credit_member',
        'credit_purchase',
        'credit_refund',
        'credit_admin',
        'debit_argument',
        'debit_admin',
        'debit_refund',
        'expire_free'
    ));

CREATE TABLE IF NOT EXISTS app.billing_refunds (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    checkout_intent_id uuid NOT NULL REFERENCES app.checkout_intents(id) ON DELETE RESTRICT,
    provider_refund_id text NOT NULL,
    source text NOT NULL,
    status text NOT NULL,
    charged_amount_minor bigint NOT NULL,
    refunded_amount_minor bigint NOT NULL,
    ink_revoked bigint NOT NULL DEFAULT 0,
    passes_revoked integer NOT NULL DEFAULT 0,
    needs_review boolean NOT NULL,
    review_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    resolution text,
    CONSTRAINT billing_refunds_provider_unique UNIQUE (provider_refund_id),
    CONSTRAINT billing_refunds_provider_format_check CHECK (
        provider_refund_id ~ '^(re|dp)_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT billing_refunds_source_check CHECK (
        source IN ('refund', 'dispute')
    ),
    CONSTRAINT billing_refunds_status_check CHECK (
        status IN ('applied', 'needs_review')
    ),
    CONSTRAINT billing_refunds_amounts_check CHECK (
        charged_amount_minor > 0 AND refunded_amount_minor > 0
        AND refunded_amount_minor <= charged_amount_minor
    ),
    CONSTRAINT billing_refunds_revoked_check CHECK (
        ink_revoked >= 0 AND passes_revoked >= 0
    ),
    CONSTRAINT billing_refunds_review_check CHECK (
        (status = 'needs_review') = needs_review
    ),
    CONSTRAINT billing_refunds_review_reason_check CHECK (
        review_reason IS NULL
        OR (trim(review_reason) <> '' AND char_length(review_reason) <= 500)
    ),
    CONSTRAINT billing_refunds_resolution_check CHECK (
        (resolved_at IS NULL) = (resolution IS NULL)
    ),
    CONSTRAINT billing_refunds_resolution_text_check CHECK (
        resolution IS NULL
        OR (trim(resolution) <> '' AND char_length(resolution) <= 500)
    )
);

CREATE INDEX IF NOT EXISTS billing_refunds_account_idx
    ON app.billing_refunds (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS billing_refunds_intent_idx
    ON app.billing_refunds (checkout_intent_id);
CREATE INDEX IF NOT EXISTS billing_refunds_needs_review_idx
    ON app.billing_refunds (created_at)
    WHERE needs_review AND resolved_at IS NULL;

COMMENT ON TABLE app.billing_refunds IS 'Explicit refund and chargeback records: one row per provider refund/dispute object, with the compensating quantities and the human review flag';
COMMENT ON COLUMN app.billing_refunds.provider_refund_id IS 'Private provider refund (re_...) or dispute (dp_...) identifier; the idempotency anchor of the compensation, never part of a public projection, export or log';
COMMENT ON COLUMN app.billing_refunds.source IS 'Whether the money was returned (refund) or contested (dispute/chargeback); disputes always wait for review';
COMMENT ON COLUMN app.billing_refunds.status IS 'Explicit outcome: applied when the unused benefit covered the reversal, needs_review when support/fraud must decide';
COMMENT ON COLUMN app.billing_refunds.ink_revoked IS 'Purchased INK withdrawn by the compensating debit_refund entry; capped at the available balance so consumption never becomes a negative balance';
COMMENT ON COLUMN app.billing_refunds.passes_revoked IS 'Remaining purchased passes revoked by zeroing the lot projection; consumed passes are never rewritten, they become review';
COMMENT ON COLUMN app.billing_refunds.needs_review IS 'True when part of the benefit was already consumed, the refund was partial over passes, the source was a dispute, or nothing reversible remained';
COMMENT ON COLUMN app.billing_refunds.review_reason IS 'Machine-readable reason the row waits for review (for example already_consumed, partial_pass_refund, chargeback)';
COMMENT ON COLUMN app.billing_refunds.resolution IS 'Justification written when a human resolves the review; the rest of the record is immutable';

-- +goose StatementBegin
-- A refund record is a fact of money going back: only its human resolution
-- may be appended afterwards, so a compensation can never be rewritten,
-- reclassified or erased.
CREATE OR REPLACE FUNCTION app.billing_refunds_protect() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.checkout_intent_id IS DISTINCT FROM OLD.checkout_intent_id
        OR NEW.provider_refund_id IS DISTINCT FROM OLD.provider_refund_id
        OR NEW.source IS DISTINCT FROM OLD.source
        OR NEW.status IS DISTINCT FROM OLD.status
        OR NEW.charged_amount_minor IS DISTINCT FROM OLD.charged_amount_minor
        OR NEW.refunded_amount_minor IS DISTINCT FROM OLD.refunded_amount_minor
        OR NEW.ink_revoked IS DISTINCT FROM OLD.ink_revoked
        OR NEW.passes_revoked IS DISTINCT FROM OLD.passes_revoked
        OR NEW.needs_review IS DISTINCT FROM OLD.needs_review
        OR NEW.review_reason IS DISTINCT FROM OLD.review_reason
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'a refund record is preserved and can only be resolved'
            USING ERRCODE = '23514', CONSTRAINT = 'billing_refunds_immutable';
    END IF;

    IF OLD.resolved_at IS NOT NULL AND (
        NEW.resolved_at IS DISTINCT FROM OLD.resolved_at
        OR NEW.resolution IS DISTINCT FROM OLD.resolution
    ) THEN
        RAISE EXCEPTION 'the resolution of a refund is final'
            USING ERRCODE = '23514', CONSTRAINT = 'billing_refunds_resolution_final';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS billing_refunds_retained_delete ON app.billing_refunds;
CREATE TRIGGER billing_refunds_retained_delete
    BEFORE DELETE ON app.billing_refunds
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS billing_refunds_protect ON app.billing_refunds;
CREATE TRIGGER billing_refunds_protect
    BEFORE UPDATE ON app.billing_refunds
    FOR EACH ROW EXECUTE FUNCTION app.billing_refunds_protect();

ALTER TABLE app.billing_refunds OWNER TO arena_owner;
ALTER FUNCTION app.billing_refunds_protect() OWNER TO arena_owner;

GRANT SELECT, INSERT, UPDATE ON app.billing_refunds TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS billing_refunds_protect ON app.billing_refunds;
DROP TRIGGER IF EXISTS billing_refunds_retained_delete ON app.billing_refunds;
DROP FUNCTION IF EXISTS app.billing_refunds_protect();
DROP TABLE IF EXISTS app.billing_refunds;
ALTER TABLE app.wallet_operations
    DROP CONSTRAINT IF EXISTS wallet_operations_type_check;
ALTER TABLE app.wallet_operations
    ADD CONSTRAINT wallet_operations_type_check CHECK (operation_type IN (
        'credit_free',
        'credit_member',
        'credit_purchase',
        'credit_refund',
        'credit_admin',
        'debit_argument',
        'debit_admin',
        'expire_free'
    ));
