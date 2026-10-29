-- name: GetEnvironment :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM environments t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListEnvironment :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM environments t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutEnvironment :one
INSERT INTO environments (id, name, os, access, agents_available, notes) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), CAST(@payload AS jsonb)->>'name', COALESCE(CAST(@payload AS jsonb)->>'os', ''), COALESCE(CAST(@payload AS jsonb)->>'access', ''), ARRAY(SELECT jsonb_array_elements_text(CAST(@payload AS jsonb)->'agents_available')), COALESCE(CAST(@payload AS jsonb)->>'notes', '')) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, os = EXCLUDED.os, access = EXCLUDED.access, agents_available = EXCLUDED.agents_available, notes = EXCLUDED.notes, updated_at = now() WHERE environments.archived_at IS NULL RETURNING (to_jsonb(environments) - 'search_vector')::jsonb AS data;
-- name: ArchiveEnvironment :one
UPDATE environments SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(environments) - 'search_vector')::jsonb AS data;
