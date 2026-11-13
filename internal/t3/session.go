package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// A session is one WebSocket kept open per T3 instance (and token) that every
// RPC reuses: opening a socket per call costs a ticket request and a handshake,
// which made each read of a thread pay that latency again. Requests carry
// their own id, so many calls share the socket at once.
type session struct {
	conn    *websocket.Conn
	key     string
	nextID  atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan rpcMessage
	closed  bool
	err     error
	idle    *time.Timer
}

// sessionIdle closes a socket nobody used for this long.
const sessionIdle = 10 * time.Minute

// dialTimeout bounds the ticket request and the handshake of a new session.
const dialTimeout = 10 * time.Second

// slowCall is logged so slow T3 answers show up when tuning.
const slowCall = time.Second

var (
	sessionsMu sync.Mutex
	sessions   = map[string]*session{}
	// dialing serializes the opening of a session per key, so a burst of
	// calls opens one socket instead of one each.
	dialing = map[string]*sync.Mutex{}
)

var errSessionClosed = errors.New("t3: session closed")

func (c Client) sessionKey() string { return c.BaseURL + "\x00" + c.Token }

// session returns the open session of the client's instance, dialing it if needed.
func (c Client) session(ctx context.Context) (*session, error) {
	key := c.sessionKey()
	sessionsMu.Lock()
	if s := sessions[key]; s != nil {
		sessionsMu.Unlock()
		return s, nil
	}
	lock := dialing[key]
	if lock == nil {
		lock = &sync.Mutex{}
		dialing[key] = lock
	}
	sessionsMu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	sessionsMu.Lock()
	if s := sessions[key]; s != nil {
		sessionsMu.Unlock()
		return s, nil
	}
	sessionsMu.Unlock()

	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	ticket, err := c.ticket(dctx)
	if err != nil {
		return nil, err
	}
	target, err := c.wsURL(ticket)
	if err != nil {
		return nil, err
	}
	conn, resp, err := websocket.Dial(dctx, target, &websocket.DialOptions{HTTPClient: c.HTTP})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, errUnauthorized
		}
		return nil, fmt.Errorf("t3: websocket: %w", err)
	}
	conn.SetReadLimit(rpcReadLimit)
	s := &session{conn: conn, key: key, pending: map[string]chan rpcMessage{}}
	s.idle = time.AfterFunc(sessionIdle, func() { s.fail(errSessionClosed) })
	sessionsMu.Lock()
	sessions[key] = s
	sessionsMu.Unlock()
	go s.read()
	return s, nil
}

// read routes each answer to its caller and keeps the socket alive.
func (s *session) read() {
	for {
		_, data, err := s.conn.Read(context.Background())
		if err != nil {
			s.fail(err)
			return
		}
		msgs, err := decodeFrame(data)
		if err != nil {
			continue
		}
		for _, m := range msgs {
			switch m.Tag {
			case "Ping":
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := s.conn.Write(ctx, websocket.MessageText, []byte(`{"_tag":"Pong"}`))
				cancel()
				if err != nil {
					s.fail(err)
					return
				}
			case "Exit":
				s.mu.Lock()
				ch := s.pending[m.RequestID]
				delete(s.pending, m.RequestID)
				s.mu.Unlock()
				if ch != nil {
					ch <- m
				}
			case "Defect", "ClientProtocolError":
				// Not tied to one request: the socket is no longer trustworthy.
				s.fail(fmt.Errorf("t3: %s", data))
				return
			}
		}
	}
}

// fail closes the session and wakes every waiting call with err.
func (s *session) fail(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed, s.err = true, err
	pending := s.pending
	s.pending = map[string]chan rpcMessage{}
	s.mu.Unlock()
	s.idle.Stop()
	sessionsMu.Lock()
	if sessions[s.key] == s {
		delete(sessions, s.key)
	}
	sessionsMu.Unlock()
	_ = s.conn.Close(websocket.StatusNormalClosure, "")
	for _, ch := range pending {
		close(ch)
	}
}

// errTransport marks failures of the socket, as opposed to answers from T3:
// only those are worth retrying on a fresh connection.
type errTransport struct{ err error }

func (e errTransport) Error() string { return e.err.Error() }
func (e errTransport) Unwrap() error { return e.err }

// do sends one request and waits for its answer.
func (s *session) do(ctx context.Context, method string, payload, out any) error {
	id := strconv.FormatUint(s.nextID.Add(1), 10)
	ch := make(chan rpcMessage, 1)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errTransport{errSessionClosed}
	}
	s.pending[id] = ch
	s.mu.Unlock()
	s.idle.Reset(sessionIdle)
	forget := func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}
	req, err := json.Marshal(map[string]any{"_tag": "Request", "id": id, "tag": method, "payload": payload, "headers": []any{}})
	if err != nil {
		forget()
		return err
	}
	// The socket is shared: a cancelled caller must not tear it down mid-frame.
	wctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = s.conn.Write(wctx, websocket.MessageText, req)
	cancel()
	if err != nil {
		forget()
		s.fail(err)
		return errTransport{err}
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return errTransport{fmt.Errorf("t3: %s: %w", method, s.err)}
		}
		return finish(method, m.Exit, out)
	case <-ctx.Done():
		forget()
		return fmt.Errorf("t3: %s: %w", method, ctx.Err())
	}
}

// callShared runs one RPC on the instance's shared session, retrying once on a
// fresh socket when the old one turned out to be dead.
func (c Client) callShared(ctx context.Context, method string, payload, out any) error {
	start := time.Now()
	defer func() {
		if d := time.Since(start); d > slowCall {
			log.Printf("t3: %s took %s", method, d.Round(time.Millisecond))
		}
	}()
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var s *session
		s, err = c.session(ctx)
		if err != nil {
			return err
		}
		err = s.do(ctx, method, payload, out)
		var te errTransport
		if !errors.As(err, &te) {
			return err
		}
	}
	return err
}
