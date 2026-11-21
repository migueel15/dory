-- +goose Up
-- The thread whose agent sent a user message, such as a subagent handing its
-- result to its parent; empty when the user wrote it.
ALTER TABLE thread_messages ADD COLUMN sender_thread_id text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE thread_messages DROP COLUMN sender_thread_id;
