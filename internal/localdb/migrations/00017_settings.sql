-- +goose Up
-- Replicated: settings shared by the whole mesh, such as how each kind of
-- notice is delivered. The id is the key of the setting.
CREATE TABLE settings (
  id text PRIMARY KEY,
  value text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);

-- +goose Down
DROP TABLE settings;
