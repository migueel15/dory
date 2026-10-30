-- +goose Up
ALTER TABLE items ADD COLUMN refs text[] NOT NULL DEFAULT '{}';

ALTER TABLE memories ADD COLUMN kind text NOT NULL DEFAULT 'context' CHECK (kind IN ('decision','lesson','context')),
  ADD COLUMN title text NOT NULL DEFAULT '';
UPDATE memories SET kind = 'decision' WHERE tags && ARRAY['decision','decisión','decisiones'];
UPDATE memories SET kind = 'lesson' WHERE kind = 'context' AND tags && ARRAY['lesson','leccion','lección','lecciones','calibracion','calibración'];
-- The title joins the full text search.
DROP INDEX memories_search_idx;
ALTER TABLE memories DROP COLUMN search_vector,
  ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || content)) STORED;
CREATE INDEX memories_search_idx ON memories USING gin(search_vector);

-- +goose Down
DROP INDEX memories_search_idx;
ALTER TABLE memories DROP COLUMN search_vector, DROP COLUMN title, DROP COLUMN kind,
  ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX memories_search_idx ON memories USING gin(search_vector);
ALTER TABLE items DROP COLUMN refs;
