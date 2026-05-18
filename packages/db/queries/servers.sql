-- name: ListServers :many
SELECT id, organisation_id, name, hostname, environment, tags, status, last_seen_at, created_at
FROM servers
WHERE organisation_id = $1
ORDER BY name ASC;

-- name: GetServer :one
SELECT id, organisation_id, name, hostname, environment, tags, status, last_seen_at, created_at
FROM servers
WHERE id = $1 AND organisation_id = $2;

-- name: AddServer :one
INSERT INTO servers (organisation_id, name, hostname, environment, tags, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;
