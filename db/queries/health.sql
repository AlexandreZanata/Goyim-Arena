-- Health metadata and connectivity queries for the PostgreSQL platform adapter.

-- GetHealthMetadata retrieves the latest applied migration metadata from app.schema_metadata.
-- name: GetHealthMetadata :one
SELECT version_id, is_applied, tstamp
FROM app.schema_metadata
ORDER BY version_id DESC
LIMIT 1;

-- PingHealth executes a trivial query (SELECT 1) to verify connection readiness.
-- name: PingHealth :one
SELECT 1 AS ready;
