-- +goose Up
ALTER TABLE thread_runtime ADD COLUMN last_turn_model text NOT NULL DEFAULT '{}';
UPDATE thread_runtime
SET last_turn_model = json_object(
    'provider', session_provider,
    'model', CASE WHEN json_extract(reported_settings, '$.Provider') = session_provider
                  THEN json_extract(reported_settings, '$.Configured.model') END,
    'reported_model', CASE WHEN json_extract(reported_settings, '$.Provider') = session_provider
                           THEN COALESCE(json_extract(reported_settings, '$.Reported.model'), '') ELSE '' END
)
WHERE session_id <> '' AND session_provider <> '';

-- +goose Down
ALTER TABLE thread_runtime DROP COLUMN last_turn_model;
