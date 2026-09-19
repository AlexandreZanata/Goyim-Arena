-- MFA queries (P16-T05). The two statements that carry a security invariant
-- are written as single statements on purpose:
--
--   * UpsertPendingMFAEnrollment refuses to overwrite a confirmed enrollment,
--     so "start enrollment" cannot become a way to reset somebody's second
--     factor;
--   * AdvanceMFAVerifiedStep only moves forward, so two requests presenting the
--     same code cannot both be accepted.

-- name: GetMFAEnrollment :one
SELECT account_id, secret_sealed, confirmed_at, last_accepted_step, created_at, updated_at
FROM app.account_mfa
WHERE account_id = $1;

-- name: UpsertPendingMFAEnrollment :one
INSERT INTO app.account_mfa (account_id, secret_sealed)
VALUES ($1, $2)
ON CONFLICT (account_id) DO UPDATE
    SET secret_sealed = EXCLUDED.secret_sealed,
        last_accepted_step = -1,
        updated_at = now()
    WHERE app.account_mfa.confirmed_at IS NULL
RETURNING account_id, secret_sealed, confirmed_at, last_accepted_step, created_at, updated_at;

-- name: ConfirmMFAEnrollment :one
UPDATE app.account_mfa
SET confirmed_at = $2,
    last_accepted_step = $3,
    updated_at = now()
WHERE account_id = $1
  AND confirmed_at IS NULL
RETURNING confirmed_at;

-- name: AdvanceMFAVerifiedStep :execrows
UPDATE app.account_mfa
SET last_accepted_step = $2,
    updated_at = now()
WHERE account_id = $1
  AND last_accepted_step < $2;

-- name: DeleteMFABackupCodes :execrows
DELETE FROM app.mfa_backup_codes
WHERE account_id = $1;

-- name: InsertMFABackupCode :exec
INSERT INTO app.mfa_backup_codes (account_id, code_hash)
VALUES ($1, $2);

-- name: ListUnusedMFABackupCodes :many
SELECT id, code_hash
FROM app.mfa_backup_codes
WHERE account_id = $1
  AND used_at IS NULL
ORDER BY created_at, id;

-- name: ConsumeMFABackupCode :execrows
UPDATE app.mfa_backup_codes
SET used_at = $2
WHERE id = $1
  AND used_at IS NULL;

-- name: MarkSessionMFAVerified :execrows
UPDATE app.sessions
SET mfa_verified_at = $2
WHERE id = $1
  AND revoked_at IS NULL;

-- name: GetSessionMFAVerifiedAt :one
SELECT mfa_verified_at
FROM app.sessions
WHERE id = $1;
