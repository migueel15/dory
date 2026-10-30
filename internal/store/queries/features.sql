-- name: GetFeature :one
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM features t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListFeature :many
SELECT (to_jsonb(t) - 'search_vector')::jsonb AS data FROM features t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutFeature :one
INSERT INTO features (id, project_id, title, body_md, code_refs, status) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'project_id','')::uuid, CAST(@payload AS jsonb)->>'title', COALESCE(CAST(@payload AS jsonb)->>'body_md', ''), ARRAY(SELECT jsonb_array_elements_text(CAST(@payload AS jsonb)->'code_refs')), COALESCE(CAST(@payload AS jsonb)->>'status', 'planned')) ON CONFLICT (id) DO UPDATE SET project_id = EXCLUDED.project_id, title = EXCLUDED.title, body_md = EXCLUDED.body_md, code_refs = EXCLUDED.code_refs, status = EXCLUDED.status, updated_at = now() WHERE features.archived_at IS NULL RETURNING (to_jsonb(features) - 'search_vector')::jsonb AS data;
-- name: ArchiveFeature :one
UPDATE features SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING (to_jsonb(features) - 'search_vector')::jsonb AS data;
