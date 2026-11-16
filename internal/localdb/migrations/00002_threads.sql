-- +goose Up
-- A thread is a conversation with an agent. Its organisation (title, pin,
-- read state, links) replicates; its conversation and processes stay on the
-- node that owns it (node_id).
CREATE TABLE threads (
  id text PRIMARY KEY,
  node_id text NOT NULL,
  project_id text REFERENCES projects(id),
  task_id text REFERENCES tasks(id),
  parent_id text REFERENCES threads(id),
  provider text NOT NULL,
  model text NOT NULL DEFAULT '',
  effort text NOT NULL DEFAULT '',
  runtime_mode text NOT NULL DEFAULT 'full-access' CHECK (runtime_mode IN ('full-access','approval-required')),
  title text NOT NULL DEFAULT '',
  branch text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'idle' CHECK (status IN ('idle','working','waiting','error')),
  pinned_at text,
  read_at text,
  last_activity_at text,
  snoozed_until text,
  settled_at text,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
CREATE INDEX threads_task_idx ON threads(task_id);
CREATE INDEX threads_project_idx ON threads(project_id);

-- Local: how this node runs each of its threads.
CREATE TABLE thread_runtime (
  thread_id text PRIMARY KEY REFERENCES threads(id),
  session_id text NOT NULL DEFAULT '',
  worktree_path text NOT NULL DEFAULT '',
  base_branch text NOT NULL DEFAULT ''
);

-- Local: the conversation, as normalized messages. seq orders them and lets
-- clients ask for what changed since a point.
CREATE TABLE thread_messages (
  seq integer PRIMARY KEY AUTOINCREMENT,
  id text NOT NULL UNIQUE,
  thread_id text NOT NULL REFERENCES threads(id),
  run_id text NOT NULL DEFAULT '',
  role text NOT NULL,
  text text NOT NULL DEFAULT '',
  title text NOT NULL DEFAULT '',
  detail text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT '',
  streaming integer NOT NULL DEFAULT 0,
  created_at text NOT NULL,
  updated_at text NOT NULL
);
CREATE INDEX thread_messages_thread_idx ON thread_messages(thread_id, seq);

-- +goose Down
DROP TABLE thread_messages;
DROP TABLE thread_runtime;
DROP TABLE threads;
