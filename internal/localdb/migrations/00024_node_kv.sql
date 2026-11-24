-- +goose Up
CREATE TABLE node_kv (
    key text PRIMARY KEY,
    value text NOT NULL CHECK (json_valid(value)),
    updated_at text NOT NULL
);

-- +goose Down
DROP TABLE node_kv;
