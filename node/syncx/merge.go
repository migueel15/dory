package syncx

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

const timeLayout = "2006-01-02T15:04:05.000000Z"

// FormatTime renders a stamp's wall clock the way records store timestamps.
func FormatTime(h HLC) string { return h.Time().Format(timeLayout) }

// arrayFields are replicated as add-wins sets.
var arrayFields = []string{"tags", "refs", "stack", "agents_available", "code_refs", "permissions", "open_questions"}

// longText fields keep the losing text of a concurrent edit as a conflict event.
var longText = []string{"body_md", "content", "progress", "raw_text"}

// columnFields live in columns, not in data.
var columnFields = []string{"id", "created_at", "updated_at", "archived_at"}

const legacyTag = "!legacy"

func canon(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func tagFor(h string, i int) string { return fmt.Sprintf("%s#%04d", h, i) }

type rec struct {
	exists   bool
	data     map[string]any
	fc       map[string]string
	archived sql.NullString
	created  string
	owner    string
}

type elem struct{ adds, rems []string }

func (e *elem) live() []string {
	var out []string
	for _, a := range e.adds {
		if !slices.Contains(e.rems, a) {
			out = append(out, a)
		}
	}
	return out
}

func union(a, b []string) []string {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

type merger struct {
	ctx     context.Context
	q       Execer
	hooks   Hooks
	touched map[[2]string]bool
}

func (m *merger) load(kind, id string) (*rec, error) {
	r := &rec{data: map[string]any{}, fc: map[string]string{}}
	var data, fc string
	err := m.q.QueryRowContext(m.ctx, `SELECT data, field_clock, owner_device, created_at, archived_at FROM records WHERE kind = ? AND id = ?`, kind, id).
		Scan(&data, &fc, &r.owner, &r.created, &r.archived)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	r.exists = true
	if err = json.Unmarshal([]byte(data), &r.data); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(fc), &r.fc); err != nil || r.fc == nil {
		r.fc = map[string]string{}
	}
	return r, nil
}

func (m *merger) save(kind, id string, r *rec) error {
	origin, err := ParseHLC(r.fc["$origin"])
	if err != nil {
		return err
	}
	updated, err := ParseHLC(r.fc["$updated"])
	if err != nil {
		return err
	}
	data, err := json.Marshal(r.data)
	if err != nil {
		return err
	}
	fc, err := json.Marshal(r.fc)
	if err != nil {
		return err
	}
	_, err = m.q.ExecContext(m.ctx, `INSERT INTO records (kind, id, data, field_clock, owner_device, created_at, updated_at, archived_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, id) DO UPDATE SET data = excluded.data, field_clock = excluded.field_clock, owner_device = excluded.owner_device,
		created_at = excluded.created_at, updated_at = excluded.updated_at, archived_at = excluded.archived_at`,
		kind, id, string(data), string(fc), origin.Device, FormatTime(origin), FormatTime(updated), r.archived)
	return err
}

// loadElems returns the set state of one array field. Elements of a legacy
// array (written before sync) count as added under a shared tag.
func (m *merger) loadElems(kind, id, field string, data map[string]any) (map[string]*elem, error) {
	rows, err := m.q.QueryContext(m.ctx, `SELECT elem, adds, rems FROM set_elems WHERE kind = ? AND id = ? AND field = ?`, kind, id, field)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*elem{}
	for rows.Next() {
		var k, a, r string
		if err = rows.Scan(&k, &a, &r); err != nil {
			return nil, err
		}
		e := &elem{}
		if err = json.Unmarshal([]byte(a), &e.adds); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(r), &e.rems); err != nil {
			return nil, err
		}
		out[k] = e
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		if arr, ok := data[field].([]any); ok {
			for _, v := range arr {
				out[canon(v)] = &elem{adds: []string{legacyTag}}
			}
		}
	}
	return out, nil
}

func (m *merger) saveElems(kind, id, field string, set map[string]*elem) error {
	for k, e := range set {
		a, _ := json.Marshal(nonNil(e.adds))
		r, _ := json.Marshal(nonNil(e.rems))
		if _, err := m.q.ExecContext(m.ctx, `INSERT INTO set_elems (kind, id, field, elem, adds, rems) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (kind, id, field, elem) DO UPDATE SET adds = excluded.adds, rems = excluded.rems`, kind, id, field, k, string(a), string(r)); err != nil {
			return err
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// materialize lists the live elements of a set, oldest add first.
func materialize(set map[string]*elem) []any {
	type item struct{ first, key string }
	var items []item
	for k, e := range set {
		if l := e.live(); len(l) > 0 {
			items = append(items, item{l[0], k})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].first != items[j].first {
			return items[i].first < items[j].first
		}
		return items[i].key < items[j].key
	})
	out := make([]any, 0, len(items))
	for _, it := range items {
		var v any
		json.Unmarshal([]byte(it.key), &v)
		out = append(out, v)
	}
	return out
}

func isArrayField(f string) bool { return slices.Contains(arrayFields, f) }

// Diff computes the body of the operation that turns the record old into new
// (both without column fields). Scalars become Set entries carrying the clock
// they replace; array fields become element adds and removals.
func (l *Log) Diff(ctx context.Context, q Execer, kind, id string, old, new map[string]any) (Fields, error) {
	m := &merger{ctx: ctx, q: q}
	r, err := m.load(kind, id)
	if err != nil {
		return Fields{}, err
	}
	var f Fields
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range new {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		if !slices.Contains(columnFields, k) && !strings.HasPrefix(k, "$") {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, k := range names {
		if isArrayField(k) {
			set, err := m.loadElems(kind, id, k, old)
			if err != nil {
				return f, err
			}
			want := map[string]any{}
			var order []string
			if arr, ok := new[k].([]any); ok {
				for _, v := range arr {
					c := canon(v)
					if _, dup := want[c]; !dup {
						want[c] = v
						order = append(order, c)
					}
				}
			}
			for _, c := range order {
				if e := set[c]; e == nil || len(e.live()) == 0 {
					if f.Adds == nil {
						f.Adds = map[string][]any{}
					}
					f.Adds[k] = append(f.Adds[k], want[c])
				}
			}
			var gone []string
			for c, e := range set {
				if _, keep := want[c]; !keep && len(e.live()) > 0 {
					gone = append(gone, c)
				}
			}
			sort.Strings(gone)
			for _, c := range gone {
				var v any
				json.Unmarshal([]byte(c), &v)
				if f.Rems == nil {
					f.Rems = map[string][]Removal{}
				}
				f.Rems[k] = append(f.Rems[k], Removal{Elem: v, Tags: set[c].live()})
			}
			continue
		}
		if reflect.DeepEqual(old[k], new[k]) {
			continue
		}
		if f.Set == nil {
			f.Set = map[string]any{}
		}
		f.Set[k] = new[k]
		if c, ok := r.fc[k]; ok {
			if f.Base == nil {
				f.Base = map[string]string{}
			}
			f.Base[k] = c
		}
	}
	return f, nil
}

// merge applies one record operation. It is commutative, associative and
// idempotent per field, so replicas converge whatever the delivery order.
func (m *merger) merge(op Op, f Fields) error {
	r, err := m.load(op.Entity, op.EntityID)
	if err != nil {
		return err
	}
	h := op.HLC
	if _, ok := r.fc["$origin"]; !ok {
		if r.exists {
			// A row written before sync: keep its creation time as the origin.
			t, _ := time.Parse(timeLayout, r.created)
			r.fc["$origin"] = HLC{Wall: t.UnixMilli(), Device: r.owner}.String()
		} else {
			r.fc["$origin"] = h
		}
	}
	if h < r.fc["$origin"] {
		r.fc["$origin"] = h
	}
	if h > r.fc["$updated"] {
		r.fc["$updated"] = h
	}
	for k, v := range f.Set {
		if slices.Contains(columnFields, k) && k != "archived_at" || strings.HasPrefix(k, "$") || isArrayField(k) {
			continue
		}
		if h <= r.fc[k] {
			continue
		}
		r.fc[k] = h
		switch {
		case k == "archived_at":
			s, ok := v.(string)
			r.archived = sql.NullString{String: s, Valid: ok}
		case v == nil:
			delete(r.data, k)
		default:
			r.data[k] = v
		}
		if k == "slug" && op.Entity == "project" {
			s, _ := v.(string)
			if _, err = m.q.ExecContext(m.ctx, `INSERT INTO sync_slugs (id, intent, rank) VALUES (?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET intent = excluded.intent, rank = excluded.rank`, op.EntityID, s, h); err != nil {
				return err
			}
		}
	}
	fields := map[string]bool{}
	for k := range f.Adds {
		fields[k] = true
	}
	for k := range f.Rems {
		fields[k] = true
	}
	for k := range fields {
		set, err := m.loadElems(op.Entity, op.EntityID, k, r.data)
		if err != nil {
			return err
		}
		get := func(c string) *elem {
			if set[c] == nil {
				set[c] = &elem{}
			}
			return set[c]
		}
		for i, v := range f.Adds[k] {
			e := get(canon(v))
			e.adds = union(e.adds, []string{tagFor(h, i)})
		}
		for _, rm := range f.Rems[k] {
			e := get(canon(rm.Elem))
			e.rems = union(e.rems, rm.Tags)
		}
		if err = m.saveElems(op.Entity, op.EntityID, k, set); err != nil {
			return err
		}
		r.data[k] = materialize(set)
	}
	m.touched[[2]string{op.Entity, op.EntityID}] = true
	return m.save(op.Entity, op.EntityID, r)
}

func (m *merger) applyEvent(f Fields) error {
	e := f.Event
	if e == nil {
		return errors.New("syncx: event op without event")
	}
	change, err := json.Marshal(e["change"])
	if err != nil {
		return err
	}
	_, err = m.q.ExecContext(m.ctx, `INSERT OR IGNORE INTO events (device, seq, entity_type, entity_id, actor, change, at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e["device"], e["seq"], e["entity_type"], e["entity_id"], e["actor"], string(change), e["at"])
	return err
}

type opRow struct {
	hlc  string
	dev  string
	seq  int64
	body Fields
}

// detectConflicts saves, as a conflict event, the text that lost against a
// concurrent edit of the same long field. A pair is concurrent when the older
// write is newer than what the newer writer had seen (its base). The test only
// looks at the pair, so every replica finds the same conflicts in any order.
func (m *merger) detectConflicts(op Op, f Fields) error {
	var fields []string
	for _, k := range longText {
		if _, ok := f.Set[k].(string); ok {
			fields = append(fields, k)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	rows, err := m.q.QueryContext(m.ctx, `SELECT device_id, seq, hlc, fields_json FROM ops WHERE entity = ? AND entity_id = ? AND hlc <> ? AND kind <> 'event'`, op.Entity, op.EntityID, op.HLC)
	if err != nil {
		return err
	}
	var others []opRow
	for rows.Next() {
		var o opRow
		var body string
		if err = rows.Scan(&o.dev, &o.seq, &o.hlc, &body); err != nil {
			rows.Close()
			return err
		}
		if json.Unmarshal([]byte(body), &o.body) == nil {
			others = append(others, o)
		}
	}
	rows.Close()
	self := opRow{hlc: op.HLC, dev: op.DeviceID, seq: op.Seq, body: f}
	for _, o := range others {
		for _, k := range fields {
			_, ok := o.body.Set[k].(string)
			if !ok {
				continue
			}
			loser, winner := o, self
			if o.hlc > self.hlc {
				loser, winner = self, o
			}
			lt, _ := loser.body.Set[k].(string)
			wt, _ := winner.body.Set[k].(string)
			if lt == wt || loser.hlc <= winner.body.Base[k] {
				continue
			}
			if err = m.conflictEvent(op.Entity, op.EntityID, k, loser, lt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *merger) conflictEvent(entity, id, field string, loser opRow, text string) error {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", loser.dev, loser.seq, field)))
	seq := int64(binary.BigEndian.Uint64(sum[:8]) &^ (1 << 63))
	change, err := json.Marshal(map[string]any{"operation": "conflict", "field": field, "device": loser.dev, "hlc": loser.hlc, "text": text})
	if err != nil {
		return err
	}
	at := ""
	if h, err := ParseHLC(loser.hlc); err == nil {
		at = FormatTime(h)
	}
	_, err = m.q.ExecContext(m.ctx, `INSERT OR IGNORE INTO events (device, seq, entity_type, entity_id, actor, change, at) VALUES ('conflict', ?, ?, ?, 'sync', ?, ?)`,
		seq, entity, id, string(change), at)
	return err
}

// recomputeSlugs gives every project a unique slug. Projects are ranked by the
// stamp of their slug write (then id); the first keeps the slug, later ones get
// slug-2, slug-3 and so on. It depends only on the set of writes.
func (m *merger) recomputeSlugs() error {
	rows, err := m.q.QueryContext(m.ctx, `SELECT r.id, r.data, coalesce(s.intent, ''), coalesce(s.rank, '') FROM records r LEFT JOIN sync_slugs s ON s.id = r.id WHERE r.kind = 'project'`)
	if err != nil {
		return err
	}
	type proj struct {
		id, intent, rank string
		data             map[string]any
	}
	var ps []*proj
	for rows.Next() {
		p := &proj{}
		var data string
		if err = rows.Scan(&p.id, &data, &p.intent, &p.rank); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(data), &p.data); err != nil {
			rows.Close()
			return err
		}
		if p.intent == "" {
			p.intent, _ = p.data["slug"].(string)
		}
		ps = append(ps, p)
	}
	rows.Close()
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].rank != ps[j].rank {
			return ps[i].rank < ps[j].rank
		}
		return ps[i].id < ps[j].id
	})
	taken := map[string]bool{}
	for _, p := range ps {
		if p.intent == "" {
			continue
		}
		slug := p.intent
		for n := 2; taken[slug]; n++ {
			slug = fmt.Sprintf("%s-%d", p.intent, n)
		}
		taken[slug] = true
		if cur, _ := p.data["slug"].(string); cur == slug {
			continue
		}
		p.data["slug"] = slug
		b, err := json.Marshal(p.data)
		if err != nil {
			return err
		}
		if _, err = m.q.ExecContext(m.ctx, `UPDATE records SET data = ? WHERE kind = 'project' AND id = ?`, string(b), p.id); err != nil {
			return err
		}
		m.touched[[2]string{"project", p.id}] = true
	}
	return nil
}

// reindex runs the owner's hook on every record the operations touched.
func (m *merger) reindex() error {
	if m.hooks.Reindex == nil {
		return nil
	}
	keys := make([][2]string, 0, len(m.touched))
	for k := range m.touched {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		r, err := m.load(k[0], k[1])
		if err != nil {
			return err
		}
		if err = m.hooks.Reindex(m.ctx, m.q, k[0], k[1], r.data, r.archived.Valid); err != nil {
			return err
		}
	}
	return nil
}
