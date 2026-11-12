package checks

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// CheckSQLite writes a row, closes the database, reopens the file and rereads it.
func CheckSQLite(env *Env) (string, error) {
	path := filepath.Join(env.DataDir, "spike.db")
	want := fmt.Sprintf("hello-%d", time.Now().UnixNano())

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS kv (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		db.Close()
		return "", fmt.Errorf("create: %w", err)
	}
	res, err := db.Exec(`INSERT INTO kv (v) VALUES (?)`, want)
	if err != nil {
		db.Close()
		return "", fmt.Errorf("insert: %w", err)
	}
	id, _ := res.LastInsertId()
	if err = db.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}

	db2, err := sql.Open("sqlite", path)
	if err != nil {
		return "", fmt.Errorf("reopen: %w", err)
	}
	defer db2.Close()
	var got string
	if err = db2.QueryRow(`SELECT v FROM kv WHERE id = ?`, id).Scan(&got); err != nil {
		return "", fmt.Errorf("reread: %w", err)
	}
	if got != want {
		return "", fmt.Errorf("reread mismatch: got %q want %q", got, want)
	}
	var ver string
	_ = db2.QueryRow(`SELECT sqlite_version()`).Scan(&ver)
	fi, _ := os.Stat(path)
	size := int64(0)
	if fi != nil {
		size = fi.Size()
	}
	return fmt.Sprintf("sqlite %s, row %d reread from %s (%d bytes)", ver, id, path, size), nil
}
