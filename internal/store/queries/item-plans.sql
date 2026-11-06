-- name: GetItemPlan :one
SELECT to_jsonb(t)::jsonb AS data FROM item_plans t WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL;
-- name: ListItemPlan :many
SELECT to_jsonb(t)::jsonb AS data FROM item_plans t WHERE (sqlc.arg(include_archived)::boolean OR archived_at IS NULL) AND to_jsonb(t) @> CAST(@filter AS jsonb) ORDER BY item_id, version DESC LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
-- name: PutItemPlan :one
-- A new plan takes the next version of its item; an existing one only changes accepted_at, so versions stay immutable.
INSERT INTO item_plans (id, item_id, version, sections, open_questions, author, accepted_at) VALUES (COALESCE(NULLIF(CAST(@payload AS jsonb)->>'id','')::uuid, gen_random_uuid()), NULLIF(CAST(@payload AS jsonb)->>'item_id','')::uuid, (SELECT COALESCE(max(p.version), 0) + 1 FROM item_plans p WHERE p.item_id = NULLIF(CAST(@payload AS jsonb)->>'item_id','')::uuid), COALESCE(CAST(@payload AS jsonb)->'sections', '{}'::jsonb), ARRAY(SELECT jsonb_array_elements_text(COALESCE(CAST(@payload AS jsonb)->'open_questions', '[]'::jsonb))), COALESCE(NULLIF(CAST(@payload AS jsonb)->>'author',''), 'agent'), NULLIF(CAST(@payload AS jsonb)->>'accepted_at','')::timestamptz) ON CONFLICT (id) DO UPDATE SET accepted_at = EXCLUDED.accepted_at, updated_at = now() WHERE item_plans.archived_at IS NULL RETURNING to_jsonb(item_plans)::jsonb AS data;
-- name: ArchiveItemPlan :one
UPDATE item_plans SET archived_at = now(), updated_at = now() WHERE id = CAST(@id AS text)::uuid AND archived_at IS NULL RETURNING to_jsonb(item_plans)::jsonb AS data;
