// Package localstore implements service.Service on a local SQLite file, so a
// node keeps working without the Postgres server.
package localstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/syncx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Options configures a store. Device and Clock are the hooks later sync builds
// on: with Local set, every write stamps the fields it changed in field_clock
// and new rows record Device as owner_device.
type Options struct {
	Device string
	// Clock returns the current time; nil means time.Now.
	Clock func() time.Time
	Local bool
}

// execer is satisfied by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type backend struct {
	db   execer
	root *sql.DB
	opts Options
	seq  *atomic.Int64
	log  *syncx.Log // set when Options.Local: writes append replicated operations
}

// Store is a service.Service backed by SQLite that can be closed.
type Store struct {
	service.Service
	root *sql.DB
	log  *syncx.Log
}

// Log is the operation log; nil unless the store was opened with Local.
func (s *Store) Log() *syncx.Log { return s.log }

func (s *Store) Close() error { return s.root.Close() }

// Open opens (creating it if needed) the database at path and applies migrations.
func Open(path string, opts Options) (*Store, error) {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	dsn := "file::memory:?cache=shared"
	if path != ":memory:" {
		dsn = "file:" + strings.NewReplacer("?", "%3f", "#", "%23").Replace(path) + "?"
	} else {
		dsn += "&"
	}
	dsn += "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if err = migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	b := &backend{db: db, root: db, opts: opts, seq: new(atomic.Int64)}
	if opts.Local {
		log, err := syncx.NewLog(context.Background(), db, opts.Device, syncx.NewClock(opts.Device, opts.Clock), syncx.Hooks{Reindex: b.reindex})
		if err != nil {
			db.Close()
			return nil, err
		}
		b.log = log
	}
	return &Store{Service: b, root: db, log: b.log}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var version int
		if _, err = fmt.Sscanf(name, "%d_", &version); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		var done int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?`, version).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().UTC().Format(time.RFC3339))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

var _ service.Service = (*backend)(nil)
var _ service.Service = (*Store)(nil)

// savepoint runs fn inside a savepoint of the open transaction tx.
func (b *backend) savepoint(ctx context.Context, tx execer, fn func() error) error {
	name := fmt.Sprintf("sp%d", b.seq.Add(1))
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	if err := fn(); err != nil {
		tx.ExecContext(ctx, "ROLLBACK TO "+name)
		tx.ExecContext(ctx, "RELEASE "+name)
		return err
	}
	_, err := tx.ExecContext(ctx, "RELEASE "+name)
	return err
}

// write runs fn atomically: in its own transaction, or in a savepoint when the
// backend already belongs to an InTx.
func (b *backend) write(ctx context.Context, fn func(execer) error) error {
	if tx, ok := b.db.(*sql.Tx); ok {
		return b.savepoint(ctx, tx, func() error { return fn(tx) })
	}
	tx, err := b.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (b *backend) InTx(ctx context.Context, fn func(service.Service) error) error {
	if tx, ok := b.db.(*sql.Tx); ok {
		return b.savepoint(ctx, tx, func() error { return fn(b) })
	}
	tx, err := b.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&backend{db: tx, root: b.root, opts: b.opts, seq: b.seq, log: b.log}); err != nil {
		return err
	}
	return tx.Commit()
}

func res[T any](b *backend, kind string) service.Resource[T] {
	return resource[T]{b: b, spec: specs[kind]}
}
func (b *backend) Projects() service.Resource[service.Project] {
	return res[service.Project](b, "project")
}
func (b *backend) Repositories() service.Resource[service.Repository] {
	return res[service.Repository](b, "repository")
}
func (b *backend) Environments() service.Resource[service.Environment] {
	return res[service.Environment](b, "environment")
}
func (b *backend) RepositoryLocations() service.Resource[service.RepositoryLocation] {
	return res[service.RepositoryLocation](b, "repositorylocation")
}
func (b *backend) T3Instances() service.Resource[service.T3Instance] {
	return res[service.T3Instance](b, "t3instance")
}
func (b *backend) ItemThreads() service.Resource[service.ItemThread] {
	return res[service.ItemThread](b, "itemthread")
}
func (b *backend) ItemPlans() service.Resource[service.ItemPlan] {
	return res[service.ItemPlan](b, "itemplan")
}
func (b *backend) IssueThreads() service.Resource[service.IssueThread] {
	return res[service.IssueThread](b, "issuethread")
}
func (b *backend) Features() service.Resource[service.Feature] {
	return res[service.Feature](b, "feature")
}
func (b *backend) Items() service.Resource[service.Item] {
	return titledItems{res[service.Item](b, "item")}
}
func (b *backend) Memories() service.Resource[service.Memory] {
	return res[service.Memory](b, "memory")
}
func (b *backend) Links() service.Resource[service.Link] { return res[service.Link](b, "link") }
func (b *backend) Users() service.Resource[service.User] { return res[service.User](b, "user") }
func (b *backend) Sessions() service.Resource[service.Session] {
	return res[service.Session](b, "session")
}
func (b *backend) Tokens() service.Resource[service.Token] { return res[service.Token](b, "token") }

// titledItems enforces the title_source rules, as the Postgres service does.
type titledItems struct{ service.Resource[service.Item] }

func (t titledItems) Put(ctx context.Context, actor string, in service.Item) (service.Item, error) {
	var current *service.Item
	if in.ID != "" {
		if c, err := t.Get(ctx, in.ID); err == nil {
			current = &c
		}
	}
	source, err := service.ResolveTitleSource(current, in)
	if err != nil {
		return service.Item{}, err
	}
	in.TitleSource = source
	return t.Resource.Put(ctx, actor, in)
}

// reindex keeps the search index in step with records changed by sync.
func (b *backend) reindex(ctx context.Context, q syncx.Execer, kind, id string, doc map[string]any, archived bool) error {
	if archived {
		return b.unindexSearch(ctx, q, kind, id)
	}
	return b.indexSearch(ctx, q, kind, id, doc)
}
