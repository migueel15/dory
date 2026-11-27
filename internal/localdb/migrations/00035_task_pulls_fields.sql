-- +goose Up
ALTER TABLE task_pulls ADD COLUMN closing_issues text NOT NULL DEFAULT '[]';
ALTER TABLE task_pulls ADD COLUMN head_ref text NOT NULL DEFAULT '';
ALTER TABLE task_pulls ADD COLUMN author text NOT NULL DEFAULT '';
ALTER TABLE task_pulls ADD COLUMN merged_at text NOT NULL DEFAULT '';

CREATE INDEX task_pulls_repo_number_idx ON task_pulls(repo, number);

-- +goose Down
DROP INDEX IF EXISTS task_pulls_repo_number_idx;
