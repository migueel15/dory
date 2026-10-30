-- +goose Up
-- ADR-001: states live in closed-value fields instead of text conventions.
ALTER TABLE features DROP CONSTRAINT features_status_check;
UPDATE features SET status = 'in_progress', title = btrim(substr(title, length('[en curso]') + 1)), updated_at = now()
  WHERE title LIKE '[en curso]%';
ALTER TABLE features ALTER COLUMN status SET DEFAULT 'planned',
  ADD CONSTRAINT features_status_check CHECK (status IN ('planned','in_progress','implemented','deprecated'));

ALTER TABLE items DROP CONSTRAINT items_status_check;
ALTER TABLE items ADD COLUMN feature_id uuid REFERENCES features(id),
  ADD COLUMN blocked_reason text NOT NULL DEFAULT '';
-- archived is no longer a status: cancelled items keep their meaning, the rest stay hidden as done.
UPDATE items SET status = 'cancelled', tags = array_remove(tags, 'cancelado') WHERE status = 'archived' AND 'cancelado' = ANY(tags);
UPDATE items SET status = 'done', archived_at = COALESCE(archived_at, updated_at) WHERE status = 'archived';
UPDATE items SET status = 'blocked', blocked_reason = 'Ver descripción', tags = array_remove(tags, 'bloqueado')
  WHERE 'bloqueado' = ANY(tags) AND status NOT IN ('done','cancelled');
ALTER TABLE items ADD CONSTRAINT items_status_check CHECK (status IN ('inbox','triaged','ready','in_progress','blocked','done','cancelled')),
  ADD CONSTRAINT items_blocked_reason_check CHECK ((status = 'blocked') = (blocked_reason <> ''));
CREATE INDEX items_feature_idx ON items(feature_id);

-- part_of links become feature_id; the oldest link wins if there are several.
UPDATE items i SET feature_id = l.to_id FROM (
  SELECT DISTINCT ON (from_id) from_id, to_id FROM links
  WHERE from_type = 'item' AND to_type = 'feature' AND kind = 'part_of' AND archived_at IS NULL
  ORDER BY from_id, created_at
) l WHERE i.id = l.from_id;
UPDATE links SET archived_at = now(), updated_at = now()
  WHERE from_type = 'item' AND to_type = 'feature' AND kind = 'part_of' AND archived_at IS NULL;

-- +goose Down
DROP INDEX items_feature_idx;
ALTER TABLE items DROP CONSTRAINT items_blocked_reason_check, DROP CONSTRAINT items_status_check;
UPDATE items SET status = 'in_progress', tags = array_append(tags, 'bloqueado') WHERE status = 'blocked';
UPDATE items SET status = 'archived', tags = array_append(tags, 'cancelado') WHERE status = 'cancelled';
ALTER TABLE items DROP COLUMN blocked_reason, DROP COLUMN feature_id,
  ADD CONSTRAINT items_status_check CHECK (status IN ('inbox','triaged','ready','in_progress','done','archived'));
ALTER TABLE features DROP CONSTRAINT features_status_check;
UPDATE features SET title = '[en curso] ' || title WHERE status = 'in_progress';
UPDATE features SET status = 'implemented' WHERE status IN ('planned','in_progress');
ALTER TABLE features ALTER COLUMN status SET DEFAULT 'implemented',
  ADD CONSTRAINT features_status_check CHECK (status IN ('implemented','deprecated'));
