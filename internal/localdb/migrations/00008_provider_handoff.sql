-- +goose Up
ALTER TABLE thread_runtime ADD COLUMN session_provider text NOT NULL DEFAULT '';
ALTER TABLE thread_runtime ADD COLUMN portable_history integer NOT NULL DEFAULT 0;
UPDATE thread_runtime
SET session_provider = (SELECT provider FROM threads WHERE threads.id = thread_runtime.thread_id);

UPDATE thread_runtime SET reported_settings = json_set(reported_settings, '$.Provider', session_provider);

-- +goose Down
UPDATE thread_runtime SET reported_settings = json_remove(reported_settings, '$.Provider');
ALTER TABLE thread_runtime DROP COLUMN portable_history;
ALTER TABLE thread_runtime DROP COLUMN session_provider;
