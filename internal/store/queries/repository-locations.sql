-- name: GetRepositoryLocation :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM repository_locations t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListRepositoryLocation :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM repository_locations t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutRepositoryLocation :one
INSERT INTO repository_locations (id, repository_id, environment_id, path) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'repository_id','')::uuid, NULLIF(CAST(@payload AS jsonb)->>'environment_id','')::uuid, CAST(@payload AS jsonb)->>'path') ON CONFLICT (id) DO UPDATE SET repository_id = EXCLUDED.repository_id, environment_id = EXCLUDED.environment_id, path = EXCLUDED.path, updated_at = now() WHERE repository_locations.archived_at IS NULL RETURNING (to_jsonb(repository_locations) - 'search_vector')::jsonb AS data;
-- name: ArchiveRepositoryLocation :one
UPDATE repository_locations SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(repository_locations) - 'search_vector')::jsonb AS data;
