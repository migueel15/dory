package dictation

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeEngine returns the WAV bytes as text, optionally failing on one batch.
type fakeEngine struct {
	mu      sync.Mutex
	order   []string
	prompts []string
	fail    string
	delay   time.Duration
}

func (f *fakeEngine) Transcribe(_ context.Context, wav []byte, prompt string) (string, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order = append(f.order, string(wav))
	f.prompts = append(f.prompts, prompt)
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
	if eng.prompts[0] != "" || eng.prompts[1] != "hola qué" || eng.prompts[2] != "hola qué tal" {
		t.Fatalf("prompts should be the previous text: %q", eng.prompts)
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

// pcmWAV builds a 16 kHz mono 16-bit WAV of n samples at amplitude amp.
func pcmWAV(n int, amp int16) []byte {
	b := make([]byte, 44+2*n)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*n))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*n))
	for i := 0; i < n; i++ {
		v := amp
		if i%2 == 1 {
			v = -amp
		}
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(v))
	}
	return b
}

func TestSilentBatchSkipped(t *testing.T) {
	eng := &fakeEngine{}
	m := New(eng)
	st, _ := m.Create("u")
	out, err := m.Add("u", st.ID, 0, pcmWAV(16000, 50))
	if err != nil || out.Pending != 0 {
		t.Fatalf("silent batch should not queue: %+v %v", out, err)
	}
	if _, err := m.Add("u", st.ID, 1, []byte("hola")); err != nil {
		t.Fatalf("next seq should be accepted: %v", err)
	}
	if !silent(pcmWAV(1600, 0)) || silent(pcmWAV(16000, 3000)) {
		t.Fatal("silence detection wrong")
	}
	out, _ = m.Finish(context.Background(), "u", st.ID, nil)
	if out.Text != "hola" || len(eng.order) != 1 {
		t.Fatalf("engine must only see the voiced batch: %+v %v", out, eng.order)
	}
}
