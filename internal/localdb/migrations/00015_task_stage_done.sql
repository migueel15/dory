-- +goose Up
-- A task can be marked done by hand when its work went straight to the
-- default branch and has no pull request. SQLite cannot change a CHECK, so the
-- table is rebuilt; the full text index keeps its rows because rowids are kept.
PRAGMA defer_foreign_keys = ON;
CREATE TABLE tasks_new (
  id text PRIMARY KEY,
  project_id text REFERENCES projects(id),
  repository_id text REFERENCES repositories(id),
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  stage text NOT NULL DEFAULT 'inbox' CHECK (stage IN ('inbox','ready','done','cancelled')),
  branch text NOT NULL DEFAULT '',
  issue_number integer,
  pr_number integer,
  position text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL,
  archived_at text
);
INSERT INTO tasks_new(rowid, id, project_id, repository_id, title, description, stage, branch, issue_number, pr_number, position, created_at, updated_at, archived_at)
  SELECT rowid, id, project_id, repository_id, title, description, stage, branch, issue_number, pr_number, position, created_at, updated_at, archived_at FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX tasks_project_idx ON tasks(project_id);
CREATE INDEX tasks_issue_idx ON tasks(repository_id, issue_number);
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

-- +goose Down
UPDATE tasks SET stage = 'ready' WHERE stage = 'done';
