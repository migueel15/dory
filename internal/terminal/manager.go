package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/apperr"
)

// Manager keeps the sessions of the threads of this node, in memory: they
// last until their tab closes or Dory stops.
type Manager struct {
	// Cwd says where the shell of a thread starts: its workspace.
	Cwd func(ctx context.Context, threadID string) string
	// Changed tells that the sessions of a thread changed.
	Changed func(threadID string)
	// Start runs a shell; nil uses the system's. Tests replace it.
	Start func(cwd string, env []string, cols, rows int) (PTY, error)

	mu       sync.Mutex
	sessions map[string]*Session
	order    []string
}

func (m *Manager) changed(thread string) {
	if m.Changed != nil {
		m.Changed(thread)
	}
}

// Open starts a shell for a thread. An empty name gets "Terminal N".
func (m *Manager) Open(ctx context.Context, threadID, name string, cols, rows int) (Info, error) {
	cwd := ""
	if m.Cwd != nil {
		cwd = m.Cwd(ctx, threadID)
	}
	if st, err := os.Stat(cwd); cwd == "" || err != nil || !st.IsDir() {
		cwd, _ = os.UserHomeDir()
	}
	if cols < 1 || rows < 1 {
		cols, rows = 80, 24
	}
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "" {
		name = m.freeName(threadID)
	} else if m.byName(threadID, name) != nil {
		return Info{}, apperr.New(apperr.Conflict, "the thread already has a terminal with that name").With("name", name)
	}
	env := append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	start := m.Start
	if start == nil {
		start = start0
	}
	p, err := start(cwd, env, cols, rows)
	if err != nil {
		return Info{}, apperr.Wrap(apperr.Unavailable, err, "the terminal could not start")
	}
	var raw [6]byte
	_, _ = rand.Read(raw[:])
	s := newSession(hex.EncodeToString(raw[:]), threadID, name, cwd, p)
	if m.sessions == nil {
		m.sessions = map[string]*Session{}
	}
	m.sessions[s.id] = s
	m.order = append(m.order, s.id)
	go func() {
		<-s.done
		m.changed(threadID)
	}()
	go m.changed(threadID)
	return s.Info(), nil
}

var start0 = start

func (m *Manager) freeName(thread string) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("Terminal %d", n)
		if m.byName(thread, name) == nil {
			return name
		}
	}
}

func (m *Manager) byName(thread, name string) *Session {
	for _, id := range m.order {
		if s := m.sessions[id]; s.thread == thread && s.Info().Name == name {
			return s
		}
	}
	return nil
}

// List returns the sessions of a thread in the order they opened.
func (m *Manager) List(threadID string) []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Info{}
	for _, id := range m.order {
		if s := m.sessions[id]; s.thread == threadID {
			out = append(out, s.Info())
		}
	}
	return out
}

// Get finds a session of a thread by id or, failing that, by name.
func (m *Manager) Get(threadID, ref string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[ref]; s != nil && s.thread == threadID {
		return s, nil
	}
	if s := m.byName(threadID, ref); s != nil {
		return s, nil
	}
	return nil, apperr.New(apperr.NotFound, "there is no terminal with that id or name").With("terminal", ref)
}

// Rename gives a session another name, unique within its thread.
func (m *Manager) Rename(threadID, ref, name string) (Info, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Info{}, apperr.New(apperr.Invalid, "the name is empty")
	}
	s, err := m.Get(threadID, ref)
	if err != nil {
		return Info{}, err
	}
	m.mu.Lock()
	if other := m.byName(threadID, name); other != nil && other != s {
		m.mu.Unlock()
		return Info{}, apperr.New(apperr.Conflict, "the thread already has a terminal with that name").With("name", name)
	}
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
	m.mu.Unlock()
	m.changed(threadID)
	return s.Info(), nil
}

// Close kills a session's shell and forgets it.
func (m *Manager) Close(threadID, ref string) (Info, error) {
	s, err := m.Get(threadID, ref)
	if err != nil {
		return Info{}, err
	}
	m.mu.Lock()
	delete(m.sessions, s.id)
	for i, id := range m.order {
		if id == s.id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	info := s.Info()
	_ = s.pty.Close()
	m.changed(threadID)
	return info, nil
}

// CloseAll kills every shell, when Dory stops.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.sessions, m.order = nil, nil
	m.mu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].id < all[j].id })
	for _, s := range all {
		_ = s.pty.Close()
	}
}
