-- name: AppendEvent :one
INSERT INTO events(entity_type, entity_id, actor, change) VALUES (@entity_type, CAST(@entity_id AS text)::uuid, @actor, @change) RETURNING *;
-- name: ListEvents :many
SELECT * FROM events WHERE id > @after_id AND (CAST(@entity_type AS text) = '' OR entity_type = @entity_type) ORDER BY id LIMIT @page_limit;
