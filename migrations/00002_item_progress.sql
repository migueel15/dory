-- +goose Up
ALTER TABLE items ADD COLUMN progress text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE items DROP COLUMN progress;
