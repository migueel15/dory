-- +goose Up
-- System messages carry a stable code and its parameters; the web writes the text.
ALTER TABLE thread_messages ADD COLUMN code text NOT NULL DEFAULT '';
ALTER TABLE thread_messages ADD COLUMN params text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE thread_messages DROP COLUMN params;
ALTER TABLE thread_messages DROP COLUMN code;
