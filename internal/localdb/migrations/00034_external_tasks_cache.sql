-- +goose Up
CREATE TABLE external_tasks_cache (
    task_id text PRIMARY KEY,
    source text NOT NULL DEFAULT 'github',
    repository_id text NOT NULL,
    repo text NOT NULL,
    number integer NOT NULL,
    node_id text NOT NULL DEFAULT '',
    title text NOT NULL DEFAULT '',
    body text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'open',
    state_reason text NOT NULL DEFAULT '',
    author text NOT NULL DEFAULT '',
    assignees text NOT NULL DEFAULT '[]',
    labels text NOT NULL DEFAULT '[]',
    label_colors text NOT NULL DEFAULT '{}',
    milestone text NOT NULL DEFAULT '',
    url text NOT NULL DEFAULT '',
    comments integer NOT NULL DEFAULT 0,
    closing_prs text NOT NULL DEFAULT '[]',
    created_at text NOT NULL DEFAULT '',
    updated_at text NOT NULL DEFAULT '',
    closed_at text NOT NULL DEFAULT '',
    cached_at text NOT NULL DEFAULT ''
);

CREATE INDEX external_tasks_cache_repo_state_idx ON external_tasks_cache(repository_id, state);
CREATE INDEX external_tasks_cache_repo_number_idx ON external_tasks_cache(repo, number);

-- +goose Down
DROP TABLE external_tasks_cache;
