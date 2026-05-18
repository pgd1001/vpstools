-- name: CreateExecution :one
INSERT INTO executions (organisation_id, actor_id, command, command_hash, status, reason, dry_run)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetExecution :one
SELECT id, organisation_id, actor_id, command, command_hash, status, reason, dry_run,
       created_at, started_at, finished_at
FROM executions
WHERE id = $1 AND organisation_id = $2;

-- name: ListExecutions :many
SELECT id, organisation_id, actor_id, command, status, reason, created_at, started_at, finished_at
FROM executions
WHERE organisation_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ClaimNextJob :one
UPDATE executions
SET status = 'running', started_at = now()
WHERE id = (
  SELECT id FROM executions
  WHERE status = 'queued' AND organisation_id = $1
  ORDER BY created_at ASC
  LIMIT 1
)
RETURNING id, command;

-- name: UpdateExecutionResult :exec
UPDATE executions
SET status = $2, finished_at = now()
WHERE id = $1;
