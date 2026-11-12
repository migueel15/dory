package localstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
)

func openTest(t *testing.T, opts Options) *Store {
	t.Helper()
	if opts.Device == "" {
		opts.Device = "dev-a"
	}
	s, err := Open(t.TempDir()+"/dory.db", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var _ service.Service = (*Store)(nil)

func TestEvents(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, Options{})
	if id, err := s.LatestEventID(ctx); err != nil || id != 0 {
		t.Fatalf("empty latest = %d, %v", id, err)
	}
	u, err := s.Users().Put(ctx, "cli", service.User{Username: "ana", PasswordHash: "pw-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Sessions().Put(ctx, u.ID, service.Session{UserID: u.ID, SecretHash: "sec-hash", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Projects().Put(ctx, u.ID, service.Project{Slug: "p", Name: "P"})
	for i := 0; i < 3; i++ {
		if _, err = s.Items().Put(ctx, u.ID, service.Item{RawText: "x", Title: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Projects().Archive(ctx, u.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEvent(ctx, "agent", "thread", p.ID, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListEvents(ctx, 0, "", 0)
	if err != nil || len(all) != 8 {
		t.Fatalf("events = %d, %v", len(all), err)
	}
	for i, e := range all {
		if e.ID != int64(i+1) {
			t.Fatalf("event %d has id %d", i, e.ID)
		}
		if strings.Contains(string(e.Change), "hash") {
			t.Fatalf("hash leaked: %s", e.Change)
		}
	}
	var ch struct {
		Operation string
		Record    map[string]any
	}
	json.Unmarshal(all[len(all)-2].Change, &ch)
	if ch.Operation != "archive" || ch.Record["archived_at"] == nil {
		t.Fatalf("archive change = %s", all[len(all)-2].Change)
	}
	if last, _ := s.LatestEventID(ctx); last != 8 {
		t.Fatalf("latest = %d", last)
	}
	page, _ := s.ListEvents(ctx, 2, "", 3)
	if len(page) != 3 || page[0].ID != 3 || page[2].ID != 5 {
		t.Fatalf("page = %+v", page)
	}
	items, _ := s.ListEvents(ctx, 0, "item", 0)
	if len(items) != 3 {
		t.Fatalf("item events = %d", len(items))
	}
	if _, err = s.ListEvents(ctx, -1, "", 0); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("negative after: %v", err)
	}
	var dev string
	var seq int64
	if err = s.root.QueryRow(`SELECT device, seq FROM events WHERE id = 8`).Scan(&dev, &seq); err != nil || dev != "dev-a" || seq != 8 {
		t.Fatalf("global id = %s/%d, %v", dev, seq, err)
	}
}

func TestEventRollsBackWithWrite(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, Options{})
	_ = s.InTx(ctx, func(tx service.Service) error {
		tx.Projects().Put(ctx, "a", service.Project{Slug: "x", Name: "X"})
		return errors.New("boom")
	})
	if id, _ := s.LatestEventID(ctx); id != 0 {
		t.Fatalf("event survived rollback: %d", id)
	}
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, Options{})
	p, _ := s.Projects().Put(ctx, "a", service.Project{Slug: "nube", Name: "Plataforma Nube"})
	it1, _ := s.Items().Put(ctx, "a", service.Item{RawText: "x", Title: "Configurar la sincronización", BodyMd: "usar el servidor de red"})
	it2, _ := s.Items().Put(ctx, "a", service.Item{RawText: "x", Title: "Sincronización de red lenta", BodyMd: "red red red"})
	f, _ := s.Features().Put(ctx, "a", service.Feature{ProjectID: p.ID, Title: "Búsqueda global", BodyMd: "texto completo"})
	m, _ := s.Memories().Put(ctx, "a", service.Memory{Scope: "global", Title: "Decisión", Content: "usar sqlite local"})

	ids := func(q string) []string {
		hits, err := s.Search(ctx, q, 0, 0)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		out := []string{}
		for _, h := range hits {
			out = append(out, h.EntityType+":"+h.EntityID)
		}
		return out
	}
	has := func(q string, want ...string) {
		t.Helper()
		got := ids(q)
		if len(got) != len(want) {
			t.Fatalf("%q = %v, want %v", q, got, want)
		}
		set := strings.Join(got, ",")
		for _, w := range want {
			if !strings.Contains(set, w) {
				t.Fatalf("%q = %v, want %v", q, got, want)
			}
		}
	}
	has("sincronizacion", "item:"+it1.ID, "item:"+it2.ID)      // accents folded
	has("BUSQUEDA", "feature:"+f.ID)                           // case and accents
	has("\"servidor de red\"", "item:"+it1.ID)                 // phrase
	has("\"red servidor\"")                                    // phrase order matters
	has("sincro", "item:"+it1.ID, "item:"+it2.ID)              // prefix
	has("sincronizacion -lenta", "item:"+it1.ID)               // exclusion
	has("sqlite or busqueda", "memory:"+m.ID, "feature:"+f.ID) // or
	has("nube", "project:"+p.ID)                               // projects
	has("   ")
	has("\"\" - \"")

	hits, _ := s.Search(ctx, "red", 0, 0)
	if len(hits) != 2 || hits[0].EntityID != it2.ID || hits[0].Rank <= hits[1].Rank || hits[1].Rank <= 0 {
		t.Fatalf("ranking = %+v", hits)
	}
	var item service.Item
	if err := json.Unmarshal(hits[0].Content, &item); err != nil || item.ID != it2.ID || item.Title == "" {
		t.Fatalf("content = %s", hits[0].Content)
	}
	if hits, _ = s.Search(ctx, "red", 1, 1); len(hits) != 1 || hits[0].EntityID != it1.ID {
		t.Fatalf("offset page = %+v", hits)
	}

	// Updates reindex; archives and project cascades unindex.
	it1.Title = "Otra cosa distinta"
	it1.BodyMd = ""
	s.Items().Put(ctx, "a", it1)
	has("servidor")
	has("distinta", "item:"+it1.ID)
	s.Items().Archive(ctx, "a", it2.ID)
	has("lenta")
	pi, _ := s.Items().Put(ctx, "a", service.Item{RawText: "x", Title: "hijo cascada", ProjectID: &p.ID})
	has("cascada", "item:"+pi.ID)
	s.Projects().Archive(ctx, "a", p.ID)
	has("cascada")
	has("nube")
	has("busqueda")
}

func TestAuthLookups(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := openTest(t, Options{Clock: func() time.Time { return now }})
	if _, err := s.FindUser(ctx, "nadie"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	u, err := s.Users().Put(ctx, "cli", service.User{Username: "ana", PasswordHash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Users().Put(ctx, "cli", service.User{Username: "ana"}); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("duplicate username: %v", err)
	}
	if got, err := s.FindUser(ctx, "ana"); err != nil || got.ID != u.ID || got.PasswordHash != "h" {
		t.Fatalf("FindUser = %+v, %v", got, err)
	}
	sess, err := s.Sessions().Put(ctx, u.ID, service.Session{UserID: u.ID, SecretHash: "s1", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Sessions().Put(ctx, u.ID, service.Session{UserID: u.ID, SecretHash: "s0", ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	exp := now.Add(-time.Minute)
	tok := service.Token{UserID: u.ID, Name: "n", SecretHash: "t1", Permissions: []string{"read"}}
	if _, err = s.Tokens().Put(ctx, u.ID, tok); err != nil {
		t.Fatal(err)
	}
	tok.SecretHash, tok.ExpiresAt = "t0", &exp
	if _, err = s.Tokens().Put(ctx, u.ID, tok); err != nil {
		t.Fatal(err)
	}
	if got, err := s.FindSession(ctx, "s1"); err != nil || got.ID != sess.ID {
		t.Fatalf("FindSession = %+v, %v", got, err)
	}
	if got, err := s.FindToken(ctx, "t1"); err != nil || got.UserID != u.ID || got.Permissions[0] != "read" {
		t.Fatalf("FindToken = %+v, %v", got, err)
	}
	for _, f := range []func() error{
		func() error { _, e := s.FindSession(ctx, "s0"); return e },
		func() error { _, e := s.FindToken(ctx, "t0"); return e },
		func() error { _, e := s.FindSession(ctx, "nope"); return e },
	} {
		if err := f(); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	}
	// Archived sessions and archived owners stop authenticating.
	s.Sessions().Archive(ctx, u.ID, sess.ID)
	if _, err = s.FindSession(ctx, "s1"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("archived session: %v", err)
	}
	s.Users().Archive(ctx, "cli", u.ID)
	if _, err = s.FindToken(ctx, "t1"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("archived owner: %v", err)
	}
	// Auth rows live apart from replicated records.
	var n int
	s.root.QueryRow(`SELECT count(*) FROM records WHERE kind IN ('user','session','token')`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d auth rows in records", n)
	}
}
