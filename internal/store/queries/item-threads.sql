-- name: GetItemThread :one
SELECT to_jsonb(t)::jsonb AS data FROM item_threads t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListItemThread :many
SELECT to_jsonb(t)::jsonb AS data FROM item_threads t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY created_at, id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutItemThread :one
INSERT INTO item_threads (id, item_id, instance_id, thread_id, role) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'item_id','')::uuid, NULLIF(CAST(@payload AS jsonb)->>'instance_id','')::uuid, CAST(@payload AS jsonb)->>'thread_id', COALESCE(NULLIF(CAST(@payload AS jsonb)->>'role',''), 'implementation')) ON CONFLICT (id) DO UPDATE SET item_id = EXCLUDED.item_id, instance_id = EXCLUDED.instance_id, thread_id = EXCLUDED.thread_id, role = EXCLUDED.role, updated_at = now() WHERE item_threads.archived_at IS NULL RETURNING to_jsonb(item_threads)::jsonb AS data;
-- name: ArchiveItemThread :one
UPDATE item_threads SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING to_jsonb(item_threads)::jsonb AS data;
