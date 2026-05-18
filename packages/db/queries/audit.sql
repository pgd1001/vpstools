-- name: CreateAuditEvent :one
INSERT INTO audit_events (organisation_id, actor_id, action, target_type, target_id, result, metadata_json)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: SearchAuditEvents :many
SELECT id, organisation_id, actor_id, action, target_type, target_id, result, metadata_json, created_at
FROM audit_events
WHERE organisation_id = $1
  AND (actor_id = $2 OR $2 = '')
  AND (action = $3 OR $3 = '')
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;
