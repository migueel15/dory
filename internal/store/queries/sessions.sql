-- name: GetSession :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM sessions t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListSession :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM sessions t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutSession :one
INSERT INTO sessions (id, user_id, secret_hash, expires_at) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'user_id','')::uuid, CAST(@payload AS jsonb)->>'secret_hash', (CAST(@payload AS jsonb)->>'expires_at')::timestamptz) ON CONFLICT (id) DO UPDATE SET user_id = EXCLUDED.user_id, secret_hash = EXCLUDED.secret_hash, expires_at = EXCLUDED.expires_at, updated_at = now() WHERE sessions.archived_at IS NULL RETURNING (to_jsonb(sessions) - 'search_vector')::jsonb AS data;
-- name: ArchiveSession :one
UPDATE sessions SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(sessions) - 'search_vector')::jsonb AS data;
