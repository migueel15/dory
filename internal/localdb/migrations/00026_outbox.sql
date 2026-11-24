-- +goose Up
-- Local to this node, not replicated: deliveries to other nodes that must not
-- be lost while the target is not connected. id is also the idempotency key
-- on the receiver.
CREATE TABLE outbox (
  id text PRIMARY KEY,
  node_id text NOT NULL,
  kind text NOT NULL,
  payload text NOT NULL CHECK (json_valid(payload)),
  status text NOT NULL DEFAULT 'pending',
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at text NOT NULL,
  last_error text NOT NULL DEFAULT '',
  created_at text NOT NULL,
  updated_at text NOT NULL
);
CREATE INDEX outbox_due ON outbox(status, next_attempt_at);

-- Local too: ids of the deliveries this node already applied, so a delivery
-- that arrives twice (a retry after a lost answer) changes nothing.
CREATE TABLE applied_deliveries (
  id text PRIMARY KEY,
  applied_at text NOT NULL
);

-- +goose Down
DROP TABLE applied_deliveries;
DROP TABLE outbox;
