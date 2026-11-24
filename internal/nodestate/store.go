// Package nodestate stores local, unreplicated node preferences and snapshots.
// vapid.json, github.token, control.token, node.key, and the mesh identity stay
// on disk for the components that read them directly or before the DB opens.
package nodestate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	KeySettings          = "settings"
	KeyUsage             = "usage"
	KeyDisabledProviders = "providers.disabled"
	KeyAccounts          = "accounts"
	KeyModelCatalog      = "model-catalog"
	KeyTitleModel        = "title-model"
	KeyNodeIcon          = "node"
	KeySkillsManifest    = "skills-manifest"
)

type Store struct{ db *sql.DB }

// New uses a database opened by localdb, where transactions start with BEGIN IMMEDIATE.
func New(db *sql.DB) *Store { return &Store{db: db} }

type reader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type writer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Get decodes the saved value into out. An absent key leaves out untouched.
func (s *Store) Get(ctx context.Context, key string, out any) (bool, error) {
	return get(ctx, s.db, key, out)
}

func get(ctx context.Context, db reader, key string, out any) (bool, error) {
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM node_kv WHERE key = ?", key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), out)
}

func (s *Store) Put(ctx context.Context, key string, value any) error {
	return put(ctx, s.db, key, value)
}

func put(ctx context.Context, db writer, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO node_kv(key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(raw), timestamp())
	return err
}

// Update reads into out, lets fn modify it, and persists it in one transaction.
// The callback must use out directly and must not query the store.
func (s *Store) Update(ctx context.Context, key string, out any, fn func() error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = get(ctx, tx, key, out); err != nil {
		return err
	}
	if err = fn(); err != nil {
		return err
	}
	if err = put(ctx, tx, key, out); err != nil {
		return err
	}
	return tx.Commit()
}

// ImportFile preserves the legacy JSON as-is and archives the file after the
// insert commits. An existing key wins even if a legacy file is still present.
func (s *Store) ImportFile(ctx context.Context, key, path string) error {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM node_kv WHERE key = ?)", key).Scan(&exists)
	if exists || err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return fmt.Errorf("invalid JSON in %s", path)
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO node_kv(key, value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING`, key, string(raw), timestamp())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	return os.Rename(path, path+".migrated")
}

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
