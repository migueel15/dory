-- +goose Up
ALTER TABLE node_state ADD COLUMN compaction_horizon text NOT NULL DEFAULT '{}';

CREATE TABLE peer_vectors (
  node_id text PRIMARY KEY REFERENCES peers(node_id) ON DELETE CASCADE,
  vector text NOT NULL,
  synced_at text NOT NULL
);

CREATE TABLE row_snapshots (
  entity text NOT NULL,
  entity_id text NOT NULL,
  columns text NOT NULL,
  merge text NOT NULL,
  horizon text NOT NULL,
  hlc text NOT NULL,
  PRIMARY KEY (entity, entity_id)
);

-- Keep original clocks for late concurrent edits. Materialization can read
-- just the range above the winning edit's base.
CREATE TABLE row_snapshot_versions (
  entity text NOT NULL,
  entity_id text NOT NULL,
  hlc text NOT NULL,
  node_id text NOT NULL,
  seq integer NOT NULL,
  base text NOT NULL,
  text text NOT NULL,
  PRIMARY KEY (entity, entity_id, hlc, node_id, seq)
) WITHOUT ROWID;
