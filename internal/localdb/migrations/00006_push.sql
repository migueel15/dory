-- +goose Up
-- Local: browsers and phones that receive push notices from this node.
CREATE TABLE push_subscriptions (
  endpoint text PRIMARY KEY,
  subscription text NOT NULL,
  created_at text NOT NULL
);

-- +goose Down
DROP TABLE push_subscriptions;
