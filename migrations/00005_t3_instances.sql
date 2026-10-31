-- +goose Up
CREATE TABLE t3_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  environment_id uuid NOT NULL REFERENCES environments(id),
  base_url text NOT NULL,
  token_env text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE UNIQUE INDEX t3_instances_environment_id_idx ON t3_instances (environment_id) WHERE archived_at IS NULL;
CREATE TABLE item_threads (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  item_id uuid NOT NULL REFERENCES items(id),
  instance_id uuid NOT NULL REFERENCES t3_instances(id),
  thread_id text NOT NULL,
  role text NOT NULL DEFAULT 'implementation' CHECK (role IN ('triage','implementation','review')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE UNIQUE INDEX item_threads_unique_idx ON item_threads (item_id, instance_id, thread_id) WHERE archived_at IS NULL;
CREATE INDEX item_threads_instance_thread_idx ON item_threads (instance_id, thread_id);

-- +goose Down
DROP TABLE item_threads, t3_instances;
