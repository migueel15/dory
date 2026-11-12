-- Audit log. id is the local sequence (what SSE clients resume from); (device, seq)
-- is the global id that replication will use.
CREATE TABLE events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  device      TEXT NOT NULL,
  seq         INTEGER NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  actor       TEXT NOT NULL,
  change      TEXT NOT NULL CHECK (json_valid(change)),
  at          TEXT NOT NULL,
  UNIQUE (device, seq)
);
CREATE INDEX events_entity_idx ON events (entity_type, entity_id, id);

-- Full text search over items, memories, features and projects. Maintained by
-- the store on every write.
CREATE VIRTUAL TABLE search USING fts5(
  entity_type UNINDEXED, entity_id UNINDEXED, body,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
);
INSERT INTO search (entity_type, entity_id, body)
SELECT kind, id, trim(
  coalesce(json_extract(data, '$.title'), '') || ' ' ||
  coalesce(json_extract(data, '$.name'), '') || ' ' ||
  coalesce(json_extract(data, '$.slug'), '') || ' ' ||
  coalesce(json_extract(data, '$.description'), '') || ' ' ||
  coalesce(json_extract(data, '$.raw_text'), '') || ' ' ||
  coalesce(json_extract(data, '$.body_md'), '') || ' ' ||
  coalesce(json_extract(data, '$.content'), ''))
FROM records WHERE kind IN ('item', 'feature', 'memory', 'project') AND archived_at IS NULL;

-- Users, sessions and tokens never leave this device.
CREATE TABLE auth_records (
  kind         TEXT NOT NULL,
  id           TEXT NOT NULL,
  data         TEXT NOT NULL CHECK (json_valid(data)),
  field_clock  TEXT NOT NULL DEFAULT '{}',
  owner_device TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  archived_at  TEXT,
  PRIMARY KEY (kind, id)
) WITHOUT ROWID;
CREATE INDEX auth_records_kind_created_idx ON auth_records (kind, created_at, id);
INSERT INTO auth_records SELECT * FROM records WHERE kind IN ('user', 'session', 'token');
DELETE FROM records WHERE kind IN ('user', 'session', 'token');
