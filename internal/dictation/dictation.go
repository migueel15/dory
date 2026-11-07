// Package dictation keeps short-lived dictation sessions in memory. Clients
// upload audio in numbered batches; one worker per session transcribes them in
// order through a local engine and accumulates the text. No audio is stored.
package dictation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"
)

// TTL is how long a session lives after it was created.
const TTL = 30 * time.Minute

// MaxPending bounds the queue of a session so a slow engine cannot fill memory.
const MaxPending = 32

var (
	ErrNotFound    = errors.New("dictation session not found")
	ErrFinished    = errors.New("dictation session already finished")
	ErrOutOfOrder  = errors.New("chunk seq out of order")
	ErrQueueFull   = errors.New("too many pending chunks")
	ErrUnavailable = errors.New("dictation engine not configured")
)

// Engine transcribes one self-contained WAV batch.
type Engine interface {
	Transcribe(ctx context.Context, wav []byte) (string, error)
}

// Deliver sends the final text somewhere once a session in send mode ends.
type Deliver func(ctx context.Context, text string) error

// State is what clients see of a session.
type State struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Pending int    `json:"pending"`
	// Status is recording, finishing, done or failed.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type chunk struct {
	seq int
	wav []byte
}

type session struct {
	id      string
	owner   string
	created time.Time
	mu      sync.Mutex
	next    int // next seq expected
	queue   chan chunk
	pending int
	text    string
	status  string
	err     string
	closed  bool
	done    chan struct{} // closed when the worker drained the queue
}

// Manager owns all sessions.
type Manager struct {
	engine Engine
	now    func() time.Time
	mu     sync.Mutex
	byID   map[string]*session
}

// New returns a manager; a nil engine makes every call fail with ErrUnavailable.
func New(engine Engine) *Manager {
	return &Manager{engine: engine, now: time.Now, byID: map[string]*session{}}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create starts a session owned by owner.
func (m *Manager) Create(owner string) (State, error) {
	if m.engine == nil {
		return State{}, ErrUnavailable
	}
	s := &session{id: newID(), owner: owner, created: m.now(), queue: make(chan chunk, MaxPending), status: "recording", done: make(chan struct{})}
	m.mu.Lock()
	m.sweep()
	m.byID[s.id] = s
	m.mu.Unlock()
	go m.work(s)
	return s.state(), nil
}

// sweep drops expired sessions; callers hold m.mu.
func (m *Manager) sweep() {
	for id, s := range m.byID {
		if m.now().Sub(s.created) > TTL {
			s.close()
			delete(m.byID, id)
		}
	}
}

func (m *Manager) get(owner, id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweep()
	s, ok := m.byID[id]
	if !ok || s.owner != owner {
		return nil, ErrNotFound
	}
	return s, nil
}

// Add queues batch seq; batches must arrive in order starting at 0.
func (m *Manager) Add(owner, id string, seq int, wav []byte) (State, error) {
	s, err := m.get(owner, id)
	if err != nil {
		return State{}, err
	}
	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		return State{}, ErrFinished
	case seq < s.next:
		// A retried batch that already arrived is accepted silently.
		s.mu.Unlock()
		return s.state(), nil
	case seq > s.next:
		s.mu.Unlock()
		return State{}, ErrOutOfOrder
	case s.pending >= MaxPending:
		s.mu.Unlock()
		return State{}, ErrQueueFull
	}
	s.next++
	s.pending++
	s.queue <- chunk{seq: seq, wav: wav}
	s.mu.Unlock()
	return s.state(), nil
}

// Get returns the partial text and the pending batches.
func (m *Manager) Get(owner, id string) (State, error) {
	s, err := m.get(owner, id)
	if err != nil {
		return State{}, err
	}
	return s.state(), nil
}

// Finish closes the session. Without deliver it waits for the last batch and
// returns the text. With deliver it returns at once and delivers in the
// background once the queue is drained.
func (m *Manager) Finish(ctx context.Context, owner, id string, deliver Deliver) (State, error) {
	s, err := m.get(owner, id)
	if err != nil {
		return State{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return State{}, ErrFinished
	}
	s.status = "finishing"
	s.mu.Unlock()
	s.close()
	if deliver != nil {
		go func() {
			<-s.done
			st := s.state()
			if st.Status == "failed" {
				log.Printf("dictation: %s not delivered: %s", id, st.Error)
				return
			}
			dctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if strings.TrimSpace(st.Text) == "" {
				s.set("done", "")
				return
			}
			if err := deliver(dctx, st.Text); err != nil {
				log.Printf("dictation: deliver %s: %v", id, err)
				s.set("failed", err.Error())
				return
			}
			s.set("done", "")
		}()
		return s.state(), nil
	}
	select {
	case <-s.done:
	case <-ctx.Done():
		return State{}, ctx.Err()
	}
	s.mu.Lock()
	if s.status == "finishing" {
		s.status = "done"
	}
	s.mu.Unlock()
	st := s.state()
	if st.Status == "failed" {
		return st, errors.New(st.Error)
	}
	return st, nil
}

func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
}

func (s *session) set(status, msg string) {
	s.mu.Lock()
	s.status, s.err = status, msg
	s.mu.Unlock()
}

func (s *session) state() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return State{ID: s.id, Text: s.text, Pending: s.pending, Status: s.status, Error: s.err}
}

// work transcribes the queue in order. After an engine error the remaining
// batches are dropped and the session is marked failed.
func (m *Manager) work(s *session) {
	defer close(s.done)
	for c := range s.queue {
		s.mu.Lock()
		failed := s.status == "failed"
		s.mu.Unlock()
		var text string
		var err error
		if !failed {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			text, err = m.engine.Transcribe(ctx, c.wav)
			cancel()
		}
		s.mu.Lock()
		s.pending--
		if err != nil {
			s.status, s.err = "failed", "transcripción fallida: "+err.Error()
		} else if !failed {
			s.text = Merge(s.text, text)
		}
		s.mu.Unlock()
	}
}

// Merge appends next to prev, dropping the words repeated by the overlap
// between batches: the longest suffix of prev (up to 8 words) that matches the
// start of next.
func Merge(prev, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return prev
	}
	if prev == "" {
		return next
	}
	pw, nw := strings.Fields(prev), strings.Fields(next)
	for k := min(8, len(pw), len(nw)); k > 0; k-- {
		match := true
		for i := 0; i < k; i++ {
			if norm(pw[len(pw)-k+i]) != norm(nw[i]) {
				match = false
				break
			}
		}
		if match {
			nw = nw[k:]
			break
		}
	}
	if len(nw) == 0 {
		return prev
	}
	return prev + " " + strings.Join(nw, " ")
}

func norm(w string) string {
	return strings.ToLower(strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}
