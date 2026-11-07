-- +goose Up
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_title_source_check;
ALTER TABLE items ADD CONSTRAINT items_title_source_check CHECK (title_source IN ('pending','ai','user','fallback'));

-- +goose Down
UPDATE items SET title_source = 'user' WHERE title_source = 'fallback';
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_title_source_check;
ALTER TABLE items ADD CONSTRAINT items_title_source_check CHECK (title_source IN ('pending','ai','user'));
