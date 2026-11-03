-- +goose Up
CREATE TABLE notification_preferences (
 user_id uuid REFERENCES users(id) PRIMARY KEY,
 settings jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE notifications (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id),
 title text NOT NULL, url text NOT NULL,
 kinds text[] NOT NULL,
 bell boolean NOT NULL, toast boolean NOT NULL, push boolean NOT NULL,
 read_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications(user_id, id DESC);
CREATE TABLE notification_subscriptions (
 user_id uuid NOT NULL REFERENCES users(id), endpoint text PRIMARY KEY,
 subscription jsonb NOT NULL
);
CREATE TABLE notification_push (
 notification_id bigint REFERENCES notifications(id) ON DELETE CASCADE,
 endpoint text REFERENCES notification_subscriptions(endpoint) ON DELETE CASCADE,
 attempts integer NOT NULL DEFAULT 0, next_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(notification_id, endpoint)
);
CREATE TABLE notification_instances (instance_id text PRIMARY KEY);
CREATE TABLE notification_threads (
 instance_id text NOT NULL, thread_id text NOT NULL, state jsonb NOT NULL,
 PRIMARY KEY(instance_id, thread_id)
);
CREATE TABLE notification_keys (id boolean PRIMARY KEY DEFAULT true CHECK(id), private_key text NOT NULL, public_key text NOT NULL);
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
-- +goose Down
DROP TRIGGER item_notification ON items;
DROP FUNCTION notify_item_transition();
DROP FUNCTION emit_notification(text,text,text[]);
DROP TABLE notification_push, notification_subscriptions, notifications, notification_preferences, notification_threads, notification_instances, notification_keys;
