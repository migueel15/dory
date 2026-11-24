//go:build !windows

package terminal

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRealShell(t *testing.T) {
	m := &Manager{}
	dir := t.TempDir()
	m.Cwd = func(context.Context, string) string { return dir }
	info, err := m.Open(context.Background(), "t", "", 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer m.CloseAll()
	s, _ := m.Get("t", info.ID)
	out, _ := s.Run("pwd", 5*time.Second)
	if !strings.Contains(out, dir) {
		t.Fatalf("pwd: %q want %s", out, dir)
	}
	if _, err := m.Close("t", info.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("shell survived close")
	}
}
