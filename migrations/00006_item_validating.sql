-- +goose Up
ALTER TABLE items DROP CONSTRAINT items_status_check;
ALTER TABLE items ADD CONSTRAINT items_status_check CHECK (status IN ('inbox','triaged','ready','in_progress','validating','blocked','done','cancelled'));

-- +goose Down
UPDATE items SET status = 'in_progress' WHERE status = 'validating';
ALTER TABLE items DROP CONSTRAINT items_status_check;
ALTER TABLE items ADD CONSTRAINT items_status_check CHECK (status IN ('inbox','triaged','ready','in_progress','blocked','done','cancelled'));
