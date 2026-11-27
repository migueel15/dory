-- +goose Up
-- external_id is the stable id of an adopted task in its provider (GitHub's
-- node_id for an issue). The cache is rebuilt by the next sync with that key.
ALTER TABLE tasks ADD COLUMN external_id text NOT NULL DEFAULT '';
CREATE INDEX tasks_source_external_idx ON tasks(source, external_id);

DROP TABLE external_tasks_cache;
CREATE TABLE external_tasks_cache (
    task_id text PRIMARY KEY,
    source text NOT NULL DEFAULT 'github',
    external_id text NOT NULL,
    repository_id text NOT NULL,
    repo text NOT NULL,
    number integer NOT NULL,
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
CREATE UNIQUE INDEX external_tasks_cache_key_idx ON external_tasks_cache(source, external_id);
CREATE INDEX external_tasks_cache_repo_number_idx ON external_tasks_cache(repository_id, number);

-- +goose Down
DROP INDEX tasks_source_external_idx;
ALTER TABLE tasks DROP COLUMN external_id;
