-- Platform administrative audit trail (P14-T01). The runtime inserts facts
-- and reads them back for support and auditors; updates and deletes are
-- rejected by triggers for every role, so no query here mutates history.

-- name: InsertAuditEvent :one
INSERT INTO app.audit_events (occurred_at, actor_id, action, target_type, target_id, reason_code, metadata, idempotency_key, correlation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, occurred_at, actor_id, action, target_type, target_id, reason_code, metadata, idempotency_key, correlation_id;

-- name: GetAuditEventByIdempotencyKey :one
SELECT id, occurred_at, actor_id, action, target_type, target_id, reason_code, metadata, idempotency_key, correlation_id
FROM app.audit_events
WHERE idempotency_key = $1;

-- name: ListAuditEventsByTarget :many
SELECT id, occurred_at, actor_id, action, target_type, target_id, reason_code, metadata, idempotency_key, correlation_id
FROM app.audit_events
WHERE target_type = $1 AND target_id = $2
ORDER BY occurred_at ASC, id ASC;
