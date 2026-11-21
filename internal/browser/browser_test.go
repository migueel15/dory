package browser

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/localdb"
)

func TestHeadlessTab(t *testing.T) {
	if _, err := findChromium(); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	db, err := localdb.Open(ctx, filepath.Join(dir, "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db, Profile: filepath.Join(dir, "profile")}
	t.Cleanup(func() {
		m.reap()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.chrome != nil {
			m.chrome.stop()
		}
	})
	page := `data:text/html,<title>Hola</title><button onclick="document.title='ok'">Pulsa</button><a href="about:blank" target="_blank">nueva</a>`
	if _, err := m.Open(ctx, "th", page, false); err != nil {
		t.Fatal(err)
	}
	png, err := m.Screenshot(ctx, "th", "", false)
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("screenshot: %v", err)
	}
	r, err := m.Read(ctx, "th", "")
	if err != nil || len(r.Elements) != 2 {
		t.Fatalf("read: %+v %v", r, err)
	}
	if err := m.Click(ctx, "th", "", 0, 0, "button", false); err != nil {
		t.Fatal(err)
	}
	if r, _ = m.Read(ctx, "th", ""); r.Title != "ok" {
		t.Fatalf("title after click = %q", r.Title)
	}
	if err := m.Click(ctx, "th", "", 0, 0, "a", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	tabs, _ := m.Tabs(ctx, "th")
	if len(tabs) != 2 {
		t.Fatalf("target=_blank should open a tab, got %d", len(tabs))
	}
	m.SetUserControl("th", true)
	if err := m.Key(ctx, "th", "", "Enter"); err != ErrUserControl {
		t.Fatalf("user control: %v", err)
	}
}
