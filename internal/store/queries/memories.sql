-- name: GetMemory :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM memories t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListMemory :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM memories t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutMemory :one
INSERT INTO memories (id, scope, scope_id, kind, title, content, tags) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), CAST(@payload AS jsonb)->>'scope', NULLIF(CAST(@payload AS jsonb)->>'scope_id','')::uuid, COALESCE(NULLIF(CAST(@payload AS jsonb)->>'kind',''), 'context'), COALESCE(CAST(@payload AS jsonb)->>'title', ''), CAST(@payload AS jsonb)->>'content', ARRAY(SELECT jsonb_array_elements_text(CAST(@payload AS jsonb)->'tags'))) ON CONFLICT (id) DO UPDATE SET scope = EXCLUDED.scope, scope_id = EXCLUDED.scope_id, kind = EXCLUDED.kind, title = EXCLUDED.title, content = EXCLUDED.content, tags = EXCLUDED.tags, updated_at = now() WHERE memories.archived_at IS NULL RETURNING (to_jsonb(memories) - 'search_vector')::jsonb AS data;
-- name: ArchiveMemory :one
UPDATE memories SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(memories) - 'search_vector')::jsonb AS data;
