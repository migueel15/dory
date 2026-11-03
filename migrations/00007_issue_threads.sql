-- +goose Up
CREATE TABLE issue_threads (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid REFERENCES projects(id),
  repo text NOT NULL,
  number int NOT NULL,
  issue_url text NOT NULL,
  title text NOT NULL DEFAULT '',
  instance_id uuid NOT NULL REFERENCES t3_instances(id),
  thread_id text NOT NULL,
  model text NOT NULL DEFAULT '',
  effort text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE INDEX issue_threads_issue_idx ON issue_threads (repo, number);

-- +goose Down
DROP TABLE issue_threads;
