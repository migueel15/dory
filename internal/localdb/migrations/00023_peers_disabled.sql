-- +goose Up
-- Local: a paired node can be paused without losing the pairing.
ALTER TABLE peers ADD COLUMN disabled_at text;

-- +goose Down
ALTER TABLE peers DROP COLUMN disabled_at;
