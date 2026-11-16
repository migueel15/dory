-- +goose Up
-- Catalog of skills (requirement 5.8). targets says where each one goes:
-- {"nodes": [...], "harnesses": [...]}, an empty list meaning all.
CREATE TABLE skills (
  id text PRIMARY KEY,
  name text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  body_md text NOT NULL DEFAULT '',
  files text NOT NULL DEFAULT '{}' CHECK (json_type(files) = 'object'),
  version integer NOT NULL DEFAULT 1,
  targets text NOT NULL DEFAULT '{}' CHECK (json_type(targets) = 'object'),
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);

-- What each node really has installed, published by that node.
CREATE TABLE skill_states (
  skill_id text NOT NULL,
  node_id text NOT NULL,
  status text NOT NULL CHECK (status IN ('synced','pending','error','conflict','removed')),
  version integer NOT NULL DEFAULT 0,
  message text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text,
  PRIMARY KEY (skill_id, node_id)
);

-- +goose Down
DROP TABLE skill_states;
DROP TABLE skills;
