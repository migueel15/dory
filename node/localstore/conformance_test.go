package localstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/migrations"
	"github.com/pressly/goose/v3"
)

const actor = "test"

func TestLocalConformance(t *testing.T) {
	conformance(t, func(t *testing.T) service.Service {
		s, err := Open(t.TempDir()+"/dory.db", Options{Device: "dev-a"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}

// TestPostgresConformance runs the same suite against the Postgres service
// when DORY_TEST_DSN points at a database (migrations are applied to it).
func TestPostgresConformance(t *testing.T) {
	dsn := os.Getenv("DORY_TEST_DSN")
	if dsn == "" {
		t.Skip("DORY_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, stdlib.OpenDBFromPool(pool), "."); err != nil {
		t.Fatal(err)
	}
	conformance(t, func(*testing.T) service.Service { return service.New(pool) })
}

// conformance only relies on records it creates itself (unique names, filters
// by its own ids), so it also works on a database that already has data.
func conformance(t *testing.T, open func(*testing.T) service.Service) {
	ctx := context.Background()
	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	missing := "00000000-0000-4000-8000-000000000001"
	newProject := func(t *testing.T, s service.Service, name string) service.Project {
		t.Helper()
		p, err := s.Projects().Put(ctx, actor, service.Project{Slug: name + "-" + tag, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("project defaults and upsert", func(t *testing.T) {
		s := open(t)
		p := newProject(t, s, "defaults")
		if p.ID == "" || p.Status != "active" || p.Stack == nil || p.CreatedAt.IsZero() || p.ArchivedAt != nil {
			t.Fatalf("unexpected project %+v", p)
		}
		p.Name = "renamed"
		u, err := s.Projects().Put(ctx, actor, p)
		if err != nil {
			t.Fatal(err)
		}
		if u.ID != p.ID || u.Name != "renamed" || !u.CreatedAt.Equal(p.CreatedAt) || u.UpdatedAt.Before(p.UpdatedAt) {
			t.Fatalf("upsert changed identity: %+v vs %+v", u, p)
		}
		g, err := s.Projects().Get(ctx, p.ID)
		if err != nil || g.Name != "renamed" {
			t.Fatalf("get = %+v, %v", g, err)
		}
		if _, err = s.Projects().Put(ctx, "", service.Project{Slug: "x" + tag, Name: "x"}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("empty actor error = %v", err)
		}
	})

	t.Run("lookup errors", func(t *testing.T) {
		s := open(t)
		if _, err := s.Projects().Get(ctx, missing); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("missing = %v", err)
		}
		if _, err := s.Projects().Get(ctx, "nope"); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("bad id = %v", err)
		}
		if err := s.Projects().Archive(ctx, actor, missing); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("archive missing = %v", err)
		}
	})

	t.Run("unique slug and foreign keys", func(t *testing.T) {
		s := open(t)
		p := newProject(t, s, "unique")
		if _, err := s.Projects().Put(ctx, actor, service.Project{Slug: p.Slug, Name: "dup"}); err == nil {
			t.Fatal("duplicate slug accepted")
		}
		if _, err := s.Repositories().Put(ctx, actor, service.Repository{ProjectID: missing, RemoteUrl: "git@x:" + tag}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("dangling project = %v", err)
		}
		if _, err := s.Repositories().Put(ctx, actor, service.Repository{RemoteUrl: "git@x:" + tag}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("missing project = %v", err)
		}
		repo, err := s.Repositories().Put(ctx, actor, service.Repository{ProjectID: p.ID, RemoteUrl: "git@x:" + tag + "/ok"})
		if err != nil || repo.DefaultBranch != "main" {
			t.Fatalf("repo = %+v, %v", repo, err)
		}
		if _, err = s.Items().Put(ctx, actor, service.Item{Title: "t", ProjectID: &missing}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("dangling item project = %v", err)
		}
	})

	t.Run("archive hides and blocks writes", func(t *testing.T) {
		s := open(t)
		p := newProject(t, s, "archive")
		it, err := s.Items().Put(ctx, actor, service.Item{Title: "child", ProjectID: &p.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Projects().Archive(ctx, actor, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Projects().Get(ctx, p.ID); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("archived get = %v", err)
		}
		if _, err = s.Items().Get(ctx, it.ID); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("cascade get = %v", err)
		}
		f := service.Filter{Fields: map[string]any{"slug": p.Slug}}
		if rows, _ := s.Projects().List(ctx, f); len(rows) != 0 {
			t.Fatal("archived project listed")
		}
		f.IncludeArchived = true
		rows, _ := s.Projects().List(ctx, f)
		if len(rows) != 1 || rows[0].ArchivedAt == nil {
			t.Fatalf("include archived = %+v", rows)
		}
		p.Name = "again"
		if _, err = s.Projects().Put(ctx, actor, p); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("put archived = %v", err)
		}
		if err = s.Projects().Archive(ctx, actor, p.ID); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("double archive = %v", err)
		}
	})

	t.Run("items list filters and paging", func(t *testing.T) {
		s := open(t)
		p := newProject(t, s, "items")
		for i, st := range []string{"inbox", "ready", "done", "ready"} {
			it := service.Item{Title: fmt.Sprintf("i%d", i), ProjectID: &p.ID, Status: st, Tags: []string{"a", "b"}}
			if i == 3 {
				it.Tags = []string{"a"}
			}
			if i == 0 {
				it = service.Item{Title: "i0", ProjectID: &p.ID}
			}
			got, err := s.Items().Put(ctx, actor, it)
			if err != nil {
				t.Fatal(err)
			}
			if got.Tags == nil || got.Refs == nil || got.Source != "manual" || got.TitleSource != "user" {
				t.Fatalf("item defaults %+v", got)
			}
		}
		titles := func(f service.Filter) (out []string) {
			f.Fields = map[string]any{"project_id": p.ID}
			rows, err := s.Items().List(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				out = append(out, r.Title)
			}
			return
		}
		eq := func(got []string, want ...string) {
			t.Helper()
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("got %v want %v", got, want)
			}
		}
		eq(titles(service.Filter{}), "i0", "i1", "i2", "i3")
		eq(titles(service.Filter{Limit: 2, Offset: 1}), "i1", "i2")
		eq(titles(service.Filter{AnyOf: map[string][]string{"status": {"ready", "done"}}}), "i1", "i2", "i3")
		eq(titles(service.Filter{AnyOf: map[string][]string{"status": {"inbox"}}}), "i0")
		rows, err := s.Items().List(ctx, service.Filter{Fields: map[string]any{"project_id": p.ID, "tags": []string{"b"}, "status": "ready"}})
		if err != nil || len(rows) != 1 || rows[0].Title != "i1" {
			t.Fatalf("tag containment = %+v, %v", rows, err)
		}
		if _, err = s.Items().List(ctx, service.Filter{Limit: 1001}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("limit = %v", err)
		}
		if _, err = s.Items().Put(ctx, actor, service.Item{Title: "bad", Status: "nope"}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("bad status = %v", err)
		}
		if _, err = s.Items().Put(ctx, actor, service.Item{Title: "bad", Status: "blocked"}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("blocked without reason = %v", err)
		}
	})

	t.Run("credentials round trip", func(t *testing.T) {
		s := open(t)
		u, err := s.Users().Put(ctx, actor, service.User{Username: "u" + tag, PasswordHash: "hash"})
		if err != nil {
			t.Fatal(err)
		}
		g, err := s.Users().Get(ctx, u.ID)
		if err != nil || g.PasswordHash != "hash" {
			t.Fatalf("user = %+v, %v", g, err)
		}
		exp := time.Now().Add(time.Hour).UTC()
		if _, err = s.Sessions().Put(ctx, actor, service.Session{UserID: missing, SecretHash: "s" + tag, ExpiresAt: exp}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("session dangling user = %v", err)
		}
		tok, err := s.Tokens().Put(ctx, actor, service.Token{UserID: u.ID, Name: "n", SecretHash: "t" + tag, Permissions: []string{"read"}})
		if err != nil || tok.SecretHash != "t"+tag || tok.ExpiresAt != nil {
			t.Fatalf("token = %+v, %v", tok, err)
		}
		if _, err = s.Tokens().Put(ctx, actor, service.Token{UserID: u.ID, Name: "n", SecretHash: "t2" + tag, Permissions: []string{"root"}}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("bad permission = %v", err)
		}
	})

	t.Run("plans are versioned", func(t *testing.T) {
		s := open(t)
		it, err := s.Items().Put(ctx, actor, service.Item{Title: "planned"})
		if err != nil {
			t.Fatal(err)
		}
		p1, err := s.ItemPlans().Put(ctx, actor, service.ItemPlan{ItemID: it.ID, Sections: service.PlanSections{Goal: "g"}})
		if err != nil {
			t.Fatal(err)
		}
		p2, err := s.ItemPlans().Put(ctx, actor, service.ItemPlan{ItemID: it.ID, Sections: service.PlanSections{Goal: "g2"}})
		if err != nil || p1.Version != 1 || p2.Version != 2 || p1.Author != "agent" {
			t.Fatalf("versions = %d, %d, %v", p1.Version, p2.Version, err)
		}
		now := time.Now().UTC()
		p1.AcceptedAt, p1.Sections.Goal = &now, "changed"
		u, err := s.ItemPlans().Put(ctx, actor, p1)
		if err != nil || u.AcceptedAt == nil || u.Sections.Goal != "g" || u.Version != 1 {
			t.Fatalf("accept = %+v, %v", u, err)
		}
		latest, err := service.LatestPlan(ctx, s, it.ID)
		if err != nil || latest == nil || latest.Version != 2 {
			t.Fatalf("latest = %+v, %v", latest, err)
		}
	})

	t.Run("memory scope", func(t *testing.T) {
		s := open(t)
		if _, err := s.Memories().Put(ctx, actor, service.Memory{Scope: "global", Content: "c", ScopeID: &missing}); !errors.Is(err, service.ErrInvalid) {
			t.Fatalf("global with scope_id = %v", err)
		}
		m, err := s.Memories().Put(ctx, actor, service.Memory{Scope: "global", Content: "c"})
		if err != nil || m.Kind != "context" || m.Tags == nil {
			t.Fatalf("memory = %+v, %v", m, err)
		}
	})

	t.Run("transactions", func(t *testing.T) {
		s := open(t)
		boom := errors.New("boom")
		err := s.InTx(ctx, func(tx service.Service) error {
			if _, err := tx.Projects().Put(ctx, actor, service.Project{Slug: "rb-" + tag, Name: "R"}); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("InTx = %v", err)
		}
		if rows, _ := s.Projects().List(ctx, service.Filter{Fields: map[string]any{"slug": "rb-" + tag}}); len(rows) != 0 {
			t.Fatal("rolled back project survived")
		}
		// A failed inner write or nested InTx rolls back alone; the outer one still commits.
		err = s.InTx(ctx, func(tx service.Service) error {
			if _, err := tx.Projects().Put(ctx, actor, service.Project{Slug: "keep-" + tag, Name: "K"}); err != nil {
				return err
			}
			if _, err := tx.Repositories().Put(ctx, actor, service.Repository{ProjectID: missing, RemoteUrl: "git@x:bad" + tag}); err == nil {
				t.Error("dangling repository accepted")
			}
			if e := tx.InTx(ctx, func(in service.Service) error {
				if _, err := in.Projects().Put(ctx, actor, service.Project{Slug: "inner-" + tag, Name: "I"}); err != nil {
					return err
				}
				return boom
			}); !errors.Is(e, boom) {
				t.Errorf("nested = %v", e)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for slug, want := range map[string]int{"keep-" + tag: 1, "inner-" + tag: 0} {
			if rows, _ := s.Projects().List(ctx, service.Filter{Fields: map[string]any{"slug": slug}}); len(rows) != want {
				t.Fatalf("%s rows = %d, want %d", slug, len(rows), want)
			}
		}
	})
}
