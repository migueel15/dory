-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;
CREATE TABLE tasks_new (
  id text PRIMARY KEY,
  source text NOT NULL DEFAULT 'internal',
  project_id text REFERENCES projects(id),
  repository_id text REFERENCES repositories(id),
  number integer,
  title text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  stage text NOT NULL DEFAULT 'inbox' CHECK (stage IN ('','inbox','ready','done','cancelled')),
  branch text NOT NULL DEFAULT '',
  issue_number integer,
  pr_number integer,
  position text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
INSERT INTO tasks_new(rowid, id, source, project_id, repository_id, number, title, description, stage, branch, issue_number, pr_number, position, created_at, updated_at, archived_at)
  SELECT rowid, id, 'internal', project_id, repository_id, NULL, title, description, stage, branch, issue_number, pr_number, position, created_at, updated_at, archived_at FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX tasks_project_idx ON tasks(project_id);
CREATE INDEX tasks_issue_idx ON tasks(repository_id, issue_number);
CREATE INDEX tasks_source_number_idx ON tasks(source, repository_id, number);
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

CREATE TABLE thread_pulls (
  thread_id text NOT NULL REFERENCES threads(id),
  repository_id text NOT NULL REFERENCES repositories(id),
  number integer NOT NULL,
  branch text NOT NULL DEFAULT '',
  linked_by text NOT NULL DEFAULT 'manual',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text,
  PRIMARY KEY (thread_id, repository_id, number)
);
CREATE INDEX thread_pulls_thread_idx ON thread_pulls(thread_id);
CREATE INDEX thread_pulls_repo_number_idx ON thread_pulls(repository_id, number);
COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
DROP TABLE IF EXISTS thread_pulls;
