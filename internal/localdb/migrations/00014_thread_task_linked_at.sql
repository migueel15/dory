-- +goose Up
-- When a thread was linked to its task, so a task can show its latest thread.
-- Threads already linked keep their creation time as the best known answer.
ALTER TABLE threads ADD COLUMN task_linked_at text;
UPDATE threads SET task_linked_at = created_at WHERE task_id IS NOT NULL;

-- +goose Down
ALTER TABLE threads DROP COLUMN task_linked_at;
