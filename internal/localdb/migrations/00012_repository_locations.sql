-- +goose Up
-- Replicated: where each node keeps a repository, so every node knows the
-- environments of a project. repository_paths stays as the local copy.
CREATE TABLE repository_locations (
  repository_id text NOT NULL,
  node_id text NOT NULL,
  path text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text,
  PRIMARY KEY (repository_id, node_id)
);

-- +goose Down
DROP TABLE repository_locations;
