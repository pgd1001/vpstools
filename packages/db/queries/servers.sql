-- name: ListServers :many
SELECT s.id, s.organisation_id, s.name, s.hostname, s.environment,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('key', st.key, 'value', st.value) ORDER BY st.key)
                 FROM server_tags st WHERE st.server_id = s.id AND st.organisation_id = s.organisation_id), '[]'::jsonb) AS tags,
       s.status, s.last_seen_at, s.created_at
FROM servers s
WHERE s.organisation_id = $1
ORDER BY s.name ASC;

-- name: GetServer :one
SELECT s.id, s.organisation_id, s.name, s.hostname, s.environment,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('key', st.key, 'value', st.value) ORDER BY st.key)
                 FROM server_tags st WHERE st.server_id = s.id AND st.organisation_id = s.organisation_id), '[]'::jsonb) AS tags,
       s.status, s.last_seen_at, s.created_at
FROM servers s
WHERE s.id = $1 AND s.organisation_id = $2;

-- name: AddServer :one
INSERT INTO servers (organisation_id, name, hostname, environment, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: AddServerTag :exec
INSERT INTO server_tags (organisation_id, server_id, key, value)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organisation_id, server_id, key) DO UPDATE SET value = EXCLUDED.value;
