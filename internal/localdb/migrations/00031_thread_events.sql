-- +goose Up
-- Append-only, numbered record of what happened in a thread. thread_messages
-- is the read model derived from it.
CREATE TABLE thread_events (
    thread_id text NOT NULL,
    seq integer NOT NULL,
    run_id text NOT NULL DEFAULT '',
    type text NOT NULL,
    item_id text NOT NULL DEFAULT '',
    payload text NOT NULL CHECK (json_valid(payload)),
    created_at text NOT NULL,
    PRIMARY KEY (thread_id, seq)
);

-- Every existing message becomes one event, in the order of the conversation.
INSERT INTO thread_events(thread_id, seq, run_id, type, item_id, payload, created_at)
SELECT thread_id,
       ROW_NUMBER() OVER (PARTITION BY thread_id ORDER BY seq),
       run_id,
       CASE WHEN role IN ('clear', 'interrupt', 'model-change', 'changes') THEN 'marker' ELSE 'item.completed' END,
       id,
       json_object('id', id, 'thread_id', thread_id, 'run_id', run_id, 'role', role, 'text', text, 'title', title,
                   'detail', detail, 'status', status, 'created_at', created_at,
                   'sender_thread_id', sender_thread_id,
                   'tool', CASE WHEN tool <> '' AND json_valid(tool) THEN json(tool) END),
       created_at
FROM thread_messages;

-- +goose Down
DROP TABLE thread_events;
