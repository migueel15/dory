package titles

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

func TestPromptCarriesIDAndDescription(t *testing.T) {
	p := Prompt(service.Item{Record: service.Record{ID: "abc"}, RawText: "arreglar el login\ncon detalle"})
	for _, want := range []string{"`abc`", "update_item", `title_source: "ai"`, "archiva tu propio hilo", "t3_thread_update", "arreglar el login\ncon detalle"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

func TestPickUsesLowestEffortAndConfig(t *testing.T) {
	cat := t3.Catalog{
		Projects: []t3.CatalogProject{{ID: "p1", Title: "uno"}, {ID: "p2", Title: "Dos"}},
		Providers: []t3.CatalogProvider{{InstanceID: "codex", Models: []t3.CatalogModel{
			{Slug: "luna", Effort: &t3.EffortOption{ID: "reasoningEffort", Values: []string{"minimal", "low", "high"}}},
		}}},
	}
	in, ok := pick(cat, Config{Model: "luna", Project: "dos"})
	if !ok || in.Provider != "codex" || in.ProjectID != "p2" || in.EffortOption != "reasoningEffort" || in.Effort != "minimal" {
		t.Fatalf("got %+v %v", in, ok)
	}
	if in, _ = pick(cat, Config{Model: "luna", Effort: "low"}); in.Effort != "low" || in.ProjectID != "p1" {
		t.Fatalf("got %+v", in)
	}
	if _, ok = pick(cat, Config{Model: "otro"}); ok {
		t.Fatal("unknown model must not be picked")
	}
	if _, ok = pick(cat, Config{Model: "luna", Provider: "claude"}); ok {
		t.Fatal("unknown provider must not be picked")
	}
}

// fakeItems is the minimal item store Generate and giveUp use.
type fakeItems struct {
	service.Resource[service.Item]
	item service.Item
}

func (f *fakeItems) Get(context.Context, string) (service.Item, error) { return f.item, nil }
func (f *fakeItems) Put(_ context.Context, _ string, v service.Item) (service.Item, error) {
	f.item = v
	return v, nil
}

type fakeService struct {
	service.Service
	items *fakeItems
}

func (s fakeService) Items() service.Resource[service.Item] { return s.items }

func newGen(item service.Item, launch func(context.Context, string, string) error, timeout time.Duration) (*Generator, *fakeItems, Config) {
	items := &fakeItems{item: item}
	pollEvery = 5 * time.Millisecond
	cfg := Config{Model: "m", Timeout: timeout}
	return &Generator{svc: fakeService{items: items}, load: func(context.Context) (Config, error) { return cfg, nil },
		launch: func(ctx context.Context, _ Config, title, prompt string) error { return launch(ctx, title, prompt) }}, items, cfg
}

func TestSettingsConfig(t *testing.T) {
	if c := (Settings{Model: "m"}).Config(); c.Model != "" || c.Timeout != DefaultTimeout {
		t.Fatalf("disabled settings must turn generation off: %+v", c)
	}
	c := Settings{Enabled: true, InstanceID: "i", Provider: "codex", Model: "m", Effort: "low", TimeoutSeconds: 30}.Config()
	if c.Instance != "i" || c.Model != "m" || c.Effort != "low" || c.Timeout != 30*time.Second {
		t.Fatalf("got %+v", c)
	}
	if (Settings{Enabled: true, Model: "m"}).valid() || !(Settings{Enabled: true, InstanceID: "i", Model: "m"}).valid() || !(Settings{}).valid() {
		t.Fatal("enabled settings need an instance and a model")
	}
}

func TestGenerateFailureLeavesUserTitle(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", TitleSource: service.TitlePending}
	g, items, cfg := newGen(item, func(context.Context, string, string) error { return errors.New("t3 caído") }, time.Second)
	g.Generate(item, cfg)
	if items.item.TitleSource != service.TitleUser || items.item.Title != "borrador" {
		t.Fatalf("got %+v", items.item)
	}
}

func TestGenerateTimeoutLeavesUserTitle(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", TitleSource: service.TitlePending}
	var prompt string
	g, items, cfg := newGen(item, func(_ context.Context, _, p string) error { prompt = p; return nil }, 40*time.Millisecond)
	g.Generate(item, cfg)
	if prompt != Prompt(item) {
		t.Fatalf("unexpected prompt %q", prompt)
	}
	if items.item.TitleSource != service.TitleUser || items.item.Title != "borrador" {
		t.Fatalf("got %+v", items.item)
	}
}

func TestGenerateStopsWhenTitleArrives(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", TitleSource: service.TitlePending}
	var items *fakeItems
	var g *Generator
	var cfg Config
	g, items, cfg = newGen(item, func(context.Context, string, string) error {
		items.item.Title, items.item.TitleSource = "Corto", service.TitleAI
		return nil
	}, time.Second)
	g.Generate(item, cfg)
	if items.item.TitleSource != service.TitleAI || items.item.Title != "Corto" {
		t.Fatalf("got %+v", items.item)
	}
}
