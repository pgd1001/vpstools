-- name: GetUser :one
SELECT id, email, display_name, status, created_at
FROM users
WHERE id = $1;

-- name: GetUserMembership :one
SELECT m.id, m.organisation_id, m.user_id, m.role, m.created_at, o.name AS org_name
FROM memberships m
JOIN organisations o ON o.id = m.organisation_id
WHERE m.user_id = $1
LIMIT 1;
