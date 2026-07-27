-- name: CreateExecution :one
INSERT INTO executions (organisation_id, actor_user_id, actor_role_at_time, execution_type, command, command_hash, status, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetExecution :one
SELECT id, organisation_id, actor_user_id, actor_role_at_time, execution_type,
       command, command_hash, status, reason, requested_at, started_at, finished_at
FROM executions
WHERE id = $1 AND organisation_id = $2;

-- name: ListExecutions :many
SELECT id, organisation_id, actor_user_id, actor_role_at_time, execution_type,
       command, status, reason, requested_at, started_at, finished_at
FROM executions
WHERE organisation_id = $1
ORDER BY requested_at DESC
LIMIT $2 OFFSET $3;

-- name: ClaimNextJob :one
UPDATE executions AS e
SET status = 'running', started_at = now()
WHERE e.id = (
  SELECT queued.id FROM executions AS queued
  WHERE queued.status = 'queued' AND queued.organisation_id = $1
  ORDER BY queued.requested_at ASC
  LIMIT 1
)
RETURNING id, command;

-- name: UpdateExecutionResult :exec
UPDATE executions
SET status = $2, finished_at = now()
WHERE id = $1;
