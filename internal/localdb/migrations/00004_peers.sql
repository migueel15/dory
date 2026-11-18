-- +goose Up
-- Local: the nodes this one is paired with (requirement RF-05). A node is
-- recognised by its public key; revoking one stops its sync.
CREATE TABLE peers (
  node_id text PRIMARY KEY,
  name text NOT NULL DEFAULT '',
  public_key text NOT NULL UNIQUE,
  address text NOT NULL DEFAULT '',
  info text NOT NULL DEFAULT '{}',
  paired_at text NOT NULL,
  last_seen text,
  revoked_at text
);

-- +goose Down
DROP TABLE peers;
