package devices

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// Frame kinds sent to viewers as the first byte of a binary message.
const (
	FrameConfig = 0
	FrameKey    = 1
	FrameDelta  = 2
)

const idleGrace = 5 * time.Second

// Sub is one viewer of a session. Frames arrive on C; it closes when the
// session ends.
type Sub struct {
	C     chan []byte
	stale bool
	s     *Session
}

// Session is the scrcpy stream of one device, shared by all its viewers.
type Session struct {
	m      *Manager
	serial string
	ready  chan struct{}
	err    error

	mu     sync.Mutex
	conn   *scrcpyConn
	subs   map[*Sub]struct{}
	config []byte
	timer  *time.Timer
	closed bool
	ctlMu  sync.Mutex
}

// Manager owns the sessions of this node.
type Manager struct {
	Tools Tools
	ADB   ADB

	mu       sync.Mutex
	sessions map[string]*Session
	avds     map[string]*avdProc
	lastUI   map[string][]UIElement
}

// NewManager looks for the Android tools and starts empty.
func NewManager() *Manager {
	t := FindTools()
	return &Manager{Tools: t, ADB: ADB{Path: t.ADB}, sessions: map[string]*Session{}, avds: map[string]*avdProc{}}
}

// Subscribe joins the stream of a device, starting scrcpy for the first viewer.
func (m *Manager) Subscribe(ctx context.Context, serial string, o Options) (*Sub, Meta, error) {
	for {
		m.mu.Lock()
		s := m.sessions[serial]
		if s == nil {
			s = &Session{m: m, serial: serial, ready: make(chan struct{}), subs: map[*Sub]struct{}{}}
			m.sessions[serial] = s
			m.mu.Unlock()
			conn, err := startScrcpy(ctx, m.ADB, serial, o)
			s.mu.Lock()
			s.conn, s.err = conn, err
			s.mu.Unlock()
			close(s.ready)
			if err != nil {
				m.drop(s)
				return nil, Meta{}, err
			}
			go s.run()
		} else {
			m.mu.Unlock()
		}
		select {
		case <-s.ready:
		case <-ctx.Done():
			return nil, Meta{}, ctx.Err()
		}
		if s.err != nil {
			return nil, Meta{}, s.err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			continue
		}
		if s.timer != nil {
			s.timer.Stop()
			s.timer = nil
		}
		sub := &Sub{C: make(chan []byte, 128), s: s, stale: true}
		if s.config != nil {
			sub.C <- s.config
		}
		s.subs[sub] = struct{}{}
		meta := s.conn.meta
		s.mu.Unlock()
		// A new viewer needs a key frame to start decoding.
		_ = s.Control([]byte{CtlResetVideo})
		return sub, meta, nil
	}
}

func (m *Manager) drop(s *Session) {
	m.mu.Lock()
	if m.sessions[s.serial] == s {
		delete(m.sessions, s.serial)
	}
	m.mu.Unlock()
}

// Active reports whether a device is being streamed.
func (m *Manager) Active(serial string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[serial]
	return s != nil && s.err == nil
}

// Control sends a control message to the device of a session if one is active.
func (m *Manager) Control(serial string, msg []byte) error {
	m.mu.Lock()
	s := m.sessions[serial]
	m.mu.Unlock()
	if s == nil {
		return errors.New("no hay sesión activa")
	}
	return s.Control(msg)
}

// Close ends the session of a device.
func (m *Manager) Close(serial string) {
	m.mu.Lock()
	s := m.sessions[serial]
	m.mu.Unlock()
	if s != nil {
		s.shutdown()
	}
}

func (s *Session) run() {
	// Drain what the device sends on the control socket (clipboard, ack).
	go io.Copy(io.Discard, s.conn.control)
	for {
		p, err := ReadPacket(s.conn.video)
		if err != nil {
			s.shutdown()
			return
		}
		kind := byte(FrameDelta)
		switch {
		case p.Config:
			kind = FrameConfig
		case p.Key:
			kind = FrameKey
		}
		msg := append([]byte{kind}, p.Data...)
		s.mu.Lock()
		if kind == FrameConfig {
			s.config = msg
		}
		resend := false
		for sub := range s.subs {
			if sub.stale && kind == FrameDelta {
				continue
			}
			select {
			case sub.C <- msg:
				if kind != FrameDelta {
					sub.stale = false
				}
			default:
				// A slow viewer skips to the next key frame.
				if !sub.stale {
					sub.stale, resend = true, true
				}
			}
		}
		s.mu.Unlock()
		if resend {
			_ = s.Control([]byte{CtlResetVideo})
		}
	}
}

// Control writes one control message; only known types pass.
func (s *Session) Control(msg []byte) error {
	if len(msg) == 0 || len(msg) > maxControlSize {
		return errors.New("mensaje de control no válido")
	}
	switch msg[0] {
	case CtlKeycode, CtlText, CtlTouch, CtlScroll, CtlBackOrOn, CtlDisplayPwr, CtlResetVideo:
	default:
		return errors.New("tipo de control no admitido")
	}
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return errors.New("sesión cerrada")
	}
	s.ctlMu.Lock()
	defer s.ctlMu.Unlock()
	c.control.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err := c.control.Write(msg)
	return err
}

// Leave removes a viewer; the session ends shortly after the last one goes.
func (sub *Sub) Leave() {
	s := sub.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	if len(s.subs) == 0 && !s.closed {
		s.timer = time.AfterFunc(idleGrace, func() {
			s.mu.Lock()
			idle := len(s.subs) == 0
			s.mu.Unlock()
			if idle {
				s.shutdown()
			}
		})
	}
}

func (s *Session) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conn := s.conn
	for sub := range s.subs {
		close(sub.C)
	}
	s.subs = map[*Sub]struct{}{}
	s.mu.Unlock()
	s.m.drop(s)
	if conn != nil {
		conn.close()
	}
}
