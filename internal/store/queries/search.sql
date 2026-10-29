-- name: Search :many
SELECT entity_type, entity_id, content, rank FROM (
 SELECT 'item'::text entity_type, id::text entity_id, (to_jsonb(t) - 'search_vector')::jsonb content, ts_rank(search_vector, websearch_to_tsquery('simple', @query)) rank FROM items t WHERE archived_at IS NULL AND search_vector @@ websearch_to_tsquery('simple', @query)
 UNION ALL SELECT 'feature', id::text, (to_jsonb(t) - 'search_vector')::jsonb, ts_rank(search_vector, websearch_to_tsquery('simple', @query)) FROM features t WHERE archived_at IS NULL AND search_vector @@ websearch_to_tsquery('simple', @query)
 UNION ALL SELECT 'memory', id::text, (to_jsonb(t) - 'search_vector')::jsonb, ts_rank(search_vector, websearch_to_tsquery('simple', @query)) FROM memories t WHERE archived_at IS NULL AND search_vector @@ websearch_to_tsquery('simple', @query)
) hits ORDER BY rank DESC, entity_type, entity_id LIMIT CAST(@page_limit AS int) OFFSET CAST(@page_offset AS int);
