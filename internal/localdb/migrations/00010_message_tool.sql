-- +goose Up
-- The tool call of a tool message in Dory's own terms (runtime.ToolCall), as JSON.
ALTER TABLE thread_messages ADD COLUMN tool text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE thread_messages DROP COLUMN tool;
