-- +goose Up
ALTER TABLE thread_runtime ADD COLUMN reported_settings text NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE thread_runtime DROP COLUMN reported_settings;
