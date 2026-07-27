-- name: CreateAuditEvent :one
INSERT INTO audit_events (organisation_id, actor_user_id, action, target_type, target_id, result, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: SearchAuditEvents :many
SELECT id, organisation_id, actor_user_id, action, target_type, target_id,
       result, metadata, occurred_at, previous_hash, event_hash
FROM audit_events
WHERE organisation_id = $1
  AND (actor_user_id = $2 OR $2 = '')
  AND (action = $3 OR $3 = '')
ORDER BY occurred_at DESC
LIMIT $4 OFFSET $5;
