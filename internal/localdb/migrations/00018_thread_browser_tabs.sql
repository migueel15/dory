-- +goose Up
-- Local, not replicated: the browser tabs of each thread of this node. The
-- page itself lives in a browser engine; this keeps what reopens it.
CREATE TABLE thread_browser_tabs (
  id text PRIMARY KEY,
  thread_id text NOT NULL,
  url text NOT NULL DEFAULT 'about:blank',
  title text NOT NULL DEFAULT '',
  position integer NOT NULL DEFAULT 0,
  active integer NOT NULL DEFAULT 0,
  created_at text NOT NULL
);
CREATE INDEX thread_browser_tabs_thread ON thread_browser_tabs(thread_id, position);

-- +goose Down
DROP TABLE thread_browser_tabs;
