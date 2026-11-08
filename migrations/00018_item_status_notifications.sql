-- +goose Up
-- Every item status can notify. The new events (inbox, triaged, in_progress,
-- validating, done, cancelled) start disabled until the user enables them;
-- item_ready, item_blocked and the thread events keep their enabled default.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION emit_notification(p_title text, p_body text, p_url text, p_kinds text[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u record; k text; setting jsonb; b boolean; t boolean; p boolean; n bigint;
BEGIN
 FOR u IN SELECT users.id, COALESCE(settings, '{}'::jsonb) settings FROM users LEFT JOIN notification_preferences ON user_id=users.id WHERE archived_at IS NULL LOOP
 b := false; t := false; p := false;
 FOREACH k IN ARRAY p_kinds LOOP
 setting := u.settings->k;
 IF COALESCE((setting->>'enabled')::boolean, k IN ('item_ready','item_blocked','thread_finished','thread_attention')) THEN
 b := b OR COALESCE((setting->>'bell')::boolean,true);
 t := t OR COALESCE((setting->>'toast')::boolean,true);
 p := p OR COALESCE((setting->>'push')::boolean,true);
 END IF;
 END LOOP;
 IF b OR t OR p THEN
 INSERT INTO notifications(user_id,title,body,url,kinds,bell,toast,push) VALUES(u.id,p_title,notification_detail(p_body),p_url,p_kinds,b,t,p) RETURNING id INTO n;
 IF p THEN
 INSERT INTO notification_push(notification_id,endpoint) SELECT n,endpoint FROM notification_subscriptions WHERE user_id=u.id;
 END IF;
 END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_item_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pname text;
BEGIN
 IF NEW.archived_at IS NULL AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM NEW.status) THEN
 SELECT name INTO pname FROM projects WHERE id=NEW.project_id;
 PERFORM emit_notification(CASE NEW.status
   WHEN 'inbox' THEN 'Tarea en bandeja'
   WHEN 'triaged' THEN 'Tarea clasificada'
   WHEN 'ready' THEN 'Tarea lista'
   WHEN 'in_progress' THEN 'Tarea en curso'
   WHEN 'validating' THEN 'Tarea en validación'
   WHEN 'blocked' THEN 'Tarea bloqueada'
   WHEN 'done' THEN 'Tarea hecha'
   WHEN 'cancelled' THEN 'Tarea cancelada'
   ELSE 'Tarea: ' || NEW.status END || COALESCE(' · ' || pname, ''),
  NEW.title, '/inbox?notification-item=' || NEW.id, ARRAY['item_' || NEW.status]);
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_item_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pname text;
BEGIN
 IF NEW.archived_at IS NULL AND NEW.status IN ('ready','blocked') AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM NEW.status) THEN
 SELECT name INTO pname FROM projects WHERE id=NEW.project_id;
 PERFORM emit_notification(CASE NEW.status WHEN 'ready' THEN 'Tarea lista' ELSE 'Tarea bloqueada' END || COALESCE(' · ' || pname, ''),
 NEW.title, '/inbox?notification-item=' || NEW.id, ARRAY['item_' || NEW.status]);
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
