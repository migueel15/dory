package terminal

import (
	"sync"
	"time"
)

// bufferSize is how much recent output a session keeps for reconnecting
// clients and for the agent.
const bufferSize = 256 << 10

// Info is what the interface and the agents see of a session.
type Info struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Running bool   `json:"running"`
}

// Session is one shell with the last of its output.
type Session struct {
	id, thread, cwd string
	pty             PTY
	done            chan struct{}

	mu    sync.Mutex
	name  string
	buf   []byte // circular once full
	start int    // index of the oldest byte
	total uint64 // bytes ever written to buf
	subs  map[chan []byte]struct{}
}

func newSession(id, thread, name, cwd string, p PTY) *Session {
	s := &Session{id: id, thread: thread, name: name, cwd: cwd, pty: p, done: make(chan struct{}), subs: map[chan []byte]struct{}{}}
	go s.pump()
	return s
}

// pump copies the shell's output to the buffer and the subscribers until it exits.
func (s *Session) pump() {
	chunk := make([]byte, 32<<10)
	for {
		n, err := s.pty.Read(chunk)
		if n > 0 {
			s.append(chunk[:n])
		}
		if err != nil {
			break
		}
	}
	_ = s.pty.Wait()
	s.mu.Lock()
	close(s.done)
	for c := range s.subs {
		close(c)
		delete(s.subs, c)
	}
	s.mu.Unlock()
}

func (s *Session) append(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += uint64(len(p))
	if len(p) >= bufferSize {
		s.buf = append(s.buf[:0], p[len(p)-bufferSize:]...)
		s.start = 0
	} else if len(s.buf) < bufferSize {
		s.buf = append(s.buf, p...)
		if over := len(s.buf) - bufferSize; over > 0 {
			s.buf = s.buf[over:]
		}
	} else {
		for _, b := range p {
			s.buf[s.start] = b
			s.start = (s.start + 1) % bufferSize
		}
	}
	for c := range s.subs {
		cp := append([]byte(nil), p...)
		select {
		case c <- cp:
		default:
			// A client that cannot keep up is dropped; it reconnects and gets the buffer.
			close(c)
			delete(s.subs, c)
		}
	}
}

// snapshot returns the buffered output in order. Callers hold mu.
func (s *Session) snapshot() []byte {
	out := make([]byte, 0, len(s.buf))
	out = append(out, s.buf[s.start:]...)
	return append(out, s.buf[:s.start]...)
}

// Subscribe returns the output so far and a channel with what follows; the
// channel closes when the shell exits or the subscriber falls behind.
func (s *Session) Subscribe() ([]byte, <-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := make(chan []byte, 256)
	first := s.snapshot()
	select {
	case <-s.done:
		close(c)
	default:
		s.subs[c] = struct{}{}
	}
	return first, c, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[c]; ok {
			delete(s.subs, c)
			close(c)
		}
	}
}

// Write sends input to the shell.
func (s *Session) Write(p []byte) error {
	_, err := s.pty.Write(p)
	return err
}

// Resize changes the size of the terminal.
func (s *Session) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return nil
	}
	return s.pty.Resize(cols, rows)
}

// Running tells whether the shell is alive.
func (s *Session) Running() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Info describes the session.
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{ID: s.id, Name: s.name, Cwd: s.cwd, Running: s.Running()}
}

// Mark returns a position in the output, for Since.
func (s *Session) Mark() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// Since returns the output written after mark that the buffer still holds.
func (s *Session) Since(mark uint64) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.snapshot()
	n := s.total - mark
	if n >= uint64(len(all)) {
		return all
	}
	return all[len(all)-int(n):]
}

// Text returns the last lines of output as plain text.
func (s *Session) Text(lines int) string {
	s.mu.Lock()
	all := s.snapshot()
	s.mu.Unlock()
	return Tail(PlainText(all), lines)
}

// Run types a command and waits until the output stays quiet or timeout
// passes; it returns what the command printed, as plain text.
func (s *Session) Run(command string, timeout time.Duration) (string, error) {
	mark := s.Mark()
	if err := s.Write([]byte(command + enter)); err != nil {
		return "", err
	}
	const quiet = 700 * time.Millisecond
	deadline := time.Now().Add(timeout)
	last, lastChange := mark, time.Now()
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			return PlainText(s.Since(mark)), nil
		case <-time.After(50 * time.Millisecond):
		}
		if m := s.Mark(); m != last {
			last, lastChange = m, time.Now()
		} else if m != mark && time.Since(lastChange) >= quiet {
			break
		}
	}
	return PlainText(s.Since(mark)), nil
}
