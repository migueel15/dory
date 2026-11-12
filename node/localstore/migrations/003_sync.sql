-- Operation log: every replicated change, local or received. (device_id, seq)
-- is the global id; seq is gapless per device.
CREATE TABLE ops (
  device_id   TEXT NOT NULL,
  seq         INTEGER NOT NULL,
  hlc         TEXT NOT NULL,
  entity      TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  kind        TEXT NOT NULL,
  fields_json TEXT NOT NULL DEFAULT '',
  schema_v    INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (device_id, seq)
) WITHOUT ROWID;
CREATE INDEX ops_hlc_idx ON ops (hlc);
CREATE INDEX ops_entity_idx ON ops (entity, entity_id);

-- Array fields as add-wins sets: add tags and the tags removed by tombstones.
CREATE TABLE set_elems (
  kind  TEXT NOT NULL,
  id    TEXT NOT NULL,
  field TEXT NOT NULL,
  elem  TEXT NOT NULL,
  adds  TEXT NOT NULL DEFAULT '[]',
  rems  TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (kind, id, field, elem)
) WITHOUT ROWID;

-- The slug each project asked for and when; the stored slug is derived from it.
CREATE TABLE sync_slugs (
  id     TEXT PRIMARY KEY,
  intent TEXT NOT NULL,
  rank   TEXT NOT NULL
) WITHOUT ROWID;
