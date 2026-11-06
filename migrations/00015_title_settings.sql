-- +goose Up
-- Global settings of the AI title generation, edited from the web (one row).
CREATE TABLE IF NOT EXISTS title_settings (
 id boolean PRIMARY KEY DEFAULT true CHECK (id),
 settings jsonb NOT NULL DEFAULT '{}'
);

-- +goose Down
DROP TABLE IF EXISTS title_settings;
