-- +goose Up
-- source 'system' marks the skills Dory ships in its binary: they cannot be
-- edited, only disabled or forked. forked_from points a fork to its system
-- skill. disabled_at turns a skill off on every node without archiving it.
ALTER TABLE skills ADD COLUMN source text NOT NULL DEFAULT '' CHECK (source IN ('', 'system'));
ALTER TABLE skills ADD COLUMN forked_from text;
ALTER TABLE skills ADD COLUMN disabled_at text;

-- +goose Down
ALTER TABLE skills DROP COLUMN disabled_at;
ALTER TABLE skills DROP COLUMN forked_from;
ALTER TABLE skills DROP COLUMN source;
