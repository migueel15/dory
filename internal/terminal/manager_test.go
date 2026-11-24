package terminal

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// fake is a PTY that echoes what it is given, in upper case.
type fake struct {
	in     chan []byte
	closed chan struct{}
}

func newFake() *fake { return &fake{in: make(chan []byte, 16), closed: make(chan struct{})} }

func (f *fake) Read(b []byte) (int, error) {
	select {
	case p := <-f.in:
		return copy(b, p), nil
	case <-f.closed:
		return 0, io.EOF
	}
}
func (f *fake) Write(b []byte) (int, error) {
	f.in <- []byte(strings.ToUpper(strings.ReplaceAll(string(b), "\r", "\r\n")))
	return len(b), nil
}
func (f *fake) Resize(int, int) error { return nil }
func (f *fake) Wait() error           { <-f.closed; return nil }
func (f *fake) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

func manager() *Manager {
	return &Manager{Start: func(string, []string, int, int) (PTY, error) { return newFake(), nil }}
}

func TestOpenNamesAndClose(t *testing.T) {
	m := manager()
	ctx := context.Background()
	a, err := m.Open(ctx, "t1", "", 80, 24)
	if err != nil || a.Name != "Terminal 1" {
		t.Fatalf("first: %+v %v", a, err)
	}
	b, _ := m.Open(ctx, "t1", "", 80, 24)
	if b.Name != "Terminal 2" {
		t.Fatalf("second: %+v", b)
	}
	if _, err := m.Open(ctx, "t1", "Terminal 1", 80, 24); err == nil {
		t.Fatal("a repeated name must fail")
	}
	if _, err := m.Rename("t1", b.ID, "Terminal 1"); err == nil {
		t.Fatal("renaming to a name in use must fail")
	}
	if _, err := m.Rename("t1", "Terminal 2", "build"); err != nil {
		t.Fatal(err)
	}
	if s, err := m.Get("t1", "build"); err != nil || s.id != b.ID {
		t.Fatalf("by name: %v", err)
	}
	if _, err := m.Get("t2", a.ID); err == nil {
		t.Fatal("another thread must not reach it")
	}
	if len(m.List("t1")) != 2 {
		t.Fatal("list")
	}
	if _, err := m.Close("t1", a.ID); err != nil || len(m.List("t1")) != 1 {
		t.Fatalf("close: %v", err)
	}
	m.CloseAll()
	if len(m.List("t1")) != 0 {
		t.Fatal("close all")
	}
}

func TestBufferAndRun(t *testing.T) {
	m := manager()
	info, _ := m.Open(context.Background(), "t1", "", 80, 24)
	s, _ := m.Get("t1", info.ID)
	out, err := s.Run("echo hi", 3*time.Second)
	if err != nil || strings.TrimSpace(out) != "ECHO HI" {
		t.Fatalf("run: %q %v", out, err)
	}
	// A late client gets what was printed before it came.
	first, c, cancel := s.Subscribe()
	defer cancel()
	if !strings.Contains(string(first), "ECHO HI") {
		t.Fatalf("buffer: %q", first)
	}
	_ = s.Write([]byte("more"))
	select {
	case got := <-c:
		if string(got) != "MORE" {
			t.Fatalf("live: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no live output")
	}
}

func TestBufferKeepsTheTail(t *testing.T) {
	s := &Session{subs: map[chan []byte]struct{}{}}
	big := make([]byte, bufferSize)
	for i := range big {
		big[i] = 'a'
	}
	s.append(big)
	s.append([]byte("xyz"))
	got := s.snapshot()
	if len(got) != bufferSize || string(got[len(got)-3:]) != "xyz" || got[0] != 'a' {
		t.Fatalf("snapshot %d %q", len(got), got[len(got)-3:])
	}
	if string(s.Since(s.total-3)) != "xyz" {
		t.Fatal("since")
	}
}

func TestPlainText(t *testing.T) {
	got := PlainText([]byte("\x1b[32mok\x1b[0m\r\nload\rdone\r\n\x1b]0;title\x07$ "))
	if got != "ok\ndone\n$" {
		t.Fatalf("%q", got)
	}
}
