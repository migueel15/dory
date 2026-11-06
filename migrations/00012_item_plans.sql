-- +goose Up
-- Versioned plans of an item: agents propose versions, only the user accepts one.
CREATE TABLE item_plans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  item_id uuid NOT NULL REFERENCES items(id),
  version int NOT NULL CHECK (version > 0),
  sections jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(sections) = 'object'),
  open_questions text[] NOT NULL DEFAULT '{}',
  author text NOT NULL DEFAULT 'agent' CHECK (author IN ('agent','user')),
  accepted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  UNIQUE (item_id, version)
);

-- +goose Down
DROP TABLE item_plans;
