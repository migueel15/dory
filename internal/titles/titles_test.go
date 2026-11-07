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
	for _, want := range []string{"`abc`", "update_item", `title_source: "ai"`, "No archives", "arreglar el login\ncon detalle"} {
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
		launch: func(ctx context.Context, _ Config, title, prompt string) (func(), error) {
			return nil, launch(ctx, title, prompt)
		}}, items, cfg
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

func TestGenerateFailureLeavesFallbackTitle(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", TitleSource: service.TitlePending}
	g, items, cfg := newGen(item, func(context.Context, string, string) error { return errors.New("t3 caído") }, time.Second)
	g.Generate(item, cfg)
	if items.item.TitleSource != service.TitleFallback || items.item.Title != "borrador" {
		t.Fatalf("got %+v", items.item)
	}
}

func TestGenerateTimeoutLeavesFallbackTitle(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", TitleSource: service.TitlePending}
	var prompt string
	g, items, cfg := newGen(item, func(_ context.Context, _, p string) error { prompt = p; return nil }, 40*time.Millisecond)
	g.Generate(item, cfg)
	if prompt != Prompt(item) {
		t.Fatalf("unexpected prompt %q", prompt)
	}
	if items.item.TitleSource != service.TitleFallback || items.item.Title != "borrador" {
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

func TestRegenerate(t *testing.T) {
	item := service.Item{Record: service.Record{ID: "1"}, Title: "borrador", RawText: "texto real", TitleSource: service.TitleFallback}
	var prompt string
	launched := make(chan struct{})
	g, items, _ := newGen(item, func(_ context.Context, _, p string) error { prompt = p; close(launched); return nil }, 40*time.Millisecond)
	out, err := g.Regenerate(context.Background(), "u", "1")
	if err != nil || out.TitleSource != service.TitlePending {
		t.Fatalf("got %+v, %v", out, err)
	}
	<-launched
	if !strings.Contains(prompt, "texto real") {
		t.Fatalf("prompt lacks the description: %q", prompt)
	}
	// The generation times out and the item returns to fallback.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if items.item.TitleSource == service.TitleFallback {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("item did not return to fallback: %+v", items.item)
}

func TestRegenerateRejectsOtherStates(t *testing.T) {
	g, _, _ := newGen(service.Item{Record: service.Record{ID: "1"}, TitleSource: service.TitleUser}, nil, time.Second)
	if _, err := g.Regenerate(context.Background(), "u", "1"); !errors.Is(err, ErrNotFallback) {
		t.Fatalf("got %v", err)
	}
}

func TestRegenerateOff(t *testing.T) {
	g, items, _ := newGen(service.Item{Record: service.Record{ID: "1"}, TitleSource: service.TitleFallback}, nil, time.Second)
	g.load = func(context.Context) (Config, error) { return Config{}, nil }
	if _, err := g.Regenerate(context.Background(), "u", "1"); !errors.Is(err, ErrOff) {
		t.Fatalf("got %v", err)
	}
	if items.item.TitleSource != service.TitleFallback {
		t.Fatalf("an off generator must not touch the item: %+v", items.item)
	}
}
