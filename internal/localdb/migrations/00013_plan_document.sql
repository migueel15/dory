-- +goose Up
-- The plan of a task is one Markdown document edited in place by the user and
-- the agents; revision counts its saves so a stale edit can be refused.
ALTER TABLE plans ADD COLUMN body_md text NOT NULL DEFAULT '';
ALTER TABLE plans ADD COLUMN revision integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE plans DROP COLUMN revision;
ALTER TABLE plans DROP COLUMN body_md;
