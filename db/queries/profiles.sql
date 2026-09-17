-- Profile, username history and interface-locale queries for the PostgreSQL
-- platform adapter.
--
-- Privacy contract (docs/PRIVACY.md): profile queries never select or return
-- email, credentials or payment-provider identifiers. Email stays in
-- app.accounts; payment identifiers belong to the billing module.
-- Public lookups only ever read from app.profiles by username_normalized.

-- name: CreateProfile :one
INSERT INTO app.profiles (account_id, username, username_normalized, interface_locale)
VALUES ($1, $2, $3, $4)
RETURNING account_id, username, username_normalized, interface_locale, created_at, updated_at;

-- name: GetProfileByAccountID :one
SELECT account_id, username, username_normalized, interface_locale, created_at, updated_at
FROM app.profiles
WHERE account_id = $1;

-- GetPublicProfileByUsername returns only the publicly allowed profile fields.
-- It never selects email, credentials, internal financial identifiers or
-- administrative flags, and deliberately omits account_id.
-- name: GetPublicProfileByUsername :one
SELECT username, interface_locale, created_at
FROM app.profiles
WHERE username_normalized = $1;

-- name: UpdateProfileUsername :one
UPDATE app.profiles
SET username = $2, username_normalized = $3, updated_at = now()
WHERE account_id = $1
RETURNING account_id, username, username_normalized, interface_locale, created_at, updated_at;

-- name: UpdateProfileLocale :one
UPDATE app.profiles
SET interface_locale = $2, updated_at = now()
WHERE account_id = $1
RETURNING account_id, username, username_normalized, interface_locale, created_at, updated_at;

-- name: CreateUsernameHistoryEntry :exec
INSERT INTO app.username_history (account_id, username, username_normalized)
VALUES ($1, $2, $3);

-- name: ListUsernameHistoryByAccountID :many
SELECT id, account_id, username, username_normalized, changed_at
FROM app.username_history
WHERE account_id = $1
ORDER BY changed_at DESC, id DESC;
