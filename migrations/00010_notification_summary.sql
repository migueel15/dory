-- +goose Up
ALTER TABLE notifications ADD COLUMN body text NOT NULL DEFAULT '';
-- +goose StatementBegin
CREATE FUNCTION notification_detail(p text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN char_length(l) > 80 THEN left(l, 79) || '…' ELSE l END
 FROM (SELECT btrim(split_part(btrim(COALESCE(p,''), E' \n\r\t'), E'\n', 1)) AS l) s
$$;
-- +goose StatementEnd
DROP TRIGGER item_notification ON items;
DROP FUNCTION notify_item_transition();
DROP FUNCTION emit_notification(text,text,text[]);
-- +goose StatementBegin
CREATE FUNCTION emit_notification(p_title text, p_body text, p_url text, p_kinds text[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u record; k text; setting jsonb; b boolean; t boolean; p boolean; n bigint;
BEGIN
 FOR u IN SELECT users.id, COALESCE(settings, '{}'::jsonb) settings FROM users LEFT JOIN notification_preferences ON user_id=users.id WHERE archived_at IS NULL LOOP
 b := false; t := false; p := false;
 FOREACH k IN ARRAY p_kinds LOOP
 setting := u.settings->k;
 IF COALESCE((setting->>'enabled')::boolean,true) THEN
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
CREATE FUNCTION notify_item_transition() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER item_notification AFTER INSERT OR UPDATE ON items FOR EACH ROW EXECUTE FUNCTION notify_item_transition();
-- +goose Down
DROP TRIGGER item_notification ON items;
DROP FUNCTION notify_item_transition();
DROP FUNCTION emit_notification(text,text,text,text[]);
DROP FUNCTION notification_detail(text);
ALTER TABLE notifications DROP COLUMN body;
-- +goose StatementBegin
CREATE FUNCTION emit_notification(p_title text, p_url text, p_kinds text[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u record; k text; setting jsonb; b boolean; t boolean; p boolean; n bigint;
BEGIN
 FOR u IN SELECT users.id, COALESCE(settings, '{}'::jsonb) settings FROM users LEFT JOIN notification_preferences ON user_id=users.id WHERE archived_at IS NULL LOOP
 b := false; t := false; p := false;
 FOREACH k IN ARRAY p_kinds LOOP
 setting := u.settings->k;
 IF COALESCE((setting->>'enabled')::boolean,true) THEN
 b := b OR COALESCE((setting->>'bell')::boolean,true);
 t := t OR COALESCE((setting->>'toast')::boolean,true);
 p := p OR COALESCE((setting->>'push')::boolean,true);
 END IF;
 END LOOP;
 IF b OR t OR p THEN
 INSERT INTO notifications(user_id,title,url,kinds,bell,toast,push) VALUES(u.id,p_title,p_url,p_kinds,b,t,p) RETURNING id INTO n;
 IF p THEN
 INSERT INTO notification_push(notification_id,endpoint) SELECT n,endpoint FROM notification_subscriptions WHERE user_id=u.id;
 END IF;
 END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION notify_item_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.archived_at IS NULL AND NEW.status IN ('ready','blocked') AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM NEW.status) THEN
 PERFORM emit_notification(NEW.title || CASE NEW.status WHEN 'ready' THEN ' — listo para implementar' ELSE ' — bloqueado' END,
 '/inbox?notification-item=' || NEW.id, ARRAY['item_' || NEW.status]);
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER item_notification AFTER INSERT OR UPDATE ON items FOR EACH ROW EXECUTE FUNCTION notify_item_transition();
