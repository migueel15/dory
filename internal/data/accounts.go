package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/migueel15/dory/internal/auth"
)

// Accounts keeps users, sessions and tokens of this node. They never go
// through the operation log: credentials do not leave the machine.
type Accounts struct{ db *sql.DB }

func (s *Store) Accounts() Accounts { return Accounts{s.db} }

var _ auth.Store = Accounts{}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrNotFound
	}
	return err
}

func (a Accounts) FindUser(ctx context.Context, username string) (u auth.User, err error) {
	err = a.db.QueryRowContext(ctx, `SELECT id, username, password_hash FROM users WHERE username = ? AND archived_at IS NULL`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	return u, notFound(err)
}

func (a Accounts) GetUser(ctx context.Context, id string) (u auth.User, err error) {
	err = a.db.QueryRowContext(ctx, `SELECT id, username, password_hash FROM users WHERE id = ? AND archived_at IS NULL`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	return u, notFound(err)
}

func (a Accounts) CountUsers(ctx context.Context) (n int, err error) {
	err = a.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE archived_at IS NULL`).Scan(&n)
	return n, err
}

func (a Accounts) CreateUser(ctx context.Context, username, passwordHash string) (auth.User, error) {
	now := stamp(time.Now())
	u := auth.User{ID: newID(), Username: username, PasswordHash: passwordHash}
	_, err := a.db.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, now, now)
	return u, err
}

func (a Accounts) FindSession(ctx context.Context, secretHash string) (s auth.Session, err error) {
	var expires string
	err = a.db.QueryRowContext(ctx, `SELECT id, user_id, expires_at FROM sessions WHERE secret_hash = ? AND archived_at IS NULL AND expires_at > ?`,
		secretHash, stamp(time.Now())).Scan(&s.ID, &s.UserID, &expires)
	if err == nil {
		s.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	}
	return s, notFound(err)
}

func (a Accounts) CreateSession(ctx context.Context, userID, secretHash string, expires time.Time) error {
	now := stamp(time.Now())
	_, err := a.db.ExecContext(ctx, `INSERT INTO sessions(id, user_id, secret_hash, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		newID(), userID, secretHash, stamp(expires), now, now)
	return err
}

func (a Accounts) EndSession(ctx context.Context, id string) error {
	now := stamp(time.Now())
	_, err := a.db.ExecContext(ctx, `UPDATE sessions SET archived_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}

func (a Accounts) FindToken(ctx context.Context, secretHash string) (t auth.Token, err error) {
	var permissions string
	var expires sql.NullString
	err = a.db.QueryRowContext(ctx, `SELECT id, user_id, name, secret_hash, permissions, expires_at FROM api_tokens
		WHERE secret_hash = ? AND archived_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`, secretHash, stamp(time.Now())).
		Scan(&t.ID, &t.UserID, &t.Name, &t.SecretHash, &permissions, &expires)
	if err != nil {
		return t, notFound(err)
	}
	if expires.Valid {
		e, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return t, err
		}
		t.ExpiresAt = &e
	}
	return t, json.Unmarshal([]byte(permissions), &t.Permissions)
}

func (a Accounts) CreateToken(ctx context.Context, t auth.Token) error {
	now := stamp(time.Now())
	permissions, err := json.Marshal(t.Permissions)
	if err != nil {
		return err
	}
	var expires any
	if t.ExpiresAt != nil {
		expires = stamp(*t.ExpiresAt)
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO api_tokens(id, user_id, name, secret_hash, permissions, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), t.UserID, t.Name, t.SecretHash, string(permissions), expires, now, now)
	return err
}
