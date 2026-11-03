-- +goose Up
ALTER TABLE issue_threads ADD COLUMN kind text NOT NULL DEFAULT 'define_issue' CHECK (kind IN ('define_issue', 'resolve_conflicts'));

-- +goose Down
ALTER TABLE issue_threads DROP COLUMN kind;
