-- +goose Up
-- 00020 establishes the Stripe persistence schema (P12-T02):
-- app.stripe_customers, app.stripe_events, app.checkout_intents,
-- app.subscriptions and the reconciliation state
-- (app.billing_reconciliation_runs, app.billing_reconciliation_findings).
--
-- Invariants (docs/SECURITY.md §8, docs/THREAT_MODEL.md THR-STRIPE-01/02/03,
-- docs/PRIVACY.md, docs/MONETIZATION.md §4):
-- 1. Only a verified, authenticated webhook may reach this schema: the event
--    row is persisted *before* any effect and its provider event id is
--    unique, so a redelivered webhook is identified immediately and returns
--    without re-executing a mutation (REQ-BIL-03, THR-STRIPE-03).
-- 2. The raw webhook payload is deliberately NOT persisted. What remains is
--    the evidence needed to recognise the event again: its identity, its
--    type, the provider creation instant, a SHA-256 digest of the exact body
--    and its size. The body itself may carry personal data (customer email,
--    address) and is unbounded, so it never enters the database
--    (SECURITY.md §8 "não registrar payload integral"; PRIVACY.md).
-- 3. Stripe identifiers are private data. They live only in these tables,
--    are never part of a public projection, view, export or log, and each
--    one is shape-checked so a truncated or foreign value cannot be stored.
-- 4. Status is text with CHECK and each lifecycle is enforced by a trigger:
--    only the transitions of the real provider machine are accepted and
--    terminal states never revive (a canceled subscription can never look
--    active again, a paid checkout can never expire). Provider identity and
--    the commercial facts resolved by the server (market, product, catalog
--    version, currency, amount) are immutable once written.
-- 5. Checkout intents record the *server-authoritative* commercial decision
--    (T04) and therefore pin the market/currency pair of the versioned
--    catalog: BR charges BRL and INTERNATIONAL charges USD, exactly like
--    the billing domain and docs/MONETIZATION.md §4.
-- 6. A Stripe checkout session id encodes its mode (cs_test_/cs_live_), so
--    the database refuses a session id that contradicts livemode: test and
--    live objects can never be mixed in one row.
-- 7. Reconciliation is recorded, never silently applied: a run states the
--    window it inspected with its counters, and every divergence is a
--    finding that can only ever be *resolved* afterwards — its content is
--    immutable, so a discrepancy cannot be rewritten away (T10).
-- 8. Financial and reconciliation history is retained: no cascade from
--    accounts (ON DELETE RESTRICT) and DELETE is rejected for every role by
--    a defensive trigger, not merely by the privilege grant.
-- 9. Least privilege: arena_owner owns every object; arena_app may read and
--    insert, update the mutable projections and resolution of findings, and
--    never delete anything.

CREATE TABLE IF NOT EXISTS app.stripe_customers (
    account_id uuid PRIMARY KEY REFERENCES app.accounts(id) ON DELETE RESTRICT,
    stripe_customer_id text NOT NULL,
    livemode boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stripe_customers_customer_unique UNIQUE (stripe_customer_id),
    -- Provider identifiers are prefix plus alphanumerics, bounded exactly like
    -- the billing domain vocabulary (prefix + at most 194 characters).
    CONSTRAINT stripe_customers_customer_format_check CHECK (
        stripe_customer_id ~ '^cus_[A-Za-z0-9]{1,194}$'
    )
);

COMMENT ON TABLE app.stripe_customers IS 'Mapping between a local account and its Stripe customer; the provider identifier is private and never leaves the billing module';
COMMENT ON COLUMN app.stripe_customers.stripe_customer_id IS 'Private Stripe customer identifier (cus_...); shape checked, never part of a public projection, export or log';
COMMENT ON COLUMN app.stripe_customers.livemode IS 'Stripe mode the customer belongs to; pinned so test and live objects can never be mixed for one account';

CREATE TABLE IF NOT EXISTS app.stripe_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    stripe_event_id text NOT NULL,
    event_type text NOT NULL,
    livemode boolean NOT NULL,
    stripe_created_at timestamptz NOT NULL,
    payload_sha256 text NOT NULL,
    payload_bytes integer NOT NULL,
    status text NOT NULL DEFAULT 'received',
    attempts integer NOT NULL DEFAULT 0,
    last_error text,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    CONSTRAINT stripe_events_event_unique UNIQUE (stripe_event_id),
    CONSTRAINT stripe_events_event_format_check CHECK (
        stripe_event_id ~ '^evt_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT stripe_events_type_check CHECK (
        event_type ~ '^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$'
        AND char_length(event_type) <= 120
    ),
    CONSTRAINT stripe_events_payload_digest_check CHECK (
        payload_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT stripe_events_payload_bytes_check CHECK (payload_bytes > 0),
    CONSTRAINT stripe_events_status_check CHECK (
        status IN ('received', 'processing', 'processed', 'ignored', 'failed')
    ),
    CONSTRAINT stripe_events_processing_check CHECK (
        -- Terminal outcomes are dated and only a failed event carries an
        -- error, which is cleared when it is retried.
        (status IN ('processed', 'ignored')) = (processed_at IS NOT NULL)
        -- Anything past "received" was claimed at least once.
        AND (status = 'received' OR attempts >= 1)
        AND (last_error IS NULL OR status = 'failed')
        AND (last_error IS NULL OR (trim(last_error) <> '' AND char_length(last_error) <= 500))
    )
);

CREATE INDEX IF NOT EXISTS stripe_events_pending_idx
    ON app.stripe_events (received_at)
    WHERE status IN ('received', 'processing', 'failed');
CREATE INDEX IF NOT EXISTS stripe_events_type_idx
    ON app.stripe_events (event_type, received_at DESC);

COMMENT ON TABLE app.stripe_events IS 'Verified Stripe webhook events: the unique provider event id makes redelivery harmless, and the raw payload is never persisted';
COMMENT ON COLUMN app.stripe_events.stripe_event_id IS 'Unique provider event identifier (evt_...); the idempotency anchor of the webhook (REQ-BIL-03, THR-STRIPE-03)';
COMMENT ON COLUMN app.stripe_events.stripe_created_at IS 'Provider creation instant of the event, kept so out-of-order deliveries can be reasoned about';
COMMENT ON COLUMN app.stripe_events.payload_sha256 IS 'SHA-256 of the exact raw body: evidence of integrity and duplication without storing the payload (which may carry personal data)';
COMMENT ON COLUMN app.stripe_events.payload_bytes IS 'Size in bytes of the exact raw body that was verified';
COMMENT ON COLUMN app.stripe_events.attempts IS 'Number of times the event was claimed for processing; retries reuse this row instead of inserting a new one';
COMMENT ON COLUMN app.stripe_events.last_error IS 'Bounded reason of the latest failure; exists only while status is failed and is cleared on retry';

CREATE TABLE IF NOT EXISTS app.checkout_intents (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.stripe_customers(account_id) ON DELETE RESTRICT,
    market text NOT NULL,
    product_id text NOT NULL,
    catalog_version integer NOT NULL,
    currency text NOT NULL,
    amount_minor bigint NOT NULL,
    livemode boolean NOT NULL,
    status text NOT NULL DEFAULT 'created',
    stripe_checkout_session_id text,
    stripe_payment_intent_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    paid_at timestamptz,
    closed_at timestamptz,
    CONSTRAINT checkout_intents_market_check CHECK (market IN ('BR', 'INTERNATIONAL')),
    CONSTRAINT checkout_intents_currency_check CHECK (currency IN ('BRL', 'USD')),
    CONSTRAINT checkout_intents_market_currency_check CHECK (
        (market = 'BR' AND currency = 'BRL')
        OR (market = 'INTERNATIONAL' AND currency = 'USD')
    ),
    CONSTRAINT checkout_intents_product_check CHECK (
        product_id ~ '^[a-z][a-z0-9_]{1,62}[a-z0-9]$'
    ),
    CONSTRAINT checkout_intents_catalog_version_check CHECK (catalog_version > 0),
    CONSTRAINT checkout_intents_amount_check CHECK (amount_minor > 0),
    CONSTRAINT checkout_intents_status_check CHECK (
        status IN ('created', 'open', 'paid', 'expired', 'failed')
    ),
    CONSTRAINT checkout_intents_session_format_check CHECK (
        stripe_checkout_session_id IS NULL
        OR stripe_checkout_session_id ~ '^cs_(test|live)_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT checkout_intents_session_mode_check CHECK (
        stripe_checkout_session_id IS NULL
        OR (livemode AND starts_with(stripe_checkout_session_id, 'cs_live_'))
        OR (NOT livemode AND starts_with(stripe_checkout_session_id, 'cs_test_'))
    ),
    CONSTRAINT checkout_intents_payment_intent_format_check CHECK (
        stripe_payment_intent_id IS NULL OR stripe_payment_intent_id ~ '^pi_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT checkout_intents_lifecycle_check CHECK (
        -- A shipment of the session: created has none, open/paid/expired
        -- must have one and a failed attempt may have none (the provider
        -- refused the session itself).
        (status <> 'created' OR stripe_checkout_session_id IS NULL)
        AND (status NOT IN ('open', 'paid', 'expired') OR stripe_checkout_session_id IS NOT NULL)
        AND (stripe_payment_intent_id IS NULL OR stripe_checkout_session_id IS NOT NULL)
        AND (status IN ('expired', 'failed')) = (closed_at IS NOT NULL)
        AND (status = 'paid') = (paid_at IS NOT NULL)
    ),
    CONSTRAINT checkout_intents_session_unique UNIQUE (stripe_checkout_session_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS checkout_intents_payment_intent_unique
    ON app.checkout_intents (stripe_payment_intent_id)
    WHERE stripe_payment_intent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS checkout_intents_account_idx
    ON app.checkout_intents (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS checkout_intents_unsettled_idx
    ON app.checkout_intents (created_at)
    WHERE status IN ('created', 'open');

COMMENT ON TABLE app.checkout_intents IS 'Server-authoritative checkout intents: the local record of the commercial decision and its provider correlation; only a verified webhook settles one';
COMMENT ON COLUMN app.checkout_intents.market IS 'Commercial region resolved by the server (never inferred from IP) with the currency it charges';
COMMENT ON COLUMN app.checkout_intents.product_id IS 'Versioned catalog product (lower snake case), the same vocabulary as the billing domain';
COMMENT ON COLUMN app.checkout_intents.catalog_version IS 'Catalog version that was in force when the price was resolved, so an old purchase stays explainable after a price change';
COMMENT ON COLUMN app.checkout_intents.amount_minor IS 'Exact price in minor units of the currency, as resolved from the catalog; never provided by the browser';
COMMENT ON COLUMN app.checkout_intents.stripe_checkout_session_id IS 'Private Stripe checkout session identifier (cs_...); its prefix is pinned to livemode';
COMMENT ON COLUMN app.checkout_intents.stripe_payment_intent_id IS 'Private Stripe payment intent identifier (pi_...), kept for refund and dispute correlation';
COMMENT ON COLUMN app.checkout_intents.paid_at IS 'Instant the provider confirmed the payment; success pages never set it';
COMMENT ON COLUMN app.checkout_intents.closed_at IS 'Instant the intent closed without payment (expired or failed)';

CREATE TABLE IF NOT EXISTS app.subscriptions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.stripe_customers(account_id) ON DELETE RESTRICT,
    stripe_subscription_id text NOT NULL,
    status text NOT NULL,
    livemode boolean NOT NULL,
    market text NOT NULL,
    product_id text NOT NULL,
    catalog_version integer NOT NULL,
    stripe_price_id text NOT NULL,
    current_period_start timestamptz,
    current_period_end timestamptz,
    cancel_at_period_end boolean NOT NULL DEFAULT false,
    canceled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subscriptions_subscription_unique UNIQUE (stripe_subscription_id),
    CONSTRAINT subscriptions_subscription_format_check CHECK (
        stripe_subscription_id ~ '^sub_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT subscriptions_status_check CHECK (
        status IN (
            'incomplete', 'incomplete_expired', 'trialing', 'active',
            'past_due', 'canceled', 'unpaid', 'paused'
        )
    ),
    CONSTRAINT subscriptions_market_check CHECK (market IN ('BR', 'INTERNATIONAL')),
    CONSTRAINT subscriptions_product_check CHECK (
        product_id ~ '^[a-z][a-z0-9_]{1,62}[a-z0-9]$'
    ),
    CONSTRAINT subscriptions_catalog_version_check CHECK (catalog_version > 0),
    -- Mirrors domain.ParseStripePriceID exactly: prefix plus alphanumerics,
    -- total length at most 200 characters.
    CONSTRAINT subscriptions_price_format_check CHECK (
        stripe_price_id ~ '^price_[A-Za-z0-9]{1,194}$'
    ),
    CONSTRAINT subscriptions_period_check CHECK (
        (current_period_start IS NULL) = (current_period_end IS NULL)
        AND (current_period_end IS NULL OR current_period_end > current_period_start)
    )
);

CREATE INDEX IF NOT EXISTS subscriptions_account_idx
    ON app.subscriptions (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS subscriptions_live_idx
    ON app.subscriptions (status, current_period_end)
    WHERE status IN ('trialing', 'active', 'past_due', 'unpaid', 'paused');

COMMENT ON TABLE app.subscriptions IS 'Mirror of the provider subscription state; the entitlement franchises of a period are granted from this state and never from a browser visit';
COMMENT ON COLUMN app.subscriptions.status IS 'Provider status vocabulary; canceled and incomplete_expired are terminal and can never revive';
COMMENT ON COLUMN app.subscriptions.stripe_price_id IS 'Private Stripe price in use by the subscription; compared against the versioned catalog by reconciliation';
COMMENT ON COLUMN app.subscriptions.current_period_start IS 'Start of the billed period, the anchor of the per-period Member franchise';
COMMENT ON COLUMN app.subscriptions.current_period_end IS 'End of the billed period; a period-bound pass expires here and the franchise does not accumulate';
COMMENT ON COLUMN app.subscriptions.cancel_at_period_end IS 'Provider cancellation scheduled for the end of the period; benefits last until then';
COMMENT ON COLUMN app.subscriptions.canceled_at IS 'Instant the provider registered the cancellation, when it reports one';

CREATE TABLE IF NOT EXISTS app.billing_reconciliation_runs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    livemode boolean NOT NULL,
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'running',
    scanned_objects integer NOT NULL DEFAULT 0,
    findings_count integer NOT NULL DEFAULT 0,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT billing_reconciliation_runs_window_check CHECK (window_end > window_start),
    CONSTRAINT billing_reconciliation_runs_status_check CHECK (
        status IN ('running', 'completed', 'failed')
    ),
    CONSTRAINT billing_reconciliation_runs_counts_check CHECK (
        scanned_objects >= 0 AND findings_count >= 0
    ),
    CONSTRAINT billing_reconciliation_runs_finished_check CHECK (
        (status = 'running') = (finished_at IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS billing_reconciliation_runs_window_idx
    ON app.billing_reconciliation_runs (window_start, window_end);
CREATE INDEX IF NOT EXISTS billing_reconciliation_runs_open_idx
    ON app.billing_reconciliation_runs (started_at)
    WHERE status = 'running';

COMMENT ON TABLE app.billing_reconciliation_runs IS 'Reconciliation windows already inspected, with the outcome the job reported; job-level execution bookkeeping belongs to the jobs module';
COMMENT ON COLUMN app.billing_reconciliation_runs.window_start IS 'Inclusive start of the inspected window (half-open with window_end)';

CREATE TABLE IF NOT EXISTS app.billing_reconciliation_findings (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    run_id uuid NOT NULL REFERENCES app.billing_reconciliation_runs(id) ON DELETE RESTRICT,
    account_id uuid REFERENCES app.accounts(id) ON DELETE RESTRICT,
    kind text NOT NULL,
    reference text NOT NULL,
    details text,
    observed_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    resolution text,
    CONSTRAINT billing_reconciliation_findings_kind_check CHECK (
        kind IN (
            'missing_local', 'missing_remote', 'amount_mismatch',
            'currency_mismatch', 'status_mismatch', 'unprocessed_event'
        )
    ),
    CONSTRAINT billing_reconciliation_findings_reference_check CHECK (
        trim(reference) <> '' AND char_length(reference) <= 200
    ),
    CONSTRAINT billing_reconciliation_findings_details_check CHECK (
        details IS NULL OR (trim(details) <> '' AND char_length(details) <= 500)
    ),
    CONSTRAINT billing_reconciliation_findings_resolution_check CHECK (
        (resolved_at IS NULL) = (resolution IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS billing_reconciliation_findings_run_idx
    ON app.billing_reconciliation_findings (run_id);
CREATE INDEX IF NOT EXISTS billing_reconciliation_findings_account_idx
    ON app.billing_reconciliation_findings (account_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS billing_reconciliation_findings_unresolved_idx
    ON app.billing_reconciliation_findings (observed_at)
    WHERE resolved_at IS NULL;

COMMENT ON TABLE app.billing_reconciliation_findings IS 'Divergences found between the local state and the provider; recorded so they are reviewed instead of silently corrected';
COMMENT ON COLUMN app.billing_reconciliation_findings.account_id IS 'Account the divergence concerns; NULL when the object exists only on the provider side';
COMMENT ON COLUMN app.billing_reconciliation_findings.reference IS 'Private reference of the diverging object (local identifier or provider identifier); never part of a public projection, export or log';
COMMENT ON COLUMN app.billing_reconciliation_findings.resolution IS 'Justification written when the finding is resolved; the rest of the record is immutable';

-- +goose StatementBegin
-- Retention: billing history is evidence of money and of the reconciliation
-- duty, so no row of this module is ever deleted, by any role.
CREATE OR REPLACE FUNCTION app.billing_retain_rows() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'billing rows are retained and are never deleted (table %)', TG_TABLE_NAME
        USING ERRCODE = '23514', CONSTRAINT = 'billing_rows_retained';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- The customer mapping pins the account and the Stripe mode; the customer
-- object itself may be replaced if the provider deletes it.
CREATE OR REPLACE FUNCTION app.stripe_customers_protect_mapping() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.livemode IS DISTINCT FROM OLD.livemode
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'the account and the Stripe mode of a customer mapping are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'stripe_customers_mapping_immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- A verified event keeps its identity and its payload evidence forever; only
-- the processing state moves, and never backwards out of a terminal outcome.
CREATE OR REPLACE FUNCTION app.stripe_events_protect_processing() RETURNS trigger AS $$
BEGIN
    IF NEW.stripe_event_id IS DISTINCT FROM OLD.stripe_event_id
        OR NEW.event_type IS DISTINCT FROM OLD.event_type
        OR NEW.livemode IS DISTINCT FROM OLD.livemode
        OR NEW.stripe_created_at IS DISTINCT FROM OLD.stripe_created_at
        OR NEW.payload_sha256 IS DISTINCT FROM OLD.payload_sha256
        OR NEW.payload_bytes IS DISTINCT FROM OLD.payload_bytes
        OR NEW.received_at IS DISTINCT FROM OLD.received_at
    THEN
        RAISE EXCEPTION 'the verified event identity and its payload evidence are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'stripe_events_identity_immutable';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NOT (
            (OLD.status = 'received' AND NEW.status IN ('processing', 'ignored', 'failed'))
            OR (OLD.status = 'processing' AND NEW.status IN ('processed', 'ignored', 'failed'))
            OR (OLD.status = 'failed' AND NEW.status = 'processing')
        ) THEN
            RAISE EXCEPTION 'illegal stripe event transition % -> %', OLD.status, NEW.status
                USING ERRCODE = '23514', CONSTRAINT = 'stripe_events_status_transition';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- The commercial decision of an intent is historical: it can never be
-- recomputed, and a settled, expired or failed intent never goes back.
CREATE OR REPLACE FUNCTION app.checkout_intents_protect_lifecycle() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.market IS DISTINCT FROM OLD.market
        OR NEW.product_id IS DISTINCT FROM OLD.product_id
        OR NEW.catalog_version IS DISTINCT FROM OLD.catalog_version
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.amount_minor IS DISTINCT FROM OLD.amount_minor
        OR NEW.livemode IS DISTINCT FROM OLD.livemode
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR (OLD.stripe_checkout_session_id IS NOT NULL
            AND NEW.stripe_checkout_session_id IS DISTINCT FROM OLD.stripe_checkout_session_id)
        OR (OLD.stripe_payment_intent_id IS NOT NULL
            AND NEW.stripe_payment_intent_id IS DISTINCT FROM OLD.stripe_payment_intent_id)
    THEN
        RAISE EXCEPTION 'the commercial facts and provider identifiers of a checkout intent are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'checkout_intents_immutable';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NOT (
            (OLD.status = 'created' AND NEW.status IN ('open', 'failed'))
            OR (OLD.status = 'open' AND NEW.status IN ('paid', 'expired', 'failed'))
        ) THEN
            RAISE EXCEPTION 'illegal checkout intent transition % -> %', OLD.status, NEW.status
                USING ERRCODE = '23514', CONSTRAINT = 'checkout_intents_status_transition';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Subscription identity is immutable and the status machine mirrors the
-- provider: every real edge is accepted (including out-of-order convergence)
-- while canceled and incomplete_expired stay terminal.
CREATE OR REPLACE FUNCTION app.subscriptions_protect_lifecycle() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.stripe_subscription_id IS DISTINCT FROM OLD.stripe_subscription_id
        OR NEW.livemode IS DISTINCT FROM OLD.livemode
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'the identity of a subscription mirror is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'subscriptions_identity_immutable';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NOT (
            (OLD.status = 'incomplete' AND NEW.status IN ('trialing', 'active', 'past_due', 'unpaid', 'canceled', 'incomplete_expired'))
            OR (OLD.status = 'trialing' AND NEW.status IN ('active', 'past_due', 'unpaid', 'canceled', 'incomplete_expired', 'paused'))
            OR (OLD.status = 'active' AND NEW.status IN ('trialing', 'past_due', 'unpaid', 'canceled', 'paused'))
            OR (OLD.status = 'past_due' AND NEW.status IN ('active', 'trialing', 'unpaid', 'canceled', 'paused'))
            OR (OLD.status = 'unpaid' AND NEW.status IN ('active', 'trialing', 'past_due', 'canceled', 'paused'))
            OR (OLD.status = 'paused' AND NEW.status IN ('active', 'trialing', 'past_due', 'unpaid', 'canceled'))
        ) THEN
            RAISE EXCEPTION 'illegal subscription transition % -> %', OLD.status, NEW.status
                USING ERRCODE = '23514', CONSTRAINT = 'subscriptions_status_transition';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- A discrepancy is a fact of the reconciliation: only its resolution may be
-- written, so a finding can never be rewritten, reclassified or erased.
CREATE OR REPLACE FUNCTION app.billing_reconciliation_findings_protect() RETURNS trigger AS $$
BEGIN
    IF NEW.run_id IS DISTINCT FROM OLD.run_id
        OR NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.kind IS DISTINCT FROM OLD.kind
        OR NEW.reference IS DISTINCT FROM OLD.reference
        OR NEW.details IS DISTINCT FROM OLD.details
        OR NEW.observed_at IS DISTINCT FROM OLD.observed_at
    THEN
        RAISE EXCEPTION 'a reconciliation finding is preserved and can only be resolved'
            USING ERRCODE = '23514', CONSTRAINT = 'billing_reconciliation_findings_immutable';
    END IF;

    IF OLD.resolved_at IS NOT NULL AND (
        NEW.resolved_at IS DISTINCT FROM OLD.resolved_at
        OR NEW.resolution IS DISTINCT FROM OLD.resolution
    ) THEN
        RAISE EXCEPTION 'the resolution of a reconciliation finding is final'
            USING ERRCODE = '23514', CONSTRAINT = 'billing_reconciliation_findings_resolution_final';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS stripe_customers_retained_delete ON app.stripe_customers;
CREATE TRIGGER stripe_customers_retained_delete
    BEFORE DELETE ON app.stripe_customers
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS stripe_events_retained_delete ON app.stripe_events;
CREATE TRIGGER stripe_events_retained_delete
    BEFORE DELETE ON app.stripe_events
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS checkout_intents_retained_delete ON app.checkout_intents;
CREATE TRIGGER checkout_intents_retained_delete
    BEFORE DELETE ON app.checkout_intents
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS subscriptions_retained_delete ON app.subscriptions;
CREATE TRIGGER subscriptions_retained_delete
    BEFORE DELETE ON app.subscriptions
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS billing_reconciliation_runs_retained_delete ON app.billing_reconciliation_runs;
CREATE TRIGGER billing_reconciliation_runs_retained_delete
    BEFORE DELETE ON app.billing_reconciliation_runs
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS billing_reconciliation_findings_retained_delete ON app.billing_reconciliation_findings;
CREATE TRIGGER billing_reconciliation_findings_retained_delete
    BEFORE DELETE ON app.billing_reconciliation_findings
    FOR EACH ROW EXECUTE FUNCTION app.billing_retain_rows();

DROP TRIGGER IF EXISTS stripe_customers_protect_mapping ON app.stripe_customers;
CREATE TRIGGER stripe_customers_protect_mapping
    BEFORE UPDATE ON app.stripe_customers
    FOR EACH ROW EXECUTE FUNCTION app.stripe_customers_protect_mapping();

DROP TRIGGER IF EXISTS stripe_events_protect_processing ON app.stripe_events;
CREATE TRIGGER stripe_events_protect_processing
    BEFORE UPDATE ON app.stripe_events
    FOR EACH ROW EXECUTE FUNCTION app.stripe_events_protect_processing();

DROP TRIGGER IF EXISTS checkout_intents_protect_lifecycle ON app.checkout_intents;
CREATE TRIGGER checkout_intents_protect_lifecycle
    BEFORE UPDATE ON app.checkout_intents
    FOR EACH ROW EXECUTE FUNCTION app.checkout_intents_protect_lifecycle();

DROP TRIGGER IF EXISTS subscriptions_protect_lifecycle ON app.subscriptions;
CREATE TRIGGER subscriptions_protect_lifecycle
    BEFORE UPDATE ON app.subscriptions
    FOR EACH ROW EXECUTE FUNCTION app.subscriptions_protect_lifecycle();

DROP TRIGGER IF EXISTS billing_reconciliation_findings_protect ON app.billing_reconciliation_findings;
CREATE TRIGGER billing_reconciliation_findings_protect
    BEFORE UPDATE ON app.billing_reconciliation_findings
    FOR EACH ROW EXECUTE FUNCTION app.billing_reconciliation_findings_protect();

ALTER TABLE app.stripe_customers OWNER TO arena_owner;
ALTER TABLE app.stripe_events OWNER TO arena_owner;
ALTER TABLE app.checkout_intents OWNER TO arena_owner;
ALTER TABLE app.subscriptions OWNER TO arena_owner;
ALTER TABLE app.billing_reconciliation_runs OWNER TO arena_owner;
ALTER TABLE app.billing_reconciliation_findings OWNER TO arena_owner;

ALTER FUNCTION app.billing_retain_rows() OWNER TO arena_owner;
ALTER FUNCTION app.stripe_customers_protect_mapping() OWNER TO arena_owner;
ALTER FUNCTION app.stripe_events_protect_processing() OWNER TO arena_owner;
ALTER FUNCTION app.checkout_intents_protect_lifecycle() OWNER TO arena_owner;
ALTER FUNCTION app.subscriptions_protect_lifecycle() OWNER TO arena_owner;
ALTER FUNCTION app.billing_reconciliation_findings_protect() OWNER TO arena_owner;

-- Runtime surface: the runtime records customers, events, intents,
-- subscriptions and reconciliation and moves only the mutable projections.
-- DELETE is not granted anywhere: retention is enforced by the trigger too.
GRANT SELECT, INSERT, UPDATE ON app.stripe_customers TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.stripe_events TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.checkout_intents TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.subscriptions TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.billing_reconciliation_runs TO arena_app;
GRANT SELECT, INSERT, UPDATE ON app.billing_reconciliation_findings TO arena_app;

-- +goose Down
DROP TRIGGER IF EXISTS billing_reconciliation_findings_protect ON app.billing_reconciliation_findings;
DROP TRIGGER IF EXISTS subscriptions_protect_lifecycle ON app.subscriptions;
DROP TRIGGER IF EXISTS checkout_intents_protect_lifecycle ON app.checkout_intents;
DROP TRIGGER IF EXISTS stripe_events_protect_processing ON app.stripe_events;
DROP TRIGGER IF EXISTS stripe_customers_protect_mapping ON app.stripe_customers;
DROP TRIGGER IF EXISTS billing_reconciliation_findings_retained_delete ON app.billing_reconciliation_findings;
DROP TRIGGER IF EXISTS billing_reconciliation_runs_retained_delete ON app.billing_reconciliation_runs;
DROP TRIGGER IF EXISTS subscriptions_retained_delete ON app.subscriptions;
DROP TRIGGER IF EXISTS checkout_intents_retained_delete ON app.checkout_intents;
DROP TRIGGER IF EXISTS stripe_events_retained_delete ON app.stripe_events;
DROP TRIGGER IF EXISTS stripe_customers_retained_delete ON app.stripe_customers;
DROP FUNCTION IF EXISTS app.billing_reconciliation_findings_protect();
DROP FUNCTION IF EXISTS app.subscriptions_protect_lifecycle();
DROP FUNCTION IF EXISTS app.checkout_intents_protect_lifecycle();
DROP FUNCTION IF EXISTS app.stripe_events_protect_processing();
DROP FUNCTION IF EXISTS app.stripe_customers_protect_mapping();
DROP FUNCTION IF EXISTS app.billing_retain_rows();
DROP TABLE IF EXISTS app.billing_reconciliation_findings;
DROP TABLE IF EXISTS app.billing_reconciliation_runs;
DROP TABLE IF EXISTS app.subscriptions;
DROP TABLE IF EXISTS app.checkout_intents;
DROP TABLE IF EXISTS app.stripe_events;
DROP TABLE IF EXISTS app.stripe_customers;
