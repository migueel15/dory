package localstore

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/syncx"
)

var ctx = context.Background()

// replica opens a store with its own skewed, strictly increasing clock.
func replica(t testing.TB, device string, skew time.Duration) *Store {
	t.Helper()
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Add(skew)
	s, err := Open(filepath.Join(t.TempDir(), device+".db"), Options{Device: device, Local: true,
		Clock: func() time.Time { at = at.Add(3 * time.Millisecond); return at }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func allOps(t testing.TB, s *Store) []syncx.Op {
	t.Helper()
	ops, err := s.Log().Since(ctx, nil, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

// pull brings dst up to date with src through the version vector.
func pull(t testing.TB, dst, src *Store) {
	t.Helper()
	for {
		vv, err := dst.Log().Vector(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ops, err := src.Log().Since(ctx, vv, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(ops) == 0 {
			return
		}
		if err = dst.Log().Apply(ctx, ops); err != nil {
			t.Fatal(err)
		}
	}
}

func syncAll(t testing.TB, rs ...*Store) {
	t.Helper()
	for round := 0; round < 2; round++ {
		for _, a := range rs {
			for _, b := range rs {
				if a != b {
					pull(t, a, b)
				}
			}
		}
	}
}

// dump is the whole replicated state of a store as text. Local event ids and
// the contents of the auth tables are left out.
func dump(t testing.TB, s *Store) string {
	t.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT kind, id, data, field_clock, owner_device, created_at, updated_at, coalesce(archived_at, 'NULL') FROM records ORDER BY kind, id`,
		`SELECT kind, id, field, elem, adds, rems FROM set_elems ORDER BY kind, id, field, elem`,
		`SELECT id, intent, rank FROM sync_slugs ORDER BY id`,
		`SELECT device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v FROM ops ORDER BY device_id, seq`,
		`SELECT device, seq, entity_type, entity_id, actor, change, at FROM events ORDER BY device, seq`,
		`SELECT entity_type, entity_id, body FROM search ORDER BY entity_type, entity_id, body`,
	} {
		rows, err := s.root.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err = rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&sb, vals...)
		}
		rows.Close()
		sb.WriteString("--\n")
	}
	return sb.String()
}

func conflicts(t testing.TB, s *Store) []string {
	t.Helper()
	rows, err := s.root.Query(`SELECT change FROM events WHERE device = 'conflict' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		var m struct{ Field, Text string }
		json.Unmarshal([]byte(c), &m)
		out = append(out, m.Field+"="+m.Text)
	}
	return out
}

func mustItem(t testing.TB, s *Store, in service.Item) service.Item {
	t.Helper()
	if in.Title == "" {
		in.Title = "t"
	}
	it, err := s.Items().Put(ctx, "u", in)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func mustProject(t testing.TB, s *Store, slug string) service.Project {
	t.Helper()
	p, err := s.Projects().Put(ctx, "u", service.Project{Slug: slug, Name: slug})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConcurrentEditSameField(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", time.Second)
	it := mustItem(t, a, service.Item{BodyMd: "base"})
	syncAll(t, a, b)

	ia, _ := a.Items().Get(ctx, it.ID)
	ia.BodyMd = "from a"
	ia.Title = "title a"
	a.Items().Put(ctx, "u", ia)
	ib, _ := b.Items().Get(ctx, it.ID)
	ib.BodyMd = "from b"
	b.Items().Put(ctx, "u", ib)
	syncAll(t, a, b)

	ga, _ := a.Items().Get(ctx, it.ID)
	gb, _ := b.Items().Get(ctx, it.ID)
	if ga.BodyMd != "from b" || gb.BodyMd != "from b" { // b's clock is ahead
		t.Fatalf("bodies %q %q", ga.BodyMd, gb.BodyMd)
	}
	if ga.Title != "title a" || gb.Title != "title a" {
		t.Fatalf("independent field lost: %q %q", ga.Title, gb.Title)
	}
	want := []string{"body_md=from a"}
	if !slices.Equal(conflicts(t, a), want) || !slices.Equal(conflicts(t, b), want) {
		t.Fatalf("conflicts %v %v", conflicts(t, a), conflicts(t, b))
	}
	if dump(t, a) != dump(t, b) {
		t.Fatal("replicas differ")
	}
}

func TestSequentialEditsAreNotConflicts(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", time.Second)
	it := mustItem(t, a, service.Item{BodyMd: "one"})
	syncAll(t, a, b)
	ib, _ := b.Items().Get(ctx, it.ID)
	ib.BodyMd = "two"
	b.Items().Put(ctx, "u", ib)
	syncAll(t, a, b)
	ia, _ := a.Items().Get(ctx, it.ID)
	ia.BodyMd = "three"
	a.Items().Put(ctx, "u", ia)
	syncAll(t, a, b)
	if c := conflicts(t, a); len(c) != 0 {
		t.Fatalf("conflicts %v", c)
	}
	if g, _ := b.Items().Get(ctx, it.ID); g.BodyMd != "three" {
		t.Fatalf("body %q", g.BodyMd)
	}
}

func TestEditVersusArchive(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	it := mustItem(t, a, service.Item{})
	syncAll(t, a, b)
	ia, _ := a.Items().Get(ctx, it.ID)
	ia.Progress = "edited"
	a.Items().Put(ctx, "u", ia)
	if err := b.Items().Archive(ctx, "u", it.ID); err != nil {
		t.Fatal(err)
	}
	syncAll(t, a, b)
	for _, s := range []*Store{a, b} {
		if _, err := s.Items().Get(ctx, it.ID); err != service.ErrNotFound {
			t.Fatalf("archived item visible: %v", err)
		}
		list, _ := s.Items().List(ctx, service.Filter{IncludeArchived: true})
		if len(list) != 1 || list[0].ArchivedAt == nil || list[0].Progress != "edited" {
			t.Fatalf("list %+v", list)
		}
		if hits, _ := s.Search(ctx, "t", 10, 0); len(hits) != 0 {
			t.Fatalf("archived item still indexed: %d", len(hits))
		}
	}
	if dump(t, a) != dump(t, b) {
		t.Fatal("replicas differ")
	}
}

func TestParallelRefsAndAddWins(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	it := mustItem(t, a, service.Item{Refs: []string{"base"}})
	syncAll(t, a, b)
	edit := func(s *Store, f func(*service.Item)) {
		x, _ := s.Items().Get(ctx, it.ID)
		f(&x)
		if _, err := s.Items().Put(ctx, "u", x); err != nil {
			t.Fatal(err)
		}
	}
	edit(a, func(x *service.Item) { x.Refs = append(x.Refs, "pr-1") })
	edit(b, func(x *service.Item) { x.Refs = append(x.Refs, "pr-2") })
	syncAll(t, a, b)
	for _, s := range []*Store{a, b} {
		g, _ := s.Items().Get(ctx, it.ID)
		got := slices.Sorted(slices.Values(g.Refs))
		if !slices.Equal(got, []string{"base", "pr-1", "pr-2"}) {
			t.Fatalf("refs %v", g.Refs)
		}
	}
	// a removes pr-1 while b adds it again: the add wins.
	edit(a, func(x *service.Item) { x.Refs = []string{"base", "pr-2"} })
	edit(b, func(x *service.Item) { x.Refs = []string{"base", "pr-2"} })
	edit(b, func(x *service.Item) { x.Refs = []string{"base", "pr-2", "pr-1"} }) // fresh tag a never saw
	syncAll(t, a, b)
	if g, _ := a.Items().Get(ctx, it.ID); len(g.Refs) != 3 || !slices.Contains(g.Refs, "pr-1") || !slices.Contains(g.Refs, "pr-2") {
		t.Fatalf("add did not win: %v", g.Refs)
	}
	// b removes pr-2, which a had already seen: removed everywhere.
	edit(b, func(x *service.Item) { x.Refs = []string{"base", "pr-1"} })
	syncAll(t, a, b)
	for _, s := range []*Store{a, b} {
		g, _ := s.Items().Get(ctx, it.ID)
		if !slices.Equal(g.Refs, []string{"base", "pr-1"}) {
			t.Fatalf("refs after removal %v", g.Refs)
		}
	}
	if dump(t, a) != dump(t, b) {
		t.Fatal("replicas differ")
	}
}

func TestConcurrentRemoveOfSameElementAndAddWins(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	it := mustItem(t, a, service.Item{Tags: []string{"x"}})
	syncAll(t, a, b)
	set := func(s *Store, tags []string) {
		x, _ := s.Items().Get(ctx, it.ID)
		x.Tags = tags
		s.Items().Put(ctx, "u", x)
	}
	set(a, []string{})
	set(b, []string{})
	set(b, []string{"x"}) // re-adds under a fresh tag the removal never saw
	syncAll(t, a, b)
	for _, s := range []*Store{a, b} {
		if g, _ := s.Items().Get(ctx, it.ID); !slices.Equal(g.Tags, []string{"x"}) {
			t.Fatalf("tags %v", g.Tags)
		}
	}
}

func TestDuplicateSlugRenamedDeterministically(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	pa := mustProject(t, a, "dup")
	pb := mustProject(t, b, "dup")
	syncAll(t, a, b)
	slugs := func(s *Store) map[string]string {
		list, err := s.Projects().List(ctx, service.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		for _, p := range list {
			m[p.ID] = p.Slug
		}
		return m
	}
	sa, sb := slugs(a), slugs(b)
	if fmt.Sprint(sa) != fmt.Sprint(sb) || len(sa) != 2 {
		t.Fatalf("slugs differ: %v %v", sa, sb)
	}
	// a wrote first (earlier clock), so a keeps the slug.
	if sa[pa.ID] != "dup" || sa[pb.ID] != "dup-2" {
		t.Fatalf("slugs %v", sa)
	}
	// Renaming the owner frees the slug for the other project.
	p, _ := a.Projects().Get(ctx, pa.ID)
	p.Slug = "renamed"
	if _, err := a.Projects().Put(ctx, "u", p); err != nil {
		t.Fatal(err)
	}
	syncAll(t, a, b)
	if sa, sb = slugs(a), slugs(b); sa[pb.ID] != "dup" || sb[pa.ID] != "renamed" {
		t.Fatalf("after rename %v %v", sa, sb)
	}
	if dump(t, a) != dump(t, b) {
		t.Fatal("replicas differ")
	}
}

func TestChildBeforeParent(t *testing.T) {
	a := replica(t, "a", 0)
	b := replica(t, "b", -time.Hour) // b's wall clock is far behind
	pa := mustProject(t, a, "p")
	pull(t, b, a)
	child := mustItem(t, b, service.Item{ProjectID: &pa.ID})
	// HLC: b observed the parent, so the child's stamp is above it.
	var parent, kid syncx.Op
	for _, o := range allOps(t, b) {
		switch o.EntityID {
		case pa.ID:
			parent = o
		case child.ID:
			kid = o
		}
	}
	if kid.HLC <= parent.HLC {
		t.Fatalf("child %s not after parent %s", kid.HLC, parent.HLC)
	}
	// A third replica receives the child first and the parent later.
	c := replica(t, "c", 0)
	var childOps, parentOps []syncx.Op
	for _, o := range allOps(t, b) {
		if o.EntityID == pa.ID {
			parentOps = append(parentOps, o)
		} else if o.Entity != "event" || strings.Contains(string(o.Fields), child.ID) {
			childOps = append(childOps, o)
		}
	}
	if err := c.Log().Apply(ctx, childOps); err != nil {
		t.Fatal(err)
	}
	if g, err := c.Items().Get(ctx, child.ID); err != nil || *g.ProjectID != pa.ID {
		t.Fatalf("orphan not stored: %+v %v", g, err)
	}
	if err := c.Log().Apply(ctx, parentOps); err != nil {
		t.Fatal(err)
	}
	pull(t, c, b)
	pull(t, c, a)
	syncAll(t, a, b, c)
	if dump(t, a) != dump(t, c) || dump(t, b) != dump(t, c) {
		t.Fatal("replicas differ")
	}
}

func TestApplyNeverCreatesOpsAndForwardsUnknownFields(t *testing.T) {
	a, b, c := replica(t, "a", 0), replica(t, "b", 0), replica(t, "c", 0)
	p := mustProject(t, a, "p")
	h := syncx.HLC{Wall: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), Device: "future"}
	future := syncx.Op{DeviceID: "future", Seq: 1, HLC: h.String(), Entity: "project", EntityID: p.ID, Kind: syncx.KindPut, SchemaV: 7,
		Fields: json.RawMessage(`{"set":{"color":"teal","name":"new"},"unknown_top":[1,2]}`)}
	if err := a.Log().Apply(ctx, []syncx.Op{future, future}); err != nil {
		t.Fatal(err)
	}
	// project put + its audit event + the future op; applying added nothing else
	before := len(allOps(t, a))
	if before != 3 {
		t.Fatalf("ops = %d", before)
	}
	syncAll(t, a, b, c)
	if len(allOps(t, b)) != before {
		t.Fatalf("b has %d ops, a has %d", len(allOps(t, b)), before)
	}
	var data string
	b.root.QueryRow(`SELECT data FROM records WHERE id = ?`, p.ID).Scan(&data)
	if !strings.Contains(data, `"color":"teal"`) || !strings.Contains(data, `"name":"new"`) {
		t.Fatalf("data %s", data)
	}
	var raw string
	var v int
	c.root.QueryRow(`SELECT fields_json, schema_v FROM ops WHERE device_id = 'future'`).Scan(&raw, &v)
	if raw != string(future.Fields) || v != 7 {
		t.Fatalf("forwarded op = %s v%d", raw, v)
	}
	if g, _ := c.Projects().Get(ctx, p.ID); g.Name != "new" {
		t.Fatalf("name %q", g.Name)
	}
	// The local clock moved past the future stamp.
	q := mustProject(t, a, "q")
	for _, o := range allOps(t, a) {
		if o.EntityID == q.ID && o.HLC <= h.String() {
			t.Fatalf("clock did not observe the remote stamp: %s", o.HLC)
		}
	}
}

func TestSearchStaysConsistentAfterApply(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	it := mustItem(t, a, service.Item{Title: "alpha plan", BodyMd: "zebra"})
	syncAll(t, a, b)
	if hits, _ := b.Search(ctx, "zebra", 10, 0); len(hits) != 1 {
		t.Fatalf("hits %d", len(hits))
	}
	x, _ := a.Items().Get(ctx, it.ID)
	x.BodyMd = "giraffe"
	a.Items().Put(ctx, "u", x)
	syncAll(t, a, b)
	if hits, _ := b.Search(ctx, "zebra", 10, 0); len(hits) != 0 {
		t.Fatal("stale index")
	}
	if hits, _ := b.Search(ctx, "giraffe", 10, 0); len(hits) != 1 {
		t.Fatal("new text not indexed")
	}
}

func TestVectorAndSinceWithGaps(t *testing.T) {
	a, b := replica(t, "a", 0), replica(t, "b", 0)
	for i := 0; i < 4; i++ {
		mustProject(t, a, fmt.Sprintf("p%d", i))
	}
	ops := allOps(t, a)
	byDevice := []syncx.Op{}
	for _, o := range ops {
		if o.Seq != 2 { // cut: seq 2 is missing
			byDevice = append(byDevice, o)
		}
	}
	b.Log().Apply(ctx, byDevice)
	vv, _ := b.Log().Vector(ctx)
	if vv["a"] != 1 {
		t.Fatalf("vector %v", vv)
	}
	missing, _ := a.Log().Since(ctx, vv, 1000)
	if len(missing) != len(ops)-1 {
		t.Fatalf("since returned %d", len(missing))
	}
	pull(t, b, a)
	if vv, _ = b.Log().Vector(ctx); vv["a"] != int64(len(ops)) {
		t.Fatalf("vector after catch-up %v", vv)
	}
	if lim, _ := a.Log().Since(ctx, nil, 3); len(lim) != 3 {
		t.Fatalf("limit not honoured: %d", len(lim))
	}
	if dump(t, a) != dump(t, b) {
		t.Fatal("replicas differ")
	}
}

func TestClockSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	frozen := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	s, _ := Open(path, Options{Device: "x", Local: true, Clock: frozen})
	mustProject(t, s, "one")
	s.Close()
	s, _ = Open(path, Options{Device: "x", Local: true, Clock: frozen})
	defer s.Close()
	mustProject(t, s, "two")
	ops := allOps(t, s)
	for i := 1; i < len(ops); i++ {
		if ops[i-1].HLC >= ops[i].HLC {
			t.Fatalf("hlc not increasing: %s then %s", ops[i-1].HLC, ops[i].HLC)
		}
	}
}

func TestLocalKindsDoNotReplicate(t *testing.T) {
	a := replica(t, "a", 0)
	if _, err := a.Users().Put(ctx, "u", service.User{Username: "me"}); err != nil {
		t.Fatal(err)
	}
	if n := len(allOps(t, a)); n != 0 {
		t.Fatalf("user produced %d ops", n)
	}
}

// ---- property test ----

func randomHistory(t testing.TB, rng *rand.Rand, replicas []*Store, steps int) {
	words := []string{"red", "green", "blue", "gold", "plum"}
	pick := func() string { return words[rng.Intn(len(words))] }
	for i := 0; i < steps; i++ {
		s := replicas[rng.Intn(len(replicas))]
		projects, _ := s.Projects().List(ctx, service.Filter{Limit: 1000})
		items, _ := s.Items().List(ctx, service.Filter{Limit: 1000})
		memories, _ := s.Memories().List(ctx, service.Filter{Limit: 1000})
		switch n := rng.Intn(12); {
		case n == 0:
			s.Projects().Put(ctx, "u", service.Project{Slug: []string{"a", "b", "c"}[rng.Intn(3)], Name: pick(), Stack: []string{pick()}})
		case n == 1 && len(projects) > 0:
			p := projects[rng.Intn(len(projects))]
			p.Name, p.Description = pick(), pick()+" "+pick()
			if rng.Intn(3) == 0 {
				p.Slug = []string{"a", "b", "c", "d"}[rng.Intn(4)]
			}
			if rng.Intn(2) == 0 {
				p.Stack = append(p.Stack, pick())
			} else if len(p.Stack) > 0 {
				p.Stack = p.Stack[1:]
			}
			s.Projects().Put(ctx, "u", p)
		case n == 2 && len(projects) > 0 && rng.Intn(4) == 0:
			s.Projects().Archive(ctx, "u", projects[rng.Intn(len(projects))].ID)
		case n <= 4:
			in := service.Item{Title: pick() + " " + pick(), BodyMd: pick(), Tags: []string{pick()}}
			if len(projects) > 0 && rng.Intn(2) == 0 {
				in.ProjectID = &projects[rng.Intn(len(projects))].ID
			}
			s.Items().Put(ctx, "u", in)
		case n <= 8 && len(items) > 0:
			it := items[rng.Intn(len(items))]
			switch rng.Intn(4) {
			case 0:
				it.BodyMd = pick() + " " + pick()
			case 1:
				it.Progress = pick()
			case 2:
				it.Refs = append(it.Refs, pick())
			default:
				if len(it.Tags) > 0 && rng.Intn(2) == 0 {
					it.Tags = it.Tags[1:]
				} else {
					it.Tags = append(it.Tags, pick())
				}
			}
			s.Items().Put(ctx, "u", it)
		case n == 9 && len(items) > 0 && rng.Intn(3) == 0:
			s.Items().Archive(ctx, "u", items[rng.Intn(len(items))].ID)
		case n == 10:
			s.Memories().Put(ctx, "u", service.Memory{Scope: "global", Kind: "lesson", Title: pick(), Content: pick()})
		case n == 11 && len(memories) > 0:
			m := memories[rng.Intn(len(memories))]
			m.Content = pick() + " " + pick()
			s.Memories().Put(ctx, "u", m)
		}
		if rng.Intn(5) == 0 { // partial sync between two replicas
			a, b := replicas[rng.Intn(len(replicas))], replicas[rng.Intn(len(replicas))]
			if a != b {
				vv, _ := a.Log().Vector(ctx)
				ops, _ := b.Log().Since(ctx, vv, 1+rng.Intn(6))
				if err := a.Log().Apply(ctx, ops); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestConvergenceProperty(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			n := 3 + rng.Intn(3)
			var origin []*Store
			for i := 0; i < n; i++ {
				origin = append(origin, replica(t, fmt.Sprintf("dev%d", i), time.Duration(rng.Intn(4000)-2000)*time.Millisecond))
			}
			randomHistory(t, rng, origin, 90)
			var all []syncx.Op
			seen := map[string]bool{}
			for _, s := range origin {
				for _, o := range allOps(t, s) {
					k := fmt.Sprint(o.DeviceID, "/", o.Seq)
					if !seen[k] {
						seen[k] = true
						all = append(all, o)
					}
				}
			}
			if len(all) < 20 {
				t.Fatalf("history too short: %d", len(all))
			}
			syncAll(t, origin...)
			want := dump(t, origin[0])
			for i, s := range origin[1:] {
				if dump(t, s) != want {
					t.Fatalf("origin %d diverged after full sync", i+1)
				}
			}
			for r := 0; r < 5; r++ {
				fresh := replica(t, fmt.Sprintf("fresh%d", r), 0)
				// permuted, duplicated, applied in cuts that overlap
				list := slices.Clone(all)
				rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
				for i := 0; i < len(list)/4; i++ {
					list = append(list, all[rng.Intn(len(all))])
				}
				rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
				for len(list) > 0 {
					cut := min(len(list), 1+rng.Intn(25))
					if err := fresh.Log().Apply(ctx, list[:cut]); err != nil {
						t.Fatal(err)
					}
					list = list[cut:]
					if rng.Intn(3) == 0 { // replay an earlier chunk
						k := rng.Intn(len(all))
						fresh.Log().Apply(ctx, all[k:min(len(all), k+10)])
					}
				}
				if got := dump(t, fresh); got != want {
					t.Fatalf("fresh replica %d differs:\n%s", r, firstDiff(want, got))
				}
			}
		})
	}
}

func firstDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < min(len(la), len(lb)); i++ {
		if la[i] != lb[i] {
			return "want: " + la[i] + "\n got: " + lb[i]
		}
	}
	return fmt.Sprintf("length %d vs %d", len(la), len(lb))
}
