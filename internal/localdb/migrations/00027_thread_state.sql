-- +goose Up
CREATE TABLE thread_state (
  thread_id text PRIMARY KEY,
  node_id text NOT NULL,
  status text NOT NULL DEFAULT 'idle' CHECK (status IN ('idle','working','waiting','error')),
  error text NOT NULL DEFAULT '',
  last_activity_at text,
  updated_at text NOT NULL
);

INSERT INTO thread_state (thread_id, node_id, status, last_activity_at, updated_at)
SELECT id, node_id, status, last_activity_at, updated_at FROM threads;

-- A peer may have seen a sequence that disappears from the local tail.
ALTER TABLE node_state ADD COLUMN op_seq_floor integer NOT NULL DEFAULT 0;
UPDATE node_state SET op_seq_floor = COALESCE((SELECT MAX(seq) FROM ops WHERE node_id = node_state.node_id), 0);

UPDATE ops SET fields = json_remove(fields, '$.status', '$.last_activity_at')
WHERE entity = 'thread'
  AND (json_type(fields, '$.status') IS NOT NULL OR json_type(fields, '$.last_activity_at') IS NOT NULL);
DELETE FROM ops AS o WHERE entity = 'thread'
  AND NOT EXISTS (SELECT 1 FROM json_each(o.fields) WHERE key <> 'updated_at');

-- Keep materialized metadata in step with the surviving operations.
UPDATE threads SET updated_at = COALESCE((
  SELECT json_extract(fields, '$.updated_at')
  FROM ops
  WHERE entity = 'thread' AND entity_id = threads.id AND json_type(fields, '$.updated_at') = 'text'
  ORDER BY hlc DESC LIMIT 1
), updated_at);

ALTER TABLE threads DROP COLUMN status;
ALTER TABLE threads DROP COLUMN last_activity_at;
