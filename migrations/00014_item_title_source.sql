-- +goose Up
-- Who wrote the title: pending (an agent is generating it), ai or user.
ALTER TABLE items ADD COLUMN IF NOT EXISTS title_source text NOT NULL DEFAULT 'user' CHECK (title_source IN ('pending','ai','user'));

-- +goose Down
ALTER TABLE items DROP COLUMN IF EXISTS title_source;
