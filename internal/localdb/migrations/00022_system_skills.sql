-- +goose Up
-- source 'system' marks the skills Dory ships in its binary: they cannot be
-- edited, only disabled or forked. system_version is the version of the
-- shipped content; binaries that predate it cannot change it, so a newer node
-- can tell their edits apart and restore the shipped skill. forked_from points
-- a fork to its system skill. disabled_at turns a skill off on every node.
ALTER TABLE skills ADD COLUMN source text NOT NULL DEFAULT '' CHECK (source IN ('', 'system'));
ALTER TABLE skills ADD COLUMN system_version integer NOT NULL DEFAULT 0;
ALTER TABLE skills ADD COLUMN forked_from text;
ALTER TABLE skills ADD COLUMN disabled_at text;

-- A node that ran an older binary stored the operations of these columns
-- but left them out of its rows. Take the latest value of each from the log.
UPDATE skills SET source = (SELECT json_extract(o.fields, '$.source') FROM ops o
  WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.source') = 'text' ORDER BY o.hlc DESC LIMIT 1)
WHERE EXISTS (SELECT 1 FROM ops o WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.source') = 'text');
UPDATE skills SET system_version = (SELECT json_extract(o.fields, '$.system_version') FROM ops o
  WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.system_version') = 'integer' ORDER BY o.hlc DESC LIMIT 1)
WHERE EXISTS (SELECT 1 FROM ops o WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.system_version') = 'integer');
UPDATE skills SET forked_from = (SELECT json_extract(o.fields, '$.forked_from') FROM ops o
  WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.forked_from') IS NOT NULL ORDER BY o.hlc DESC LIMIT 1)
WHERE EXISTS (SELECT 1 FROM ops o WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.forked_from') IS NOT NULL);
UPDATE skills SET disabled_at = (SELECT json_extract(o.fields, '$.disabled_at') FROM ops o
  WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.disabled_at') IS NOT NULL ORDER BY o.hlc DESC LIMIT 1)
WHERE EXISTS (SELECT 1 FROM ops o WHERE o.entity = 'skill' AND o.entity_id = skills.id AND json_type(o.fields, '$.disabled_at') IS NOT NULL);

-- +goose Down
ALTER TABLE skills DROP COLUMN disabled_at;
ALTER TABLE skills DROP COLUMN forked_from;
ALTER TABLE skills DROP COLUMN system_version;
ALTER TABLE skills DROP COLUMN source;
