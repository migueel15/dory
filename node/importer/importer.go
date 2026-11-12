// Package importer copies the data of a running Dory (its REST API) into a
// local-first store as initial replicated operations.
//
// Records are written as operations of a pseudo device, with the stamp taken
// from the source timestamps: the creation op at created_at and the last
// change at updated_at, so the merge derives the same created_at/updated_at
// (to the millisecond, the resolution of a stamp). Ids are preserved. A second
// run only writes what changed in the source. Users, sessions, tokens,
// notifications, repository locations and secrets are never read.
package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

// DefaultDevice is the pseudo device that owns the imported operations.
const DefaultDevice = "dory-import"

// Options configures an import.
type Options struct {
	BaseURL string
	Token   string
	Client  *http.Client
	// Device overrides DefaultDevice.
	Device string
	// Idle is how long the event stream must stay quiet to be considered fully
	// read; it defaults to 3 seconds.
	Idle time.Duration
}

func (o Options) source() *source {
	s := &source{base: o.BaseURL, token: o.Token, client: o.Client, idle: o.Idle}
	if s.client == nil {
		s.client = http.DefaultClient
	}
	if s.idle <= 0 {
		s.idle = 3 * time.Second
	}
	return s
}

func (o Options) device() string {
	if o.Device != "" {
		return o.Device
	}
	return DefaultDevice
}

// Counts is what happened to the records of one entity.
type Counts struct {
	Source, Created, Updated, Unchanged int
}

// Report summarises an import.
type Report struct {
	Entities map[string]*Counts
	// Events is the number of audit events written.
	Events int
	// Ops is the number of operations appended.
	Ops int
	// Skipped lists what the source did not let the import read.
	Skipped []string
}

// Changes is the number of records and events that were written.
func (r Report) Changes() int {
	n := r.Events
	for _, c := range r.Entities {
		n += c.Created + c.Updated
	}
	return n
}

// kinds that are replicated and readable through the API, in a stable order.
type entity struct {
	kind, path string
	local      func(context.Context, service.Service) ([]map[string]any, error)
}

func localAll[T any](ctx context.Context, r service.Resource[T]) ([]map[string]any, error) {
	var out []map[string]any
	for offset := int32(0); ; offset += 1000 {
		page, err := r.List(ctx, service.Filter{IncludeArchived: true, Limit: 1000, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, v := range page {
			doc, err := toMap(v)
			if err != nil {
				return nil, err
			}
			out = append(out, doc)
		}
		if len(page) < 1000 {
			return out, nil
		}
	}
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	return m, json.Unmarshal(raw, &m)
}

const itemPlanKind = "itemplan"

var entities = []entity{
	{"project", "/api/projects", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Projects())
	}},
	{"environment", "/api/environments", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Environments())
	}},
	{"repository", "/api/repositories", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Repositories())
	}},
	{"t3instance", "/api/t3-instances", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.T3Instances())
	}},
	{"feature", "/api/features", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Features())
	}},
	{"item", "/api/items", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Items())
	}},
	{itemPlanKind, "", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.ItemPlans())
	}},
	{"itemthread", "/api/item-threads", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.ItemThreads())
	}},
	{"issuethread", "/api/issue-threads", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.IssueThreads())
	}},
	{"memory", "/api/memories", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Memories())
	}},
	{"link", "/api/links", func(c context.Context, s service.Service) ([]map[string]any, error) {
		return localAll(c, s.Links())
	}},
}

// Kinds lists the entities the import copies, in report order.
func Kinds() []string {
	out := make([]string, len(entities))
	for i, e := range entities {
		out[i] = e.kind
	}
	return out
}

// fetched is everything read from the source.
type fetched struct {
	records map[string][]map[string]any
	events  []sourceEvent
	skipped []string
}

func fetch(ctx context.Context, src *source, eventsAfter int64) (*fetched, error) {
	f := &fetched{records: map[string][]map[string]any{}}
	for _, e := range entities {
		if e.path == "" {
			continue
		}
		recs, err := src.list(ctx, e.path, true)
		if err != nil {
			return nil, err
		}
		f.records[e.kind] = recs
	}
	// Plans are listed per item; the API hides the plans of archived items.
	for _, it := range f.records["item"] {
		id, _ := it["id"].(string)
		if it["archived_at"] != nil {
			f.skipped = append(f.skipped, "itemplan of archived item "+id)
			continue
		}
		plans, err := src.list(ctx, "/api/items/"+id+"/plans", false)
		if err != nil {
			return nil, err
		}
		f.records[itemPlanKind] = append(f.records[itemPlanKind], plans...)
	}
	evs, err := src.events(ctx, eventsAfter)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if syncx.EventReplicated(e.EntityType) && e.EntityType != "t3" {
			f.events = append(f.events, e)
		}
	}
	return f, nil
}

// Run imports the source into store, which must have been opened with Local.
func Run(ctx context.Context, store *localstore.Store, opts Options) (Report, error) {
	rep := Report{Entities: map[string]*Counts{}}
	if store.Log() == nil {
		return rep, fmt.Errorf("importer: the store must be opened with Local")
	}
	st, err := loadState(ctx, store, opts.device())
	if err != nil {
		return rep, err
	}
	f, err := fetch(ctx, opts.source(), st.maxEvent)
	if err != nil {
		return rep, err
	}
	rep.Skipped = f.skipped
	var plan []pending
	for _, e := range entities {
		c := &Counts{}
		rep.Entities[e.kind] = c
		local, err := e.local(ctx, store)
		if err != nil {
			return rep, err
		}
		byID := map[string]map[string]any{}
		for _, d := range local {
			byID[d["id"].(string)] = d
		}
		for _, rec := range f.records[e.kind] {
			c.Source++
			ops, created, err := st.diffRecord(e.kind, rec, byID)
			if err != nil {
				return rep, err
			}
			switch {
			case len(ops) == 0:
				c.Unchanged++
			case created:
				c.Created++
			default:
				c.Updated++
			}
			plan = append(plan, ops...)
		}
	}
	for _, ev := range f.events {
		p, err := eventOp(opts.device(), ev)
		if err != nil {
			return rep, err
		}
		plan = append(plan, p)
		rep.Events++
	}
	ops := st.stamp(plan)
	rep.Ops = len(ops)
	if len(ops) == 0 {
		return rep, nil
	}
	_, err = store.Log().ApplyNew(ctx, ops)
	return rep, err
}

// CheckRow compares the number of items of one entity.
type CheckRow struct {
	Entity        string
	Source, Local int
}

// OK reports whether both sides hold the same number.
func (r CheckRow) OK() bool { return r.Source == r.Local }

// Check compares, per entity (archived records included), the counts of the
// source with those of the store, plus the audit events imported so far.
func Check(ctx context.Context, store *localstore.Store, opts Options) ([]CheckRow, error) {
	if store.Log() == nil {
		return nil, fmt.Errorf("importer: the store must be opened with Local")
	}
	st, err := loadState(ctx, store, opts.device())
	if err != nil {
		return nil, err
	}
	f, err := fetch(ctx, opts.source(), 0)
	if err != nil {
		return nil, err
	}
	var rows []CheckRow
	for _, e := range entities {
		local, err := e.local(ctx, store)
		if err != nil {
			return nil, err
		}
		rows = append(rows, CheckRow{e.kind, len(f.records[e.kind]), len(local)})
	}
	rows = append(rows, CheckRow{"event", len(f.events), st.events})
	return rows, nil
}

// sensitive keys are dropped wherever they appear: nothing secret is imported.
var sensitive = map[string]bool{
	"token": true, "tokens": true, "secret": true, "secret_hash": true, "password": true,
	"password_hash": true, "api_key": true, "access_token": true, "refresh_token": true,
}

func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if sensitive[k] {
				delete(t, k)
				continue
			}
			t[k] = scrub(x)
		}
	case []any:
		for i := range t {
			t[i] = scrub(t[i])
		}
	}
	return v
}

var arrayFields = []string{"tags", "refs", "stack", "agents_available", "code_refs", "permissions", "open_questions"}

func isArray(k string) bool {
	for _, a := range arrayFields {
		if a == k {
			return true
		}
	}
	return false
}

var columns = map[string]bool{"id": true, "created_at": true, "updated_at": true, "archived_at": true}

// body is a record without column fields and without nulls, as the store keeps it.
func body(doc map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range doc {
		if columns[k] || v == nil || sensitive[k] {
			continue
		}
		out[k] = scrub(v)
	}
	return out
}

func sameScalar(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	as, ok1 := a.(string)
	bs, ok2 := b.(string)
	if ok1 && ok2 {
		ta, e1 := time.Parse(time.RFC3339Nano, as)
		tb, e2 := time.Parse(time.RFC3339Nano, bs)
		return e1 == nil && e2 == nil && ta.Equal(tb)
	}
	return false
}

func canon(v any) string { b, _ := json.Marshal(v); return string(b) }

func parseTime(v any) (time.Time, error) {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return t, fmt.Errorf("importer: invalid timestamp %q", s)
	}
	return t.UTC(), nil
}

// pending is an operation waiting for its final stamp and sequence number.
type pending struct {
	wall   int64
	entity string
	id     string
	kind   string
	fields syncx.Fields
}

func (st *state) diffRecord(kind string, rec map[string]any, local map[string]map[string]any) ([]pending, bool, error) {
	id, _ := rec["id"].(string)
	if id == "" {
		return nil, false, fmt.Errorf("importer: %s without id", kind)
	}
	created, err := parseTime(rec["created_at"])
	if err != nil {
		return nil, false, err
	}
	updated, err := parseTime(rec["updated_at"])
	if err != nil {
		return nil, false, err
	}
	var archived string
	if rec["archived_at"] != nil {
		at, err := parseTime(rec["archived_at"])
		if err != nil {
			return nil, false, err
		}
		archived = at.Format("2006-01-02T15:04:05.000000Z")
	}
	next := body(rec)
	old, exists := local[id]
	var prev map[string]any
	if exists {
		prev = body(old)
	}
	var f syncx.Fields
	keys := map[string]bool{}
	for k := range next {
		keys[k] = true
	}
	for k := range prev {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if isArray(k) {
			st.diffArray(&f, kind, id, k, next[k])
			continue
		}
		if sameScalar(prev[k], next[k]) {
			continue
		}
		if f.Set == nil {
			f.Set = map[string]any{}
		}
		f.Set[k] = next[k] // nil clears the field
		st.base(&f, kind, id, k)
	}
	archiveNow := archived != "" && (!exists || old["archived_at"] == nil)
	if !exists {
		if f.Empty() {
			f.Set = map[string]any{"status": nil}
		}
		ops := []pending{{wall: created.UnixMilli(), entity: kind, id: id, kind: syncx.KindPut, fields: f}}
		if archiveNow {
			ops = append(ops, pending{wall: updated.UnixMilli(), entity: kind, id: id, kind: syncx.KindPut,
				fields: syncx.Fields{Set: map[string]any{"archived_at": archived}}})
		} else if updated.UnixMilli() > created.UnixMilli() {
			if k, ok := touchKey(next); ok {
				ops = append(ops, pending{wall: updated.UnixMilli(), entity: kind, id: id, kind: syncx.KindPut,
					fields: syncx.Fields{Set: map[string]any{k: next[k]}}})
			}
		}
		return ops, true, nil
	}
	if archiveNow {
		if f.Set == nil {
			f.Set = map[string]any{}
		}
		f.Set["archived_at"] = archived
		st.base(&f, kind, id, "archived_at")
	}
	if f.Empty() {
		return nil, false, nil
	}
	return []pending{{wall: updated.UnixMilli(), entity: kind, id: id, kind: syncx.KindPut, fields: f}}, false, nil
}

// touchKey picks a scalar to rewrite with its own value, so that the last
// change of a record gets the stamp of updated_at.
func touchKey(doc map[string]any) (string, bool) {
	names := make([]string, 0, len(doc))
	for k := range doc {
		if !isArray(k) {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

func eventOp(device string, e sourceEvent) (pending, error) {
	var change any
	if err := json.Unmarshal(e.Change, &change); err != nil {
		return pending{}, fmt.Errorf("importer: event %d: %w", e.ID, err)
	}
	at := e.At.UTC()
	f := syncx.Fields{Event: map[string]any{
		"device": device, "seq": e.ID, "entity_type": e.EntityType, "entity_id": e.EntityID,
		"actor": e.Actor, "change": scrub(change), "at": at.Format("2006-01-02T15:04:05.000000Z"),
	}}
	return pending{wall: at.UnixMilli(), entity: "event", id: fmt.Sprintf("%s:%d", device, e.ID), kind: syncx.KindEvent, fields: f}, nil
}
