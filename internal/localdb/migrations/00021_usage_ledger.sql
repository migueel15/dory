-- +goose Up
-- One row per turn with the usage its provider reported, exact.
CREATE TABLE usage_turns (
    run_id      text PRIMARY KEY,
    thread_id   text NOT NULL,
    provider    text NOT NULL,
    model       text NOT NULL DEFAULT '',
    steps       integer NOT NULL,
    input       integer NOT NULL,
    cache_read  integer NOT NULL,
    cache_write integer NOT NULL,
    output      integer NOT NULL,
    reasoning   integer NOT NULL,
    created_at  text NOT NULL
);
CREATE INDEX usage_turns_created ON usage_turns(created_at);
CREATE INDEX usage_turns_thread ON usage_turns(thread_id);
CREATE TABLE usage_steps (
    run_id      text NOT NULL REFERENCES usage_turns(run_id) ON DELETE CASCADE,
    ordinal     integer NOT NULL,
    model       text NOT NULL,
    subagent    text NOT NULL,
    input       integer NOT NULL,
    cache_read  integer NOT NULL,
    cache_write integer NOT NULL,
    output      integer NOT NULL,
    reasoning   integer NOT NULL,
    PRIMARY KEY (run_id, ordinal)
);
-- The turn's tokens split by category, estimated from what each one put in the context.
CREATE TABLE usage_parts (
    run_id      text NOT NULL REFERENCES usage_turns(run_id) ON DELETE CASCADE,
    category    text NOT NULL,
    label       text NOT NULL,
    input       real NOT NULL,
    cache_read  real NOT NULL,
    cache_write real NOT NULL,
    output      real NOT NULL,
    PRIMARY KEY (run_id, category, label)
);

-- +goose Down
DROP TABLE usage_parts;
DROP TABLE usage_steps;
DROP TABLE usage_turns;
