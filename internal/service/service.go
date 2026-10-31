// Package service is the only persistence boundary used by REST, MCP and auth.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/migueel15/dory/internal/store"
)

var ErrNotFound = errors.New("resource not found")
var ErrInvalid = errors.New("invalid argument")

// Filter matches JSON fields exactly; tags use array containment. Limit defaults
// to 100 (maximum 1000). Get hides archives; List can explicitly include them.
// AnyOf keeps rows whose field equals one of the values; only items support it.
type Filter struct {
	Fields          map[string]any
	AnyOf           map[string][]string
	Limit, Offset   int32
	IncludeArchived bool
}

// Resource.Put is full replacement/upsert. Preserve the ID for idempotent writes.
// All writes require an authenticated actor and atomically append an audit event.
// PasswordHash and SecretHash are INTERNAL values: transports must redact them.
type Resource[T any] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, filter Filter) ([]T, error)
	Put(ctx context.Context, actor string, value T) (T, error)
	Archive(ctx context.Context, actor, id string) error
}
type Event struct {
	ID         int64           `json:"id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Actor      string          `json:"actor"`
	Change     json.RawMessage `json:"change"`
	At         time.Time       `json:"at"`
}
type SearchHit struct {
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Content    json.RawMessage `json:"content"`
	Rank       float32         `json:"rank"`
}
type Events interface {
	ListEvents(context.Context, int64, string, int32) ([]Event, error)
	LatestEventID(context.Context) (int64, error)
	// RecordEvent audits an action that happened outside the database.
	RecordEvent(ctx context.Context, actor, kind, id string, change any) error
}
type Search interface {
	Search(context.Context, string, int32, int32) ([]SearchHit, error)
}
type AuthLookup interface {
	FindUser(context.Context, string) (User, error)
	FindSession(context.Context, string) (Session, error)
	FindToken(context.Context, string) (Token, error)
}
type Projects interface{ Projects() Resource[Project] }
type Repositories interface{ Repositories() Resource[Repository] }
type Environments interface{ Environments() Resource[Environment] }
type RepositoryLocations interface {
	RepositoryLocations() Resource[RepositoryLocation]
}
type T3Instances interface{ T3Instances() Resource[T3Instance] }
type ItemThreads interface{ ItemThreads() Resource[ItemThread] }
type Features interface{ Features() Resource[Feature] }
type Items interface{ Items() Resource[Item] }
type Memories interface{ Memories() Resource[Memory] }
type Links interface{ Links() Resource[Link] }
type Users interface{ Users() Resource[User] }
type Sessions interface{ Sessions() Resource[Session] }
type Tokens interface{ Tokens() Resource[Token] }
type Service interface {
	Projects
	Repositories
	Environments
	RepositoryLocations
	T3Instances
	ItemThreads
	Features
	Items
	Memories
	Links
	Users
	Sessions
	Tokens
	Events
	Search
	AuthLookup
	Transactor
}

// Transactor runs several writes atomically. Inside fn every Put opens a
// savepoint of the outer transaction, so one failure rolls back everything.
type Transactor interface {
	InTx(ctx context.Context, fn func(Service) error) error
}

// db is satisfied by both the pool and an open transaction.
type db interface {
	store.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}
type backend struct {
	pool db
	q    *store.Queries
}

func New(pool *pgxpool.Pool) Service { return &backend{pool, store.New(pool)} }

func (b *backend) InTx(ctx context.Context, fn func(Service) error) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(&backend{tx, store.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ Service = (*backend)(nil)

type resource[T any] struct {
	b       *backend
	kind    string
	get     func(context.Context, *store.Queries, string) ([]byte, error)
	list    func(context.Context, *store.Queries, Filter, []byte) ([][]byte, error)
	put     func(context.Context, *store.Queries, []byte) ([]byte, error)
	archive func(context.Context, *store.Queries, string) ([]byte, error)
}

func decode[T any](raw []byte, err error) (v T, e error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		var dbErr *pgconn.PgError
		if errors.As(err, &dbErr) && (dbErr.Code == "22P02" || dbErr.Code == "23502" || dbErr.Code == "23514" || dbErr.Code == "23503") {
			return v, fmt.Errorf("%w: %s", ErrInvalid, dbErr.Message)
		}
		return v, err
	}
	return v, json.Unmarshal(raw, &v)
}
func (r resource[T]) Get(ctx context.Context, id string) (T, error) {
	return decode[T](r.get(ctx, r.b.q, id))
}
func page(limit, offset int32) (int32, error) {
	if offset < 0 || limit < 0 || limit > 1000 {
		return 0, ErrInvalid
	}
	if limit == 0 {
		limit = 100
	}
	return limit, nil
}
func (r resource[T]) List(ctx context.Context, f Filter) ([]T, error) {
	limit, err := page(f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	f.Limit = limit
	if f.Fields == nil {
		f.Fields = map[string]any{}
	}
	raw, err := json.Marshal(f.Fields)
	if err != nil {
		return nil, err
	}
	rows, err := r.list(ctx, r.b.q, f, raw)
	if err != nil {
		return nil, err
	}
	result := make([]T, 0, len(rows))
	for _, row := range rows {
		v, e := decode[T](row, nil)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, nil
}
func (r resource[T]) mutate(ctx context.Context, actor, operation string, fn func(*store.Queries) ([]byte, error)) (T, error) {
	var zero T
	if actor == "" {
		return zero, fmt.Errorf("%w: actor required", ErrInvalid)
	}
	tx, err := r.b.pool.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback(ctx)
	q := r.b.q.WithTx(tx)
	raw, err := fn(q)
	value, err := decode[T](raw, err)
	if err != nil {
		return zero, err
	}
	var identity struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(raw, &identity); err != nil {
		return zero, err
	}
	// Never persist password or credential hashes in audit payloads.
	var snapshot map[string]any
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return zero, err
	}
	delete(snapshot, "password_hash")
	delete(snapshot, "secret_hash")
	change, err := json.Marshal(map[string]any{"operation": operation, "record": snapshot})
	if err != nil {
		return zero, err
	}
	_, err = q.AppendEvent(ctx, store.AppendEventParams{EntityType: r.kind, EntityID: identity.ID, Actor: actor, Change: change})
	if err != nil {
		return zero, err
	}
	if err = tx.Commit(ctx); err != nil {
		return zero, err
	}
	return value, nil
}
func (r resource[T]) Put(ctx context.Context, actor string, v T) (T, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		var zero T
		return zero, err
	}
	// Domain slices marshal as null when unset; SQL array extraction needs [].
	var payload map[string]any
	if err = json.Unmarshal(raw, &payload); err != nil {
		var zero T
		return zero, err
	}
	for _, key := range []string{"tags", "refs", "stack", "agents_available", "code_refs", "permissions"} {
		if value, exists := payload[key]; exists && value == nil {
			payload[key] = []string{}
		}
	}
	defaults := map[string]map[string]string{
		"project": {"status": "active"}, "repository": {"default_branch": "main"},
		"feature": {"status": "planned"}, "item": {"status": "inbox", "source": "manual"},
	}
	for key, value := range defaults[r.kind] {
		if payload[key] == "" {
			payload[key] = value
		}
	}
	raw, err = json.Marshal(payload)
	if err != nil {
		var zero T
		return zero, err
	}
	return r.mutate(ctx, actor, "put", func(q *store.Queries) ([]byte, error) { return r.put(ctx, q, raw) })
}
func (r resource[T]) Archive(ctx context.Context, actor, id string) error {
	_, err := r.mutate(ctx, actor, "archive", func(q *store.Queries) ([]byte, error) { return r.archive(ctx, q, id) })
	return err
}
func (b *backend) ListEvents(ctx context.Context, after int64, kind string, limit int32) ([]Event, error) {
	limit, err := page(limit, 0)
	if err != nil || after < 0 {
		return nil, ErrInvalid
	}
	rows, err := b.q.ListEvents(ctx, store.ListEventsParams{AfterID: after, EntityType: kind, PageLimit: limit})
	if err != nil {
		return nil, err
	}
	result := make([]Event, 0, len(rows))
	for _, v := range rows {
		result = append(result, Event{v.ID, v.EntityType, v.EntityID.String(), v.Actor, v.Change, v.At.Time})
	}
	return result, nil
}
func (b *backend) RecordEvent(ctx context.Context, actor, kind, id string, change any) error {
	raw, err := json.Marshal(change)
	if err != nil {
		return err
	}
	_, err = b.q.AppendEvent(ctx, store.AppendEventParams{EntityType: kind, EntityID: id, Actor: actor, Change: raw})
	return err
}

func (b *backend) LatestEventID(ctx context.Context) (int64, error) {
	return b.q.LatestEventID(ctx)
}
func (b *backend) Search(ctx context.Context, query string, limit, offset int32) ([]SearchHit, error) {
	limit, err := page(limit, offset)
	if err != nil {
		return nil, err
	}
	rows, err := b.q.Search(ctx, store.SearchParams{Query: query, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, err
	}
	result := make([]SearchHit, 0, len(rows))
	for _, v := range rows {
		result = append(result, SearchHit{v.EntityType, v.EntityID, v.Content, v.Rank})
	}
	return result, nil
}
func (b *backend) Projects() Resource[Project] {
	return resource[Project]{b: b, kind: "project",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetProject(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListProject(ctx, store.ListProjectParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutProject(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveProject(ctx, id)
		},
	}
}
func (b *backend) Repositories() Resource[Repository] {
	return resource[Repository]{b: b, kind: "repository",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.GetRepository(ctx, id)
		},
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListRepository(ctx, store.ListRepositoryParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) {
			return q.PutRepository(ctx, raw)
		},
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveRepository(ctx, id)
		},
	}
}
func (b *backend) Environments() Resource[Environment] {
	return resource[Environment]{b: b, kind: "environment",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.GetEnvironment(ctx, id)
		},
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListEnvironment(ctx, store.ListEnvironmentParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) {
			return q.PutEnvironment(ctx, raw)
		},
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveEnvironment(ctx, id)
		},
	}
}
func (b *backend) RepositoryLocations() Resource[RepositoryLocation] {
	return resource[RepositoryLocation]{b: b, kind: "repositorylocation",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.GetRepositoryLocation(ctx, id)
		},
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListRepositoryLocation(ctx, store.ListRepositoryLocationParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) {
			return q.PutRepositoryLocation(ctx, raw)
		},
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveRepositoryLocation(ctx, id)
		},
	}
}
func (b *backend) T3Instances() Resource[T3Instance] {
	return resource[T3Instance]{b: b, kind: "t3instance",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.GetT3Instance(ctx, id)
		},
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListT3Instance(ctx, store.ListT3InstanceParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) {
			return q.PutT3Instance(ctx, raw)
		},
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveT3Instance(ctx, id)
		},
	}
}
func (b *backend) ItemThreads() Resource[ItemThread] {
	return resource[ItemThread]{b: b, kind: "itemthread",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.GetItemThread(ctx, id)
		},
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListItemThread(ctx, store.ListItemThreadParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) {
			return q.PutItemThread(ctx, raw)
		},
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveItemThread(ctx, id)
		},
	}
}
func (b *backend) Features() Resource[Feature] {
	return resource[Feature]{b: b, kind: "feature",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetFeature(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListFeature(ctx, store.ListFeatureParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutFeature(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveFeature(ctx, id)
		},
	}
}
func (b *backend) Items() Resource[Item] {
	return resource[Item]{b: b, kind: "item",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetItem(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			anyOf, err := json.Marshal(f.AnyOf)
			if err != nil {
				return nil, err
			}
			if f.AnyOf == nil {
				anyOf = []byte("{}")
			}
			return q.ListItem(ctx, store.ListItemParams{Filter: raw, AnyOf: anyOf, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put:     func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutItem(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.ArchiveItem(ctx, id) },
	}
}
func (b *backend) Memories() Resource[Memory] {
	return resource[Memory]{b: b, kind: "memory",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetMemory(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListMemory(ctx, store.ListMemoryParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutMemory(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveMemory(ctx, id)
		},
	}
}
func (b *backend) Links() Resource[Link] {
	return resource[Link]{b: b, kind: "link",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetLink(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListLink(ctx, store.ListLinkParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put:     func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutLink(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.ArchiveLink(ctx, id) },
	}
}
func (b *backend) Users() Resource[User] {
	return resource[User]{b: b, kind: "user",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetUser(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListUser(ctx, store.ListUserParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put:     func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutUser(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.ArchiveUser(ctx, id) },
	}
}
func (b *backend) Sessions() Resource[Session] {
	return resource[Session]{b: b, kind: "session",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetSession(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListSession(ctx, store.ListSessionParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put: func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutSession(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) {
			return q.ArchiveSession(ctx, id)
		},
	}
}
func (b *backend) Tokens() Resource[Token] {
	return resource[Token]{b: b, kind: "token",
		get: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.GetToken(ctx, id) },
		list: func(ctx context.Context, q *store.Queries, f Filter, raw []byte) ([][]byte, error) {
			return q.ListToken(ctx, store.ListTokenParams{Filter: raw, IncludeArchived: f.IncludeArchived, PageLimit: f.Limit, PageOffset: f.Offset})
		},
		put:     func(ctx context.Context, q *store.Queries, raw []byte) ([]byte, error) { return q.PutToken(ctx, raw) },
		archive: func(ctx context.Context, q *store.Queries, id string) ([]byte, error) { return q.ArchiveToken(ctx, id) },
	}
}
func (b *backend) FindUser(ctx context.Context, key string) (User, error) {
	return decode[User](b.q.FindUser(ctx, key))
}
func (b *backend) FindSession(ctx context.Context, key string) (Session, error) {
	return decode[Session](b.q.FindSession(ctx, key))
}
func (b *backend) FindToken(ctx context.Context, key string) (Token, error) {
	return decode[Token](b.q.FindToken(ctx, key))
}
