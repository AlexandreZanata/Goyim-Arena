-- Personal data export records and projections (P14-T05, docs/PRIVACY.md
-- §4/§6, REQ-PRIV-01). The record statements only ever touch token hashes;
-- the data projections are approved read-only reads over the account's own
-- rows across the identity, profiles, positions, arenas, arguments, wallet,
-- pass and billing schemas. Payment provider identifiers, webhook payloads,
-- moderation evidence and antifraud signals are never selected.

-- ExpirePersonalExports moves the account's ready records past their window
-- to expired. It runs before a new request so a stale link can never block
-- or revive a fresh one; the partial unique index only covers active
-- records. Expiring is idempotent: repeated runs match nothing.
-- name: ExpirePersonalExports :execrows
UPDATE app.data_exports
SET status = 'expired',
    updated_at = sqlc.arg(now)::timestamptz
WHERE account_id = sqlc.arg(account_id)::uuid
  AND status = 'ready'
  AND expires_at <= sqlc.arg(now)::timestamptz;

-- RequestPersonalExport creates the account's active export or rotates the
-- token of the existing one. The partial unique index resolves concurrent
-- re-requests: the winner keeps one active record and every caller receives
-- it.
-- name: RequestPersonalExport :one
INSERT INTO app.data_exports (
    account_id, status, download_token_hash, download_count, max_downloads,
    requested_at, updated_at
)
VALUES (
    sqlc.arg(account_id)::uuid,
    'requested',
    sqlc.arg(download_token_hash)::bytea,
    0,
    sqlc.arg(max_downloads)::integer,
    sqlc.arg(requested_at)::timestamptz,
    sqlc.arg(requested_at)::timestamptz
)
ON CONFLICT (account_id) WHERE status IN ('requested', 'ready')
DO UPDATE SET
    download_token_hash = EXCLUDED.download_token_hash,
    download_count = 0,
    last_downloaded_at = NULL,
    updated_at = EXCLUDED.updated_at
RETURNING id, account_id, status, requested_at, generated_at, expires_at, download_count, max_downloads;

-- GetPersonalExportForGeneration loads one export record for the generation
-- job. The token hash is deliberately not returned: generation never needs
-- the capability.
-- name: GetPersonalExportForGeneration :one
SELECT id, account_id, status, requested_at, generated_at, expires_at, download_count, max_downloads
FROM app.data_exports
WHERE id = sqlc.arg(export_id)::uuid;

-- MarkPersonalExportReady attaches the generated document once. The status
-- guard resolves concurrent generations: only the first writer wins and the
-- loser resolves the recorded replay.
-- name: MarkPersonalExportReady :execrows
UPDATE app.data_exports
SET status = 'ready',
    generated_at = sqlc.arg(generated_at)::timestamptz,
    expires_at = sqlc.arg(expires_at)::timestamptz,
    document = sqlc.arg(document)::text,
    document_sha256 = sqlc.arg(document_sha256)::text,
    updated_at = sqlc.arg(generated_at)::timestamptz
WHERE id = sqlc.arg(export_id)::uuid
  AND status = 'requested';

-- GetPersonalExportDownloadGuard loads the owner-scoped download state so
-- the application can classify unknown/foreign records apart from invalid
-- tokens and unavailable links without ever reflecting whether a foreign
-- export exists.
-- name: GetPersonalExportDownloadGuard :one
SELECT id, account_id, status, expires_at, download_count, max_downloads, download_token_hash
FROM app.data_exports
WHERE id = sqlc.arg(export_id)::uuid
  AND account_id = sqlc.arg(account_id)::uuid;

-- ConsumePersonalExportDownload serves one download and consumes one unit of
-- the budget atomically: the token must match, the record must be ready, the
-- link must be unexpired and the budget unexhausted. Zero rows answer an
-- unavailable link, so a replay can never serve the document twice.
-- name: ConsumePersonalExportDownload :one
UPDATE app.data_exports
SET download_count = download_count + 1,
    last_downloaded_at = sqlc.arg(consumed_at)::timestamptz,
    updated_at = sqlc.arg(consumed_at)::timestamptz
WHERE id = sqlc.arg(export_id)::uuid
  AND account_id = sqlc.arg(account_id)::uuid
  AND status = 'ready'
  AND download_token_hash = sqlc.arg(download_token_hash)::bytea
  AND expires_at > sqlc.arg(consumed_at)::timestamptz
  AND download_count < max_downloads
RETURNING document, document_sha256;

-- GetPersonalExportAccount loads the account identity plus the optional
-- profile and communication preferences. Email belongs to the private
-- export and never to a public projection.
-- name: GetPersonalExportAccount :one
SELECT
    a.id, a.email, a.status, a.email_verified_at, a.created_at,
    p.username, p.interface_locale, p.timezone,
    p.created_at AS profile_created_at, p.updated_at AS profile_updated_at,
    cp.marketing_opt_in, cp.updated_at AS preferences_updated_at
FROM app.accounts a
LEFT JOIN app.profiles p ON p.account_id = a.id
LEFT JOIN app.communication_preferences cp ON cp.account_id = a.id
WHERE a.id = sqlc.arg(account_id)::uuid;

-- ListPersonalExportUsernameHistory returns every username the account ever
-- held, oldest first.
-- name: ListPersonalExportUsernameHistory :many
SELECT username, changed_at
FROM app.username_history
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY changed_at, id;

-- ListPersonalExportSessions returns the account's own sessions without
-- device signals: the IP address and user agent are restricted security
-- data and never enter the export.
-- name: ListPersonalExportSessions :many
SELECT id, created_at, expires_at, revoked_at
FROM app.sessions
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY created_at, id;

-- ListPersonalExportPositions returns the account's individual positions,
-- with the Arena identity as context needed to understand them.
-- name: ListPersonalExportPositions :many
SELECT
    dp.arena_id, ar.slug, ar.statement,
    dp.initial_position, dp.current_position, dp.version,
    dp.created_at, dp.updated_at
FROM app.debate_positions dp
JOIN app.arenas ar ON ar.id = dp.arena_id
WHERE dp.account_id = sqlc.arg(account_id)::uuid
ORDER BY dp.created_at, dp.arena_id;

-- ListPersonalExportPositionChanges returns the account's individual change
-- history, oldest first.
-- name: ListPersonalExportPositionChanges :many
SELECT arena_id, from_position, to_position, version, changed_at
FROM app.position_changes
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY changed_at, id;

-- ListPersonalExportArenaDrafts returns the account's unpublished drafts:
-- private data that only exists in the personal export.
-- name: ListPersonalExportArenaDrafts :many
SELECT id, statement, context, category, language, version, created_at
FROM app.arenas
WHERE creator_id = sqlc.arg(account_id)::uuid
  AND status = 'draft'
ORDER BY created_at, id;

-- ListPersonalExportArguments returns the account's own arguments and
-- replies, published and withdrawn. Moderation-removed content is restricted
-- evidence and stays out; the placeholder status is not part of the personal
-- export either.
-- name: ListPersonalExportArguments :many
SELECT id, arena_id, parent_id, relation, content, status, created_at, withdrawn_at
FROM app.arguments
WHERE author_id = sqlc.arg(account_id)::uuid
  AND status IN ('published', 'withdrawn')
ORDER BY created_at, id;

-- ListPersonalExportArgumentSources returns the sources of the given
-- arguments in deterministic order.
-- name: ListPersonalExportArgumentSources :many
SELECT argument_id, url, description
FROM app.argument_sources
WHERE argument_id = ANY(sqlc.arg(argument_ids)::uuid[])
ORDER BY created_at, id;

-- GetPersonalExportWallet returns the account's derived INK balances. An
-- account that never used the wallet has no row; the application reports
-- zeroed balances.
-- name: GetPersonalExportWallet :one
SELECT balance_free, balance_purchased
FROM app.wallet_accounts
WHERE account_id = sqlc.arg(account_id)::uuid;

-- ListPersonalExportWalletTransactions returns the account's INK ledger:
-- signed bucket deltas only. The operation reference (which may embed
-- provider identifiers) is never selected.
-- name: ListPersonalExportWalletTransactions :many
SELECT o.operation_type, t.bucket, t.amount, t.created_at
FROM app.wallet_transactions t
JOIN app.wallet_operations o ON o.id = t.operation_id
WHERE o.account_id = sqlc.arg(account_id)::uuid
ORDER BY t.created_at, t.id;

-- ListPersonalExportPassLots returns the account's Arena Pass lots without
-- the grant reference, which may embed provider or subscription
-- identifiers.
-- name: ListPersonalExportPassLots :many
SELECT origin, quantity, remaining_quantity, expires_at, created_at
FROM app.arena_pass_lots
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY created_at, id;

-- ListPersonalExportPassConsumptions returns the account's pass
-- consumptions, oldest first.
-- name: ListPersonalExportPassConsumptions :many
SELECT c.arena_id, c.consumed_at
FROM app.arena_pass_consumptions c
JOIN app.arena_pass_lots l ON l.id = c.lot_id
WHERE l.account_id = sqlc.arg(account_id)::uuid
ORDER BY c.consumed_at, c.id;

-- ListPersonalExportCheckoutIntents returns the account's local purchase
-- history: product, market, amount and status only. Provider session and
-- payment identifiers stay out of every export.
-- name: ListPersonalExportCheckoutIntents :many
SELECT product_id, market, currency, amount_minor, status, created_at, paid_at
FROM app.checkout_intents
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY created_at, id;

-- ListPersonalExportSubscriptions returns the account's subscriptions with
-- periods and cancellation state, never provider or price identifiers.
-- name: ListPersonalExportSubscriptions :many
SELECT product_id, status, current_period_start, current_period_end,
       cancel_at_period_end, created_at, updated_at
FROM app.subscriptions
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY created_at, id;
