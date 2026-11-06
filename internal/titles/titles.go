// Package titles generates the title of an idea with a fast model in a T3 thread.
// What the user writes stays as the description; the thread only sets the title.
package titles

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

const actor = "dory-titles"

// pollEvery is how often a running generation checks whether the title arrived.
var pollEvery = 2 * time.Second

// Config selects the model and limits the generation. An empty Model turns it off.
type Config struct {
	Instance string
	Provider string
	Model    string
	Effort   string
	Project  string
	Timeout  time.Duration
}

// Generator starts title threads for pending items.
type Generator struct {
	svc    service.Service
	load   func(ctx context.Context) (Config, error)
	launch func(ctx context.Context, cfg Config, title, prompt string) error
}

// New builds a Generator that reads the stored settings on every generation and
// launches threads through the connected T3 instances.
func New(svc service.Service, monitor *t3.Monitor, store Store) *Generator {
	return &Generator{
		svc: svc,
		load: func(ctx context.Context) (Config, error) {
			s, err := store.Load(ctx)
			return s.Config(), err
		},
		launch: func(ctx context.Context, cfg Config, title, prompt string) error {
			return launchOnT3(ctx, monitor, cfg, title, prompt)
		},
	}
}

// config returns the current settings; unreadable settings mean off.
func (g *Generator) config(ctx context.Context) Config {
	cfg, err := g.load(ctx)
	if err != nil {
		log.Printf("titles: settings: %v", err)
		return Config{}
	}
	return cfg
}

// Prompt is the fixed instruction: the item id, a single title change, archiving the
// agent's own thread with the T3 Code tools and the full description.
func Prompt(item service.Item) string {
	return fmt.Sprintf("Genera un título breve (≤ 80 caracteres, en el idioma del texto) para el item de Dory `%s`. "+
		"Llama una sola vez a `update_item` con `{title, title_source: \"ai\"}`. "+
		"Después archiva tu propio hilo con la herramienta de T3 Code que lo permita (por ejemplo `t3_thread_update`; "+
		"son herramientas diferidas: cárgalas antes con ToolSearch) y termina sin hacer nada más. "+
		"Si no puedes generar el título, archiva igualmente tu hilo antes de acabar. Descripción:\n%s", item.ID, item.RawText)
}

// Generate runs in the background: it starts the thread and waits until the title
// is no longer pending. On a failure or a timeout the provisional title stays as the user's.
func (g *Generator) Generate(item service.Item, cfg Config) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := g.launch(ctx, cfg, "Título: "+item.Title, Prompt(item)); err != nil {
		log.Printf("titles: item %s: %v", item.ID, err)
		g.giveUp(item.ID)
		return
	}
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("titles: item %s: timeout", item.ID)
			g.giveUp(item.ID)
			return
		case <-tick.C:
			cur, err := g.svc.Items().Get(ctx, item.ID)
			if errors.Is(err, service.ErrNotFound) || (err == nil && cur.TitleSource != service.TitlePending) {
				return
			}
		}
	}
}

// giveUp hands a still-pending title to the user, keeping the provisional text.
func (g *Generator) giveUp(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := g.svc.Items().Get(ctx, id)
	if err != nil || cur.TitleSource != service.TitlePending {
		return
	}
	cur.TitleSource = service.TitleUser
	if _, err = g.svc.Items().Put(ctx, actor, cur); err != nil {
		log.Printf("titles: item %s: %v", id, err)
	}
}

// Recover hands to the user every title left pending by a previous process.
func (g *Generator) Recover(ctx context.Context) {
	pending, err := g.svc.Items().List(ctx, service.Filter{Fields: map[string]any{"title_source": service.TitlePending}, Limit: 1000})
	if err != nil {
		log.Printf("titles: recover: %v", err)
		return
	}
	for _, item := range pending {
		g.giveUp(item.ID)
	}
}

// Wrap makes item creation start the generation for pending titles. When the
// generator is off, a pending title is stored as the user's.
func (g *Generator) Wrap(svc service.Service) service.Service {
	return generating{Service: svc, g: g}
}

type generating struct {
	service.Service
	g *Generator
}

func (s generating) Items() service.Resource[service.Item] {
	return generatingItems{Resource: s.Service.Items(), g: s.g}
}

type generatingItems struct {
	service.Resource[service.Item]
	g *Generator
}

func (r generatingItems) Put(ctx context.Context, by string, in service.Item) (service.Item, error) {
	created := true
	if in.ID != "" {
		if _, err := r.Get(ctx, in.ID); err == nil {
			created = false
		}
	}
	pending := created && in.TitleSource == service.TitlePending
	var cfg Config
	if pending {
		if cfg = r.g.config(ctx); cfg.Model == "" {
			in.TitleSource = service.TitleUser
			pending = false
		}
	}
	out, err := r.Resource.Put(ctx, by, in)
	if err == nil && pending && out.TitleSource == service.TitlePending {
		go r.g.Generate(out, cfg)
	}
	return out, err
}

// launchOnT3 starts the thread on the chosen instance, or on the first connected
// one that offers the model when none is chosen.
func launchOnT3(ctx context.Context, monitor *t3.Monitor, cfg Config, title, prompt string) error {
	var last error = t3.ErrInstanceUnavailable
	for _, st := range monitor.Statuses() {
		if st.State != t3.StateAvailable || (cfg.Instance != "" && st.InstanceID != cfg.Instance) {
			continue
		}
		c, err := monitor.InstanceClient(ctx, st.InstanceID)
		if err != nil {
			last = err
			continue
		}
		cat, err := c.Catalog(ctx)
		if err != nil {
			last = err
			continue
		}
		in, ok := pick(cat, cfg)
		if !ok {
			last = fmt.Errorf("model %q is not available on the instance %s", cfg.Model, st.InstanceID)
			continue
		}
		in.Title, in.Prompt = title, prompt
		_, err = c.Launch(ctx, in)
		return err
	}
	return last
}

// pick chooses provider, model, T3 project and the lowest reasoning effort from a catalog.
func pick(cat t3.Catalog, cfg Config) (t3.LaunchInput, bool) {
	var in t3.LaunchInput
	if len(cat.Projects) == 0 {
		return in, false
	}
	in.ProjectID = cat.Projects[0].ID
	for _, p := range cat.Projects {
		if cfg.Project != "" && (p.ID == cfg.Project || strings.EqualFold(p.Title, cfg.Project)) {
			in.ProjectID = p.ID
		}
	}
	for _, p := range cat.Providers {
		if cfg.Provider != "" && p.InstanceID != cfg.Provider {
			continue
		}
		for _, m := range p.Models {
			if m.Slug != cfg.Model {
				continue
			}
			in.Provider, in.Model = p.InstanceID, m.Slug
			if m.Effort != nil && len(m.Effort.Values) > 0 {
				in.EffortOption, in.Effort = m.Effort.ID, m.Effort.Values[0]
				for _, v := range m.Effort.Values {
					if v == cfg.Effort {
						in.Effort = v
					}
				}
			}
			return in, true
		}
	}
	return in, false
}
