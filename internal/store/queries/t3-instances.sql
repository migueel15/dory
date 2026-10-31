-- name: GetT3Instance :one
SELECT to_jsonb(t)::jsonb AS data FROM t3_instances t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListT3Instance :many
SELECT to_jsonb(t)::jsonb AS data FROM t3_instances t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutT3Instance :one
INSERT INTO t3_instances (id, environment_id, base_url, token_env) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'environment_id','')::uuid, CAST(@payload AS jsonb)->>'base_url', COALESCE(CAST(@payload AS jsonb)->>'token_env', '')) ON CONFLICT (id) DO UPDATE SET environment_id = EXCLUDED.environment_id, base_url = EXCLUDED.base_url, token_env = EXCLUDED.token_env, updated_at = now() WHERE t3_instances.archived_at IS NULL RETURNING to_jsonb(t3_instances)::jsonb AS data;
-- name: ArchiveT3Instance :one
UPDATE t3_instances SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING to_jsonb(t3_instances)::jsonb AS data;
