-- +goose Up
-- The thread whose agent created this one with thread_create; it is only
-- where the thread came from, unlike parent_id, which makes a subagent.
ALTER TABLE threads ADD COLUMN origin_id text;

-- Local to the parent's node: each piece of work a thread delegated to a
-- subagent, with its outcome. A follow-up turn of the child is a new row.
CREATE TABLE thread_delegations (
  id text PRIMARY KEY,
  parent_thread_id text NOT NULL,
  child_thread_id text NOT NULL,
  child_node_id text NOT NULL,
  title text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'running',
  result text NOT NULL DEFAULT '',
  delivered integer NOT NULL DEFAULT 0,
  created_at text NOT NULL,
  updated_at text NOT NULL
);
CREATE INDEX thread_delegations_parent ON thread_delegations(parent_thread_id, created_at);
CREATE INDEX thread_delegations_child ON thread_delegations(child_thread_id, created_at);

-- +goose Down
DROP TABLE thread_delegations;
ALTER TABLE threads DROP COLUMN origin_id;
