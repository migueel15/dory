-- +goose Up
-- Messages waiting for the running turn of a thread. The runner loads them
-- when it starts, so a restart keeps them; they do not run on their own.
CREATE TABLE thread_queue (
    thread_id text NOT NULL,
    run_id text NOT NULL,
    position integer NOT NULL,
    text text NOT NULL DEFAULT '',
    files text NOT NULL DEFAULT '[]',
    sender_thread_id text NOT NULL DEFAULT '',
    created_at text NOT NULL,
    PRIMARY KEY (thread_id, run_id)
);
CREATE INDEX thread_queue_position ON thread_queue(thread_id, position);

-- +goose Down
DROP TABLE thread_queue;
