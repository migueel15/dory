-- +goose Up
ALTER TABLE item_threads DROP CONSTRAINT item_threads_role_check;
ALTER TABLE item_threads ADD CONSTRAINT item_threads_role_check CHECK (role IN ('triage','implementation','review','replan','refine'));

-- +goose Down
DELETE FROM item_threads WHERE role = 'refine';
ALTER TABLE item_threads DROP CONSTRAINT item_threads_role_check;
ALTER TABLE item_threads ADD CONSTRAINT item_threads_role_check CHECK (role IN ('triage','implementation','review','replan'));
