-- name: GetLink :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM links t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListLink :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM links t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutLink :one
INSERT INTO links (id, from_id, from_type, to_id, to_type, kind) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'from_id','')::uuid, CAST(@payload AS jsonb)->>'from_type', NULLIF(CAST(@payload AS jsonb)->>'to_id','')::uuid, CAST(@payload AS jsonb)->>'to_type', CAST(@payload AS jsonb)->>'kind') ON CONFLICT (id) DO UPDATE SET from_id = EXCLUDED.from_id, from_type = EXCLUDED.from_type, to_id = EXCLUDED.to_id, to_type = EXCLUDED.to_type, kind = EXCLUDED.kind, updated_at = now() WHERE links.archived_at IS NULL RETURNING (to_jsonb(links) - 'search_vector')::jsonb AS data;
-- name: ArchiveLink :one
UPDATE links SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(links) - 'search_vector')::jsonb AS data;
