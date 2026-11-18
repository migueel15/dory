-- +goose Up
-- Requests of an agent waiting for the user in the middle of a turn
-- (runtime.Request). They live while the harness process waits.
CREATE TABLE thread_requests (
    id text PRIMARY KEY,
    thread_id text NOT NULL,
    kind text NOT NULL,
    message_id text NOT NULL DEFAULT '',
    questions text NOT NULL DEFAULT '[]',
    created_at text NOT NULL
);
CREATE INDEX thread_requests_thread ON thread_requests(thread_id);

-- +goose Down
DROP TABLE thread_requests;
