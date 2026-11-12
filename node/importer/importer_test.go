package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
)

type fixture struct {
	src  *localstore.Store
	dst  *localstore.Store
	opts Options
	ids  map[string]string
	tick func() time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	var n atomic.Int64
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	tick := func() time.Time { return base.Add(time.Duration(n.Add(1)) * 1500 * time.Millisecond) }
	src, err := localstore.Open(":memory:", localstore.Options{Device: "pg", Clock: tick})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { src.Close() })
	dst, err := localstore.Open(t.TempDir()+"/dst.db", localstore.Options{Device: "dst", Local: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dst.Close() })

	mux := http.NewServeMux()
	api.Register(mux, src, nil, nil, nil)
	var auth atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer secret" {
			auth.Add(1)
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return &fixture{src: src, dst: dst, tick: tick, ids: map[string]string{},
		opts: Options{BaseURL: srv.URL, Token: "secret", Idle: 300 * time.Millisecond}}
}

func (f *fixture) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := f.src.Projects().Put(ctx, "u", service.Project{Slug: "dory", Name: "Dory", Stack: []string{"go", "react"}})
	must(err)
	env, err := f.src.Environments().Put(ctx, "u", service.Environment{Name: "laptop", AgentsAvailable: []string{"claude"}})
	must(err)
	repo, err := f.src.Repositories().Put(ctx, "u", service.Repository{ProjectID: p.ID, RemoteUrl: "https://example.test/dory.git"})
	must(err)
	feat, err := f.src.Features().Put(ctx, "u", service.Feature{ProjectID: p.ID, Title: "Sync", BodyMd: "body", CodeRefs: []string{"a.go"}})
	must(err)
	_, err = f.src.T3Instances().Put(ctx, "u", service.T3Instance{EnvironmentID: env.ID, BaseUrl: "http://t3", TokenEnv: "T3_TOKEN"})
	must(err)
	pid, fid := p.ID, feat.ID
	item, err := f.src.Items().Put(ctx, "u", service.Item{ProjectID: &pid, FeatureID: &fid, Title: "Do it", BodyMd: "v1", Tags: []string{"a", "b"}, Refs: []string{"pr#1"}})
	must(err)
	old, err := f.src.Items().Put(ctx, "u", service.Item{Title: "Old", Status: "done"})
	must(err)
	must(f.src.Items().Archive(ctx, "u", old.ID))
	_, err = f.src.ItemPlans().Put(ctx, "u", service.ItemPlan{ItemID: item.ID, Sections: service.PlanSections{Goal: "g"}, OpenQuestions: []string{"q?"}})
	must(err)
	mem, err := f.src.Memories().Put(ctx, "u", service.Memory{Scope: "global", Kind: "lesson", Title: "T", Content: "C", Tags: []string{"x"}})
	must(err)
	_, err = f.src.Links().Put(ctx, "u", service.Link{FromID: item.ID, FromType: "item", ToID: mem.ID, ToType: "memory", Kind: "relates"})
	must(err)
	// Credentials must never reach the node.
	_, err = f.src.Users().Put(ctx, "u", service.User{Username: "root", PasswordHash: "hash"})
	must(err)
	f.ids["project"], f.ids["item"], f.ids["old"], f.ids["repo"] = p.ID, item.ID, old.ID, repo.ID
}

func (f *fixture) check(t *testing.T, skip ...string) {
	t.Helper()
	rows, err := Check(context.Background(), f.dst, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if !r.OK() && !slices.Contains(skip, r.Entity) {
			t.Errorf("%s: source=%d local=%d", r.Entity, r.Source, r.Local)
		}
	}
}

func TestImportCountsFieldsAndIdempotence(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.seed(t)

	rep, err := Run(ctx, f.dst, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if c := rep.Entities["item"]; c.Source != 2 || c.Created != 2 {
		t.Fatalf("items = %+v", c)
	}
	if rep.Events == 0 {
		t.Fatal("no events imported")
	}
	f.check(t)

	// Fields, ids and timestamps survive (timestamps to the millisecond).
	want, _ := f.src.Items().Get(ctx, f.ids["item"])
	got, err := f.dst.Items().Get(ctx, f.ids["item"])
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != want.Title || got.BodyMd != want.BodyMd || !reflect.DeepEqual(got.Tags, want.Tags) ||
		!reflect.DeepEqual(got.Refs, want.Refs) || *got.ProjectID != *want.ProjectID ||
		got.CreatedAt.Truncate(time.Millisecond) != want.CreatedAt.Truncate(time.Millisecond) ||
		got.UpdatedAt.Truncate(time.Millisecond) != want.UpdatedAt.Truncate(time.Millisecond) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	srcAll, _ := f.src.Items().List(ctx, service.Filter{IncludeArchived: true})
	dstAll, _ := f.dst.Items().List(ctx, service.Filter{IncludeArchived: true})
	var arch *time.Time
	for _, it := range dstAll {
		if it.ID == f.ids["old"] {
			arch = it.ArchivedAt
		}
	}
	for _, it := range srcAll {
		if it.ID == f.ids["old"] && (arch == nil || !arch.Equal(*it.ArchivedAt)) {
			t.Fatalf("archived_at = %v, want %v", arch, it.ArchivedAt)
		}
	}
	if _, err = f.dst.Items().Get(ctx, f.ids["old"]); err == nil {
		t.Fatal("archived item is live")
	}
	plans, _ := f.dst.ItemPlans().List(ctx, service.Filter{})
	if len(plans) != 1 || plans[0].Sections.Goal != "g" || plans[0].Version != 1 || len(plans[0].OpenQuestions) != 1 {
		t.Fatalf("plans = %+v", plans)
	}
	if us, _ := f.dst.Users().List(ctx, service.Filter{}); len(us) != 0 {
		t.Fatalf("users imported: %+v", us)
	}
	if evs, _ := f.dst.ListEvents(ctx, 0, "", 1000); len(evs) != rep.Events {
		t.Fatalf("events = %d, want %d", len(evs), rep.Events)
	}

	// Nothing changed: nothing is written.
	again, err := Run(ctx, f.dst, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changes() != 0 || again.Ops != 0 {
		t.Fatalf("second run = %+v", again)
	}

	// Changes in the source are carried over without duplicates.
	it := want
	it.BodyMd = "v2"
	it.Tags = []string{"b", "c"}
	if _, err = f.src.Items().Put(ctx, "u", it); err != nil {
		t.Fatal(err)
	}
	if err = f.src.Projects().Archive(ctx, "u", f.ids["project"]); err != nil {
		t.Fatal(err)
	}
	third, err := Run(ctx, f.dst, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if third.Entities["item"].Updated == 0 || third.Entities["project"].Updated != 1 || third.Events == 0 {
		t.Fatalf("third run = %+v", third.Entities)
	}
	// The API hides the plans of archived items, so they cannot be compared.
	f.check(t, "itemplan")
	// The project cascade also archived the item.
	if _, err = f.dst.Projects().Get(ctx, f.ids["project"]); err == nil {
		t.Fatal("project still live")
	}
	dstAll, _ = f.dst.Items().List(ctx, service.Filter{IncludeArchived: true})
	if len(dstAll) != 2 {
		t.Fatalf("items = %d", len(dstAll))
	}
	for _, x := range dstAll {
		if x.ID == f.ids["item"] && (x.BodyMd != "v2" || !reflect.DeepEqual(x.Tags, []string{"b", "c"}) || x.ArchivedAt == nil) {
			t.Fatalf("item = %+v", x)
		}
	}
	last, err := Run(ctx, f.dst, f.opts)
	if err != nil || last.Changes() != 0 {
		t.Fatalf("fourth run = %+v, %v", last, err)
	}
}

func TestImportRejectsBadToken(t *testing.T) {
	f := newFixture(t)
	f.opts.Token = "wrong"
	if _, err := Run(context.Background(), f.dst, f.opts); err == nil {
		t.Fatal("expected an error")
	}
}

func TestImportSkipsAndArchivesT3Instances(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.seed(t)
	rep, err := Run(ctx, f.dst, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rep.Entities["t3instance"]; ok {
		t.Fatal("t3instance must not be imported")
	}
	if ins, _ := f.dst.T3Instances().List(ctx, service.Filter{IncludeArchived: true}); len(ins) != 0 {
		t.Fatalf("imported %d t3 instances", len(ins))
	}
	// An instance owned by the import device (left by earlier versions) is archived.
	env, err := f.dst.Environments().Put(ctx, "u", service.Environment{Name: "e"})
	if err != nil {
		t.Fatal(err)
	}
	in, err := f.dst.T3Instances().Put(ctx, "u", service.T3Instance{EnvironmentID: env.ID, BaseUrl: "http://t3"})
	if err != nil {
		t.Fatal(err)
	}
	opts := f.opts
	opts.Device = "dst" // the owner of in
	if _, err := Run(ctx, f.dst, opts); err != nil {
		t.Fatal(err)
	}
	if live, _ := f.dst.T3Instances().List(ctx, service.Filter{}); len(live) != 0 {
		t.Fatalf("live instances after import: %+v (%s)", live, in.ID)
	}
}
