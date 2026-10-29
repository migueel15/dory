-- name: GetToken :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM api_tokens t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListToken :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM api_tokens t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutToken :one
INSERT INTO api_tokens (id, user_id, name, secret_hash, permissions, expires_at) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'user_id','')::uuid, CAST(@payload AS jsonb)->>'name', CAST(@payload AS jsonb)->>'secret_hash', ARRAY(SELECT jsonb_array_elements_text(CAST(@payload AS jsonb)->'permissions')), (CAST(@payload AS jsonb)->>'expires_at')::timestamptz) ON CONFLICT (id) DO UPDATE SET user_id = EXCLUDED.user_id, name = EXCLUDED.name, secret_hash = EXCLUDED.secret_hash, permissions = EXCLUDED.permissions, expires_at = EXCLUDED.expires_at, updated_at = now() WHERE api_tokens.archived_at IS NULL RETURNING (to_jsonb(api_tokens) - 'search_vector')::jsonb AS data;
-- name: ArchiveToken :one
UPDATE api_tokens SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(api_tokens) - 'search_vector')::jsonb AS data;
