package syncx

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"sync"
)

// Execer is satisfied by *sql.DB and *sql.Tx.
type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Hooks lets the owner of the database keep derived data (the search index)
// consistent after records change through sync.
type Hooks struct {
	Reindex func(ctx context.Context, q Execer, kind, id string, doc map[string]any, archived bool) error
}

// Log is the operation log of one device plus the merge that applies remote
// operations to the record tables.
type Log struct {
	db     *sql.DB
	device string
	clock  *Clock
	hooks  Hooks

	wmu      sync.Mutex
	watchers map[chan struct{}]struct{}
}

// NewLog opens the log over db (which must already have the sync tables) and
// moves clock past every stamp already stored.
func NewLog(ctx context.Context, db *sql.DB, device string, clock *Clock, hooks Hooks) (*Log, error) {
	l := &Log{db: db, device: device, clock: clock, hooks: hooks}
	var last sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT max(hlc) FROM ops`).Scan(&last); err != nil {
		return nil, err
	}
	if last.Valid {
		if h, err := ParseHLC(last.String); err == nil {
			clock.Seed(h)
		}
	}
	return l, nil
}

// Device is the id of the local device.
func (l *Log) Device() string { return l.device }

// Now issues a stamp for a local change.
func (l *Log) Now() HLC { return l.clock.Now() }

// Append stores op in the log. If op.Seq is zero it takes the next seq of the
// local device; DeviceID defaults to the local device and SchemaV to the
// current schema. It does not touch the record tables.
func (l *Log) Append(ctx context.Context, q Execer, op Op) (Op, error) {
	if op.DeviceID == "" {
		op.DeviceID = l.device
	}
	if op.SchemaV == 0 {
		op.SchemaV = SchemaVersion
	}
	if op.Seq == 0 {
		if err := q.QueryRowContext(ctx, `SELECT coalesce(max(seq), 0) + 1 FROM ops WHERE device_id = ?`, op.DeviceID).Scan(&op.Seq); err != nil {
			return op, err
		}
	}
	if err := op.validate(); err != nil {
		return op, err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO ops (device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		op.DeviceID, op.Seq, op.HLC, op.Entity, op.EntityID, op.Kind, string(op.Fields), op.SchemaV)
	return op, err
}

// Commit turns a local change into an operation stamped h, logs it and merges
// it into the record tables, all in q's transaction. An empty body writes
// nothing and reports false.
func (l *Log) Commit(ctx context.Context, q Execer, h HLC, kind, entity, id string, f Fields) (Op, bool, error) {
	if f.Empty() {
		return Op{}, false, nil
	}
	body, err := json.Marshal(f)
	if err != nil {
		return Op{}, false, err
	}
	op, err := l.Append(ctx, q, Op{HLC: h.String(), Entity: entity, EntityID: id, Kind: kind, Fields: body})
	if err != nil {
		return op, false, err
	}
	if kind == KindEvent {
		return op, true, nil
	}
	m := &merger{ctx: ctx, q: q, hooks: l.hooks, touched: map[[2]string]bool{}}
	if err = m.merge(op, f); err != nil {
		return op, false, err
	}
	if _, ok := f.Set["slug"]; ok && entity == "project" {
		if err = m.recomputeSlugs(); err != nil {
			return op, false, err
		}
	}
	return op, true, m.reindex()
}

// Apply merges remote operations. It is idempotent and order independent:
// operations already in the log are skipped, and applying never creates new
// operations. Operations may arrive duplicated, permuted or with gaps.
func (l *Log) Apply(ctx context.Context, ops []Op) error {
	_, err := l.ApplyNew(ctx, ops)
	return err
}

// ApplyNew is Apply that also returns the operations that were new. When some
// were, it wakes the watchers so that peers subscribed to this log see them.
func (l *Log) ApplyNew(ctx context.Context, ops []Op) ([]Op, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	applied, err := l.ApplyTx(ctx, tx, ops)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	for _, o := range applied {
		if h, err := ParseHLC(o.HLC); err == nil && o.DeviceID != l.device {
			l.clock.Observe(h)
		}
	}
	if len(applied) > 0 {
		l.Notify()
	}
	return applied, nil
}

// Watch returns a channel that receives a signal after the log grew (a local
// write committed or remote operations were applied). Signals coalesce. Call
// cancel to stop watching.
func (l *Log) Watch() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	l.wmu.Lock()
	if l.watchers == nil {
		l.watchers = map[chan struct{}]struct{}{}
	}
	l.watchers[ch] = struct{}{}
	l.wmu.Unlock()
	return ch, func() {
		l.wmu.Lock()
		delete(l.watchers, ch)
		l.wmu.Unlock()
	}
}

// Notify wakes the watchers. The owner of the database calls it after a local
// write committed.
func (l *Log) Notify() {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	for ch := range l.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Snapshot returns the whole log and its version vector read in one
// transaction, so a new node can start from a consistent state.
func (l *Log) Snapshot(ctx context.Context) (VersionVector, []Op, error) {
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	vv, err := vector(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v FROM ops ORDER BY hlc`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ops []Op
	for rows.Next() {
		var o Op
		var body string
		if err = rows.Scan(&o.DeviceID, &o.Seq, &o.HLC, &o.Entity, &o.EntityID, &o.Kind, &body, &o.SchemaV); err != nil {
			return nil, nil, err
		}
		o.Fields = json.RawMessage(body)
		ops = append(ops, o)
	}
	return vv, ops, rows.Err()
}

// ApplyTx is Apply inside the caller's transaction. It returns the operations
// that were new.
func (l *Log) ApplyTx(ctx context.Context, q Execer, ops []Op) ([]Op, error) {
	sorted := append([]Op(nil), ops...)
	for _, o := range sorted {
		if err := o.validate(); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].HLC < sorted[j].HLC })
	m := &merger{ctx: ctx, q: q, hooks: l.hooks, touched: map[[2]string]bool{}}
	var applied []Op
	slugs := false
	for _, o := range sorted {
		if o.SchemaV == 0 {
			o.SchemaV = SchemaVersion
		}
		res, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO ops (device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			o.DeviceID, o.Seq, o.HLC, o.Entity, o.EntityID, o.Kind, string(o.Fields), o.SchemaV)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		applied = append(applied, o)
		f, err := o.Decode()
		if err != nil {
			return nil, err
		}
		if o.Kind == KindEvent {
			err = m.applyEvent(f)
		} else {
			if _, ok := f.Set["slug"]; ok && o.Entity == "project" {
				slugs = true
			}
			if err = m.merge(o, f); err == nil {
				err = m.detectConflicts(o, f)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if slugs {
		if err := m.recomputeSlugs(); err != nil {
			return nil, err
		}
	}
	return applied, m.reindex()
}

// Vector returns, per device, the highest seq up to which the log is complete.
func (l *Log) Vector(ctx context.Context) (VersionVector, error) {
	return vector(ctx, l.db)
}

func vector(ctx context.Context, q Execer) (VersionVector, error) {
	rows, err := q.QueryContext(ctx, `SELECT device_id, count(*), max(seq) FROM ops GROUP BY device_id`)
	if err != nil {
		return nil, err
	}
	vv := VersionVector{}
	var gaps []string
	for rows.Next() {
		var d string
		var n, hi int64
		if err = rows.Scan(&d, &n, &hi); err != nil {
			rows.Close()
			return nil, err
		}
		if n == hi {
			vv[d] = hi
		} else {
			gaps = append(gaps, d)
		}
	}
	rows.Close()
	for _, d := range gaps {
		srows, err := q.QueryContext(ctx, `SELECT seq FROM ops WHERE device_id = ? ORDER BY seq`, d)
		if err != nil {
			return nil, err
		}
		var next int64 = 1
		for srows.Next() {
			var s int64
			if err = srows.Scan(&s); err != nil {
				srows.Close()
				return nil, err
			}
			if s != next {
				break
			}
			next++
		}
		srows.Close()
		vv[d] = next - 1
	}
	return vv, nil
}

// Since returns up to limit operations the holder of vv lacks, ordered by
// stamp. Because a device's stamps grow with its seq, the result is a prefix of
// what is missing for every device.
func (l *Log) Since(ctx context.Context, vv VersionVector, limit int) ([]Op, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := l.db.QueryContext(ctx, `SELECT DISTINCT device_id FROM ops`)
	if err != nil {
		return nil, err
	}
	var devices []string
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			rows.Close()
			return nil, err
		}
		devices = append(devices, d)
	}
	rows.Close()
	var out []Op
	for _, d := range devices {
		r, err := l.db.QueryContext(ctx, `SELECT device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v FROM ops
			WHERE device_id = ? AND seq > ? ORDER BY seq LIMIT ?`, d, vv[d], limit)
		if err != nil {
			return nil, err
		}
		for r.Next() {
			var o Op
			var body string
			if err = r.Scan(&o.DeviceID, &o.Seq, &o.HLC, &o.Entity, &o.EntityID, &o.Kind, &body, &o.SchemaV); err != nil {
				r.Close()
				return nil, err
			}
			o.Fields = json.RawMessage(body)
			out = append(out, o)
		}
		r.Close()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HLC < out[j].HLC })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Replicated reports whether records of kind are replicated. Users, sessions,
// tokens and repository locations never leave the device.
func Replicated(kind string) bool {
	switch kind {
	case "project", "repository", "environment", "feature", "item", "itemplan",
		"memory", "link", "itemthread", "issuethread", "t3instance", "deviceaddress":
		return true
	}
	return false
}

// EventReplicated reports whether audit events about kind are replicated.
func EventReplicated(kind string) bool {
	switch kind {
	case "user", "session", "token", "repositorylocation", "deviceaddress":
		return false
	}
	return true
}

// Archive logs and applies the archival of a record at stamp h.
func (l *Log) Archive(ctx context.Context, q Execer, h HLC, entity, id string) (Op, error) {
	m := &merger{ctx: ctx, q: q, hooks: l.hooks, touched: map[[2]string]bool{}}
	r, err := m.load(entity, id)
	if err != nil {
		return Op{}, err
	}
	f := Fields{Set: map[string]any{"archived_at": FormatTime(h)}}
	if c, ok := r.fc["archived_at"]; ok {
		f.Base = map[string]string{"archived_at": c}
	}
	op, _, err := l.Commit(ctx, q, h, KindArchive, entity, id, f)
	return op, err
}
