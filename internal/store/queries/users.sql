-- name: GetUser :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM users t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListUser :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM users t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutUser :one
INSERT INTO users (id, username, password_hash) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), CAST(@payload AS jsonb)->>'username', CAST(@payload AS jsonb)->>'password_hash') ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username, password_hash = EXCLUDED.password_hash, updated_at = now() WHERE users.archived_at IS NULL RETURNING (to_jsonb(users) - 'search_vector')::jsonb AS data;
-- name: ArchiveUser :one
UPDATE users SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(users) - 'search_vector')::jsonb AS data;
