-- name: GetProject :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM projects t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListProject :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM projects t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutProject :one
INSERT INTO projects (id, slug, name, description, status, stack) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), CAST(@payload AS jsonb)->>'slug', CAST(@payload AS jsonb)->>'name', COALESCE(CAST(@payload AS jsonb)->>'description', ''), COALESCE(CAST(@payload AS jsonb)->>'status', 'active'), ARRAY(SELECT jsonb_array_elements_text(CAST(@payload AS jsonb)->'stack'))) ON CONFLICT (id) DO UPDATE SET slug = EXCLUDED.slug, name = EXCLUDED.name, description = EXCLUDED.description, status = EXCLUDED.status, stack = EXCLUDED.stack, updated_at = now() WHERE projects.archived_at IS NULL RETURNING (to_jsonb(projects) - 'search_vector')::jsonb AS data;
-- name: ArchiveProject :one
UPDATE projects SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(projects) - 'search_vector')::jsonb AS data;
