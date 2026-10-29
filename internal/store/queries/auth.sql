-- name: FindUser :one
SELECT to_jsonb(t) AS data FROM users t WHERE username = @username AND archived_at IS NULL;
-- name: FindSession :one
SELECT to_jsonb(t) AS data FROM sessions t JOIN users u ON u.id = t.user_id WHERE t.secret_hash = @secret_hash AND t.archived_at IS NULL AND u.archived_at IS NULL AND t.expires_at > now();
-- name: FindToken :one
SELECT to_jsonb(t) AS data FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.secret_hash = @secret_hash AND t.archived_at IS NULL AND u.archived_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > now());
