-- Arena Pass entitlement queries for the PostgreSQL platform adapter.
--
-- Consumption is append-only: no query updates or deletes a consumption and
-- no query mutates a lot's quantity or expiration. The runtime grants
-- enforce the same boundary in the database (P07-T01).

-- name: CreateArenaPassLot :one
INSERT INTO app.arena_pass_lots (account_id, origin, quantity, remaining_quantity, expires_at, reference)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at;

-- CreateArenaPassLotIfAbsent inserts the grant exactly once per
-- (account, origin, reference). On a conflict it returns no row, which tells
-- the adapter to resolve the original lot (P07-T02).
-- name: CreateArenaPassLotIfAbsent :one
INSERT INTO app.arena_pass_lots (account_id, origin, quantity, remaining_quantity, expires_at, reference)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (account_id, origin, reference) DO NOTHING
RETURNING id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at;

-- name: GetArenaPassLotByGrant :one
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1 AND origin = $2 AND reference = $3;

-- name: GetArenaPassLot :one
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE id = $1;

-- name: ListArenaPassLotsByAccount :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1
ORDER BY expires_at NULLS LAST, created_at;

-- ConsumeArenaPassLot atomically decrements a lot that still has passes. The
-- conditional predicate and the remaining_quantity CHECK together make
-- over-consumption impossible, even under concurrent consumers (P07-T03).
-- name: ConsumeArenaPassLot :execrows
UPDATE app.arena_pass_lots
SET remaining_quantity = remaining_quantity - 1
WHERE id = $1 AND remaining_quantity > 0;

-- ListAvailablePassLotsForUpdate locks the consumable lots of an account in
-- consumption order: nearest expiration first, then lots that never expire.
-- Expired lots are never candidates, so they can never be consumed (P07-T03).
-- name: ListAvailablePassLotsForUpdate :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE account_id = $1
  AND remaining_quantity > 0
  AND (expires_at IS NULL OR expires_at > sqlc.arg(at)::timestamptz)
ORDER BY expires_at ASC NULLS LAST, created_at ASC
FOR UPDATE;

-- name: GetArenaPassConsumptionByArena :one
SELECT id, lot_id, arena_id, consumed_at
FROM app.arena_pass_consumptions
WHERE arena_id = $1;

-- ListExpiredArenaPassLots derives the expired lots that still hold passes.
-- Expiration is never written back: the predicate is evaluated at read time,
-- so the sweep is a pure derivation and repeated runs are identical (P07-T04).
-- name: ListExpiredArenaPassLots :many
SELECT id, account_id, origin, quantity, remaining_quantity, expires_at, reference, created_at
FROM app.arena_pass_lots
WHERE expires_at IS NOT NULL
  AND expires_at <= sqlc.arg(at)::timestamptz
  AND remaining_quantity > 0
ORDER BY expires_at ASC, id ASC
LIMIT sqlc.arg(page_limit);

-- name: CreateArenaPassConsumption :one
INSERT INTO app.arena_pass_consumptions (lot_id, arena_id)
VALUES ($1, $2)
RETURNING id, lot_id, arena_id, consumed_at;

-- name: ListArenaPassConsumptionsByAccount :many
SELECT c.id, c.lot_id, c.arena_id, c.consumed_at, l.origin, l.reference
FROM app.arena_pass_consumptions c
JOIN app.arena_pass_lots l ON l.id = c.lot_id
WHERE l.account_id = $1
ORDER BY c.consumed_at DESC, c.id DESC;

-- ListArenaPassConsumptionsPage returns one keyset-paginated page of the
-- owner's consumption history, newest first. NULL after_* parameters select
-- the first page; the (consumed_at, id) tuple comparison never duplicates or
-- skips rows (P07-T06).
-- name: ListArenaPassConsumptionsPage :many
SELECT c.id, c.lot_id, c.arena_id, c.consumed_at, l.origin, l.reference
FROM app.arena_pass_consumptions c
JOIN app.arena_pass_lots l ON l.id = c.lot_id
WHERE l.account_id = sqlc.arg(account_id)
  AND (
      sqlc.arg(after_consumed_at)::timestamptz IS NULL
      OR (c.consumed_at, c.id) < (sqlc.arg(after_consumed_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY c.consumed_at DESC, c.id DESC
LIMIT sqlc.arg(page_limit);

-- Checkout queries (P12-T04). The commercial decision of an intent is
-- resolved by the server from the versioned catalog and is immutable once
-- written; these queries insert it and resolve replays, never recompute it.

-- IsAccountEligibleForPurchase projects the single bit the checkout needs
-- before it decides to charge anyone: the account exists, is active and has a
-- verified email (docs/BUSINESS_RULES.md §7, REQ-AUTH-02). It reads no email,
-- no credential and no payment identifier, so an ineligible or forged
-- identifier can never be mistaken for a legitimate buyer. A missing row means
-- the account does not exist at all.
-- name: IsAccountEligibleForPurchase :one
SELECT (status = 'active' AND email_verified_at IS NOT NULL) AS eligible
FROM app.accounts
WHERE id = $1;

-- name: GetStripeCustomer :one
SELECT account_id, stripe_customer_id, livemode, created_at
FROM app.stripe_customers
WHERE account_id = $1;

-- RecordStripeCustomerIfAbsent stores the account→customer correlation exactly
-- once per account: a concurrent or retried insertion writes nothing and
-- resolves the stored mapping, so the account is never charged through two
-- different provider customers.
-- name: RecordStripeCustomerIfAbsent :one
INSERT INTO app.stripe_customers (account_id, stripe_customer_id, livemode)
VALUES ($1, $2, $3)
ON CONFLICT (account_id) DO NOTHING
RETURNING account_id, stripe_customer_id, livemode, created_at;

-- GetCheckoutIntentBySession resolves the intent a provider session already
-- stands for. The session identifier is unique by constraint, which is what
-- makes a replay resolve the original intent instead of creating a second one.
-- name: GetCheckoutIntentBySession :one
SELECT id, account_id, market, product_id, catalog_version, currency, amount_minor, livemode, status, stripe_checkout_session_id, created_at
FROM app.checkout_intents
WHERE stripe_checkout_session_id = $1;

-- RecordCheckoutIntentIfAbsent inserts the commercial decision exactly once per
-- provider session, so a replay of the same operation resolves the stored
-- intent. The lifecycle CHECK is what keeps the recorded state honest: an open
-- intent has a session, an expired one is closed at a known instant.
-- name: RecordCheckoutIntentIfAbsent :one
INSERT INTO app.checkout_intents (
    account_id, market, product_id, catalog_version, currency, amount_minor,
    livemode, status, stripe_checkout_session_id, closed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (stripe_checkout_session_id) DO NOTHING
RETURNING id, account_id, market, product_id, catalog_version, currency, amount_minor, livemode, status, stripe_checkout_session_id, created_at;

-- Webhook event queries (P12-T05). The provider's unique event ID is the
-- idempotency anchor: a replay resolves the existing row and never creates a
-- second one. The processing lifecycle is enforced by CHECK constraints and
-- the transition table in the stripe_events_protect_processing trigger.

-- InsertWebhookEventIfAbsent persists the verified inbound event exactly once
-- per provider event ID. On conflict (the unique stripe_event_id), it returns
-- no row, which tells the adapter to resolve the existing event.
-- name: InsertWebhookEventIfAbsent :one
INSERT INTO app.stripe_events (
    stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (stripe_event_id) DO NOTHING
RETURNING id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at;

-- GetWebhookEventByEventID resolves an existing event by its provider
-- identifier. The event must exist: a conflict without a stored event is an
-- integrity problem.
-- name: GetWebhookEventByEventID :one
SELECT id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at
FROM app.stripe_events
WHERE stripe_event_id = $1;

-- UpdateWebhookEventStatus transitions the event to the requested status. The
-- CHECK constraint stripe_events_status_transition enforces legal transitions,
-- so an illegal transition is a database error.
-- name: UpdateWebhookEventStatus :one
UPDATE app.stripe_events
SET status = $2, attempts = attempts + 1
WHERE stripe_event_id = $1
RETURNING id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at;

-- UpdateWebhookEventStatusProcessed transitions the event to the processed
-- terminal state and records the processing completion time.
-- name: UpdateWebhookEventStatusProcessed :one
UPDATE app.stripe_events
SET status = $2, processed_at = $3, last_error = NULL
WHERE stripe_event_id = $1
RETURNING id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at;

-- UpdateWebhookEventStatusFailed transitions the event to the failed state
-- with a bounded error reason. The event may be retried later.
-- name: UpdateWebhookEventStatusFailed :one
UPDATE app.stripe_events
SET status = $2, last_error = $3
WHERE stripe_event_id = $1
RETURNING id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at;

-- UpdateWebhookEventStatusIgnored transitions the event to the ignored
-- terminal state for event types that are acknowledged but not handled.
-- name: UpdateWebhookEventStatusIgnored :one
UPDATE app.stripe_events
SET status = $2, processed_at = $3, last_error = NULL
WHERE stripe_event_id = $1
RETURNING id, stripe_event_id, event_type, livemode, stripe_created_at,
    payload_sha256, payload_bytes, status, attempts, last_error,
    received_at, processed_at;
