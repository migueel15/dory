-- Document table shared by every resource kind. data holds the domain fields
-- without id and timestamps, which live in their own columns.
CREATE TABLE records (
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
CREATE INDEX records_kind_created_idx ON records (kind, created_at, id);
