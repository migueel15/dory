-- +goose Up
-- Base schema of a Dory v2 node. Ids are UUID text, times are UTC RFC 3339
-- text and lists are JSON arrays. Replicated tables only change through the
-- operation log; tables marked local never leave this node.

CREATE TABLE projects (
  id text PRIMARY KEY,
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);

CREATE TABLE repositories (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id),
  remote_url text NOT NULL UNIQUE,
  default_branch text NOT NULL DEFAULT 'main',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
CREATE INDEX repositories_project_idx ON repositories(project_id);

-- Local: where this node keeps each repository cloned.
CREATE TABLE repository_paths (
  repository_id text PRIMARY KEY REFERENCES repositories(id),
  path text NOT NULL,
  updated_at text NOT NULL
);

-- A task is one piece of work with its branch and pull request. Several tasks
-- can share an issue. The kanban column is derived from threads and the pull
-- request; stage only keeps what the user decides before that: inbox, ready
-- or cancelled.
CREATE TABLE tasks (
  id text PRIMARY KEY,
  project_id text REFERENCES projects(id),
  repository_id text REFERENCES repositories(id),
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  stage text NOT NULL DEFAULT 'inbox' CHECK (stage IN ('inbox','ready','cancelled')),
  branch text NOT NULL DEFAULT '',
  issue_number integer,
  pr_number integer,
  position text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
CREATE INDEX tasks_project_idx ON tasks(project_id);
CREATE INDEX tasks_issue_idx ON tasks(repository_id, issue_number);

-- task_id cannot start until depends_on_id is done.
CREATE TABLE task_dependencies (
  task_id text NOT NULL REFERENCES tasks(id),
  depends_on_id text NOT NULL REFERENCES tasks(id),
  created_at text NOT NULL,
  archived_at text,
  PRIMARY KEY (task_id, depends_on_id),
  CHECK (task_id <> depends_on_id)
);

-- Versioned plan of a task: agents propose versions, only the user accepts one.
CREATE TABLE plans (
  id text PRIMARY KEY,
  task_id text NOT NULL REFERENCES tasks(id),
  version integer NOT NULL CHECK (version > 0),
  sections text NOT NULL DEFAULT '{}' CHECK (json_type(sections) = 'object'),
  open_questions text NOT NULL DEFAULT '[]' CHECK (json_type(open_questions) = 'array'),
  author text NOT NULL DEFAULT 'agent' CHECK (author IN ('agent','user')),
  accepted_at text,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text,
  UNIQUE (task_id, version)
);

-- scope_id is the project id or the node id; empty for global memories.
CREATE TABLE memories (
  id text PRIMARY KEY,
  scope text NOT NULL CHECK (scope IN ('global','project','node')),
  scope_id text NOT NULL DEFAULT '',
  kind text NOT NULL DEFAULT 'context' CHECK (kind IN ('decision','lesson','context')),
  title text NOT NULL DEFAULT '',
  content text NOT NULL,
  labels text NOT NULL DEFAULT '[]' CHECK (json_type(labels) = 'array'),
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text,
  CHECK ((scope = 'global') = (scope_id = ''))
);
CREATE INDEX memories_scope_idx ON memories(scope, scope_id);

-- Full text search over tasks and memories, kept in step by triggers.
CREATE VIRTUAL TABLE tasks_fts USING fts5(title, description, content='tasks', content_rowid='rowid');
CREATE VIRTUAL TABLE memories_fts USING fts5(title, content, content='memories', content_rowid='rowid');
-- +goose StatementBegin
CREATE TRIGGER tasks_fts_insert AFTER INSERT ON tasks BEGIN
  INSERT INTO tasks_fts(rowid, title, description) VALUES (new.rowid, new.title, new.description);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER tasks_fts_update AFTER UPDATE ON tasks BEGIN
  INSERT INTO tasks_fts(tasks_fts, rowid, title, description) VALUES ('delete', old.rowid, old.title, old.description);
  INSERT INTO tasks_fts(rowid, title, description) VALUES (new.rowid, new.title, new.description);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories BEGIN
  INSERT INTO memories_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER memories_fts_update AFTER UPDATE ON memories BEGIN
  INSERT INTO memories_fts(memories_fts, rowid, title, content) VALUES ('delete', old.rowid, old.title, old.content);
  INSERT INTO memories_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END;
-- +goose StatementEnd

-- Local: accounts of this node.
CREATE TABLE users (
  id text PRIMARY KEY,
  username text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
CREATE TABLE sessions (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id),
  secret_hash text NOT NULL UNIQUE,
  expires_at text NOT NULL,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
CREATE TABLE api_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id),
  name text NOT NULL,
  secret_hash text NOT NULL UNIQUE,
  permissions text NOT NULL CHECK (json_type(permissions) = 'array' AND json_array_length(permissions) > 0),
  expires_at text,
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);

-- Operation log: every change to a replicated row, in the order of its
-- origin node. fields holds only the changed columns. Ops from other nodes are
-- stored as received so they can be passed on.
CREATE TABLE ops (
  node_id text NOT NULL,
  seq integer NOT NULL CHECK (seq > 0),
  hlc text NOT NULL,
  entity text NOT NULL,
  entity_id text NOT NULL,
  fields text NOT NULL CHECK (json_type(fields) = 'object'),
  PRIMARY KEY (node_id, seq)
);
CREATE INDEX ops_entity_idx ON ops(entity, entity_id, hlc);

-- Local: identity and clock of this node (one row).
CREATE TABLE node_state (
  id integer PRIMARY KEY CHECK (id = 1),
  node_id text NOT NULL,
  hlc text NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE node_state;
DROP TABLE ops;
DROP TABLE api_tokens;
DROP TABLE sessions;
DROP TABLE users;
DROP TRIGGER memories_fts_update;
DROP TRIGGER memories_fts_insert;
DROP TRIGGER tasks_fts_update;
DROP TRIGGER tasks_fts_insert;
DROP TABLE memories_fts;
DROP TABLE tasks_fts;
DROP TABLE memories;
DROP TABLE plans;
DROP TABLE task_dependencies;
DROP TABLE tasks;
DROP TABLE repository_paths;
DROP TABLE repositories;
DROP TABLE projects;
