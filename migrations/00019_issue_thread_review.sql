-- +goose Up
ALTER TABLE issue_threads DROP CONSTRAINT issue_threads_kind_check;
ALTER TABLE issue_threads ADD CONSTRAINT issue_threads_kind_check CHECK (kind IN ('define_issue', 'resolve_conflicts', 'review'));

-- +goose Down
DELETE FROM issue_threads WHERE kind = 'review';
ALTER TABLE issue_threads DROP CONSTRAINT issue_threads_kind_check;
ALTER TABLE issue_threads ADD CONSTRAINT issue_threads_kind_check CHECK (kind IN ('define_issue', 'resolve_conflicts'));
