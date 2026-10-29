-- +goose Up
CREATE TABLE projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'active',
  stack text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE TABLE repositories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id),
  remote_url text NOT NULL UNIQUE,
  default_branch text NOT NULL DEFAULT 'main',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE INDEX repositories_project_id_idx ON repositories (project_id);
CREATE TABLE environments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  os text NOT NULL DEFAULT '',
  access text NOT NULL DEFAULT '',
  agents_available text[] NOT NULL DEFAULT '{}',
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE TABLE repository_locations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  repository_id uuid NOT NULL REFERENCES repositories(id),
  environment_id uuid NOT NULL REFERENCES environments(id),
  path text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  UNIQUE (repository_id, environment_id)
);
CREATE INDEX repository_locations_repository_id_idx ON repository_locations (repository_id);
CREATE INDEX repository_locations_environment_id_idx ON repository_locations (environment_id);
CREATE TABLE features (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id),
  title text NOT NULL,
  body_md text NOT NULL DEFAULT '',
  code_refs text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'implemented' CHECK (status IN ('implemented','deprecated')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || body_md)) STORED
);
CREATE INDEX features_search_idx ON features USING gin(search_vector);
CREATE INDEX features_project_id_idx ON features (project_id);
CREATE TABLE items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid REFERENCES projects(id),
  raw_text text NOT NULL DEFAULT '',
  title text NOT NULL,
  body_md text NOT NULL DEFAULT '',
  tags text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'inbox' CHECK (status IN ('inbox','triaged','ready','in_progress','done','archived')),
  priority integer NOT NULL DEFAULT 0,
  source text NOT NULL DEFAULT 'manual',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || raw_text || ' ' || body_md)) STORED
);
CREATE INDEX items_search_idx ON items USING gin(search_vector);
CREATE INDEX items_project_id_idx ON items (project_id);
CREATE TABLE memories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope text NOT NULL CHECK (scope IN ('global','project','environment')),
  scope_id uuid,
  content text NOT NULL,
  tags text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  CHECK ((scope = 'global' AND scope_id IS NULL) OR (scope <> 'global' AND scope_id IS NOT NULL)),
  search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED
);
CREATE INDEX memories_search_idx ON memories USING gin(search_vector);
CREATE TABLE links (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  from_id uuid NOT NULL,
  from_type text NOT NULL CHECK (from_type IN ('project','repository','environment','feature','item','memory')),
  to_id uuid NOT NULL,
  to_type text NOT NULL CHECK (to_type IN ('project','repository','environment','feature','item','memory')),
  kind text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  UNIQUE (from_id, from_type, to_id, to_type, kind)
);
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  secret_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE TABLE api_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  name text NOT NULL,
  secret_hash text NOT NULL UNIQUE,
  permissions text[] NOT NULL CHECK (cardinality(permissions)>0 AND permissions <@ ARRAY['read','write','admin']::text[]),
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE INDEX api_tokens_user_id_idx ON api_tokens (user_id);
CREATE TABLE events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity_type text NOT NULL, entity_id uuid NOT NULL, actor text NOT NULL,
 change jsonb NOT NULL, at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX events_entity_idx ON events(entity_type, entity_id, id);
CREATE INDEX items_status_idx ON items(status, project_id);
-- +goose Down
DROP TABLE events, api_tokens, sessions, users, links, memories, items, features, repository_locations, environments, repositories, projects;
