-- +goose Up
CREATE TABLE task_pulls (
    repo text NOT NULL,
    ref text NOT NULL,
    number integer,
    state text NOT NULL,
    draft integer NOT NULL DEFAULT 0,
    ci text NOT NULL DEFAULT '',
    review_decision text NOT NULL DEFAULT '',
    review_requests text NOT NULL DEFAULT '[]',
    data text NOT NULL DEFAULT '{}',
    updated_at text NOT NULL,
    PRIMARY KEY (repo, ref)
);

-- +goose Down
DROP TABLE task_pulls;
