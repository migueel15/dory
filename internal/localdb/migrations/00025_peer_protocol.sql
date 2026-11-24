-- +goose Up
ALTER TABLE peers ADD COLUMN protocol integer NOT NULL DEFAULT 0;
ALTER TABLE peers ADD COLUMN last_error text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE peers DROP COLUMN last_error;
ALTER TABLE peers DROP COLUMN protocol;
