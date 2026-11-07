package dictation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeEngine returns the WAV bytes as text, optionally failing on one batch.
type fakeEngine struct {
	mu    sync.Mutex
	order []string
	fail  string
	delay time.Duration
}

func (f *fakeEngine) Transcribe(_ context.Context, wav []byte) (string, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order = append(f.order, string(wav))
	if string(wav) == f.fail {
		return "", errors.New("boom")
	}
	return string(wav), nil
}

func TestOrderAndFinishInsert(t *testing.T) {
	eng := &fakeEngine{delay: 5 * time.Millisecond}
	m := New(eng)
	st, err := m.Create("u")
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range []string{"hola qué", "qué tal", "estás"} {
		if _, err := m.Add("u", st.ID, i, []byte(w)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Add("u", st.ID, 5, []byte("x")); !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("want out of order, got %v", err)
	}
	if _, err := m.Add("u", st.ID, 1, []byte("retry")); err != nil {
		t.Fatalf("retried batch should be ignored, got %v", err)
	}
	if _, err := m.Get("other", st.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another user must not see the session")
	}
	out, err := m.Finish(context.Background(), "u", st.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "hola qué tal estás" || out.Pending != 0 || out.Status != "done" {
		t.Fatalf("unexpected state %+v", out)
	}
	if len(eng.order) != 3 || eng.order[2] != "estás" {
		t.Fatalf("batches out of order: %v", eng.order)
	}
	if _, err := m.Add("u", st.ID, 3, []byte("x")); !errors.Is(err, ErrFinished) {
		t.Fatalf("want finished, got %v", err)
	}
}

func TestFinishSendDelivers(t *testing.T) {
	m := New(&fakeEngine{})
	st, _ := m.Create("u")
	m.Add("u", st.ID, 0, []byte("crea una tarea"))
	got := make(chan string, 1)
	out, err := m.Finish(context.Background(), "u", st.ID, func(_ context.Context, text string) error { got <- text; return nil })
	if err != nil || out.Status != "finishing" {
		t.Fatalf("send should return at once: %+v %v", out, err)
	}
	select {
	case text := <-got:
		if text != "crea una tarea" {
			t.Fatalf("delivered %q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("not delivered")
	}
}

func TestEngineErrorFailsSession(t *testing.T) {
	m := New(&fakeEngine{fail: "b"})
	st, _ := m.Create("u")
	m.Add("u", st.ID, 0, []byte("a"))
	m.Add("u", st.ID, 1, []byte("b"))
	m.Add("u", st.ID, 2, []byte("c"))
	out, err := m.Finish(context.Background(), "u", st.ID, nil)
	if err == nil || out.Status != "failed" || out.Text != "a" {
		t.Fatalf("want failure keeping partial text, got %+v %v", out, err)
	}
	delivered := false
	st, _ = m.Create("u")
	m.Add("u", st.ID, 0, []byte("b"))
	m.Finish(context.Background(), "u", st.ID, func(context.Context, string) error { delivered = true; return nil })
	time.Sleep(50 * time.Millisecond)
	if delivered {
		t.Fatal("a failed session must not be delivered")
	}
}

func TestExpiry(t *testing.T) {
	m := New(&fakeEngine{})
	now := time.Now()
	m.now = func() time.Time { return now }
	st, _ := m.Create("u")
	now = now.Add(TTL + time.Second)
	if _, err := m.Get("u", st.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestUnconfigured(t *testing.T) {
	if _, err := New(nil).Create("u"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestMerge(t *testing.T) {
	for _, c := range [][3]string{
		{"", "Hola.", "Hola."},
		{"vamos a la playa", "Playa, y luego", "vamos a la playa y luego"},
		{"uno dos", "tres", "uno dos tres"},
		{"uno dos", "", "uno dos"},
	} {
		if got := Merge(c[0], c[1]); got != c[2] {
			t.Errorf("Merge(%q,%q)=%q want %q", c[0], c[1], got, c[2])
		}
	}
}
