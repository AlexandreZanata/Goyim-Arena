-- Identity and authentication queries for the PostgreSQL platform adapter.

-- name: CreateAccount :one
INSERT INTO app.accounts (email, status)
VALUES ($1, $2)
RETURNING id, email, status, email_verified_at, created_at, updated_at;

-- name: GetAccountByID :one
SELECT id, email, status, email_verified_at, created_at, updated_at
FROM app.accounts
WHERE id = $1;

-- name: GetAccountByEmail :one
SELECT id, email, status, email_verified_at, created_at, updated_at
FROM app.accounts
WHERE lower(email) = lower($1);

-- GetAccountProfile retrieves account profile data without ever reading or exposing password credentials.
-- name: GetAccountProfile :one
SELECT id, email, status, email_verified_at, created_at, updated_at
FROM app.accounts
WHERE id = $1;

-- name: UpdateAccountStatus :one
UPDATE app.accounts
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING id, email, status, email_verified_at, created_at, updated_at;

-- name: SetEmailVerified :one
UPDATE app.accounts
SET status = 'active', email_verified_at = now(), updated_at = now()
WHERE id = $1
RETURNING id, email, status, email_verified_at, created_at, updated_at;

-- name: CreatePasswordCredential :exec
INSERT INTO app.password_credentials (account_id, password_hash, algorithm, version)
VALUES ($1, $2, $3, $4);

-- name: GetPasswordCredentialByAccountID :one
SELECT account_id, password_hash, algorithm, version, created_at, updated_at
FROM app.password_credentials
WHERE account_id = $1;

-- name: UpdatePasswordCredential :exec
UPDATE app.password_credentials
SET password_hash = $2, algorithm = $3, version = $4, updated_at = now()
WHERE account_id = $1;

-- name: CreateEmailVerificationToken :one
INSERT INTO app.email_verification_tokens (account_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, account_id, token_hash, expires_at, used_at, created_at;

-- name: GetActiveEmailVerificationToken :one
SELECT id, account_id, token_hash, expires_at, used_at, created_at
FROM app.email_verification_tokens
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now();

-- name: GetEmailVerificationTokenByHash :one
SELECT id, account_id, token_hash, expires_at, used_at, created_at
FROM app.email_verification_tokens
WHERE token_hash = $1;

-- name: InvalidateActiveEmailVerificationTokens :exec
UPDATE app.email_verification_tokens
SET used_at = now()
WHERE account_id = $1 AND used_at IS NULL;

-- name: MarkEmailVerificationTokenUsed :exec
UPDATE app.email_verification_tokens
SET used_at = now()
WHERE id = $1;

-- name: CreatePasswordResetToken :one
INSERT INTO app.password_reset_tokens (account_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, account_id, token_hash, expires_at, used_at, created_at;

-- name: GetActivePasswordResetToken :one
SELECT id, account_id, token_hash, expires_at, used_at, created_at
FROM app.password_reset_tokens
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now();

-- name: MarkPasswordResetTokenUsed :exec
UPDATE app.password_reset_tokens
SET used_at = now()
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO app.sessions (account_id, token_hash, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, account_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, ip_address, user_agent;

-- name: GetActiveSessionByTokenHash :one
SELECT id, account_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, ip_address, user_agent
FROM app.sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: UpdateSessionLastSeen :exec
UPDATE app.sessions
SET last_seen_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE app.sessions
SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllAccountSessions :exec
UPDATE app.sessions
SET revoked_at = now()
WHERE account_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :execrows
DELETE FROM app.sessions
WHERE expires_at < now() OR revoked_at IS NOT NULL;
