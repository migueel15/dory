-- name: GetRepository :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM repositories t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListRepository :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM repositories t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutRepository :one
INSERT INTO repositories (id, project_id, remote_url, default_branch) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'project_id','')::uuid, CAST(@payload AS jsonb)->>'remote_url', COALESCE(CAST(@payload AS jsonb)->>'default_branch', 'main')) ON CONFLICT (id) DO UPDATE SET project_id = EXCLUDED.project_id, remote_url = EXCLUDED.remote_url, default_branch = EXCLUDED.default_branch, updated_at = now() WHERE repositories.archived_at IS NULL RETURNING (to_jsonb(repositories) - 'search_vector')::jsonb AS data;
-- name: ArchiveRepository :one
UPDATE repositories SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(repositories) - 'search_vector')::jsonb AS data;
