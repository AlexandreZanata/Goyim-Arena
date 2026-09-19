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

-- name: MarkEmailVerificationTokenUsed :execrows
UPDATE app.email_verification_tokens
SET used_at = now()
WHERE id = $1 AND used_at IS NULL;

-- name: CreatePasswordResetToken :one
INSERT INTO app.password_reset_tokens (account_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, account_id, token_hash, expires_at, used_at, created_at;

-- name: GetActivePasswordResetToken :one
SELECT id, account_id, token_hash, expires_at, used_at, created_at
FROM app.password_reset_tokens
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now();

-- name: GetPasswordResetTokenByHash :one
SELECT id, account_id, token_hash, expires_at, used_at, created_at
FROM app.password_reset_tokens
WHERE token_hash = $1;

-- name: InvalidateActivePasswordResetTokens :exec
UPDATE app.password_reset_tokens
SET used_at = now()
WHERE account_id = $1 AND used_at IS NULL;

-- name: MarkPasswordResetTokenUsed :execrows
UPDATE app.password_reset_tokens
SET used_at = now()
WHERE id = $1 AND used_at IS NULL;

-- name: CreateSession :one
INSERT INTO app.sessions (account_id, token_hash, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, account_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, ip_address, user_agent, mfa_verified_at;

-- name: GetActiveSessionByTokenHash :one
SELECT id, account_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, ip_address, user_agent, mfa_verified_at
FROM app.sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: GetSessionByTokenHash :one
SELECT id, account_id, token_hash, created_at, expires_at, last_seen_at, revoked_at, ip_address, user_agent, mfa_verified_at
FROM app.sessions
WHERE token_hash = $1;

-- name: UpdateSessionLastSeen :exec
UPDATE app.sessions
SET last_seen_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchSession :exec
UPDATE app.sessions
SET last_seen_at = $2, expires_at = $3
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

-- ListActiveAccountSessions returns the sessions of one account that are
-- still usable (P16-T06), newest activity first.
--
-- The three instants are the policy boundaries, not `now()`: the session
-- policy lives in the domain, so the adapter states the same question the
-- domain asks (`created_at + absolute`, `last_seen_at + idle`, the stored
-- deadline) instead of re-deriving its own answer here. A row outside them is
-- already refused by the evaluator, and listing it would show the owner a
-- session that cannot accept a request.
-- name: ListActiveAccountSessions :many
SELECT id, created_at, last_seen_at, expires_at, ip_address, user_agent
FROM app.sessions
WHERE account_id = $1
  AND revoked_at IS NULL
  AND expires_at > $2
  AND last_seen_at > $3
  AND created_at > $4
ORDER BY last_seen_at DESC, id
LIMIT $5;

-- RevokeAccountSessionByID ends one session of one account.
--
-- The account is part of the key on purpose: a revoke addressed by session
-- identifier alone would let a caller end a session it does not own, and the
-- single statement is what makes two concurrent revokes of the same row agree
-- (one reports a change, the other reports none) without a read-modify-write.
-- name: RevokeAccountSessionByID :execrows
UPDATE app.sessions
SET revoked_at = now()
WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL;

-- RevokeSessionsPastDeadline revokes every session that has passed its policy
-- deadline (P16-T06).
--
-- It revokes, it never deletes: the rows of terminal sessions are the retention
-- pass's to remove, under its own window and its holds, so this statement
-- cannot shorten a retention floor. Marking the row terminal makes the
-- refusal a state instead of an arithmetic comparison, which is the same
-- defense in depth the rest of the module uses.
-- name: RevokeSessionsPastDeadline :execrows
UPDATE app.sessions
SET revoked_at = now()
WHERE revoked_at IS NULL
  AND (expires_at <= $1 OR last_seen_at <= $2 OR created_at <= $3);
