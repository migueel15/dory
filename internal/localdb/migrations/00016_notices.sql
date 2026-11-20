-- +goose Up
-- Replicated: the inbox of notices. Every node sees the same notices and the
-- same read state; read ones are deleted locally 30 days after they were read.
CREATE TABLE notices (
  id text PRIMARY KEY,
  kind text NOT NULL DEFAULT '',
  title text NOT NULL DEFAULT '',
  body text NOT NULL DEFAULT '',
  url text NOT NULL DEFAULT '',
  read_at text,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);

-- +goose Down
DROP TABLE notices;
