package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// Target is one page that speaks the Chrome DevTools Protocol, whether it
// lives in the node's headless Chromium or in a native view of the desktop
// app. The agent drives both the same way.
type Target interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Close()
}

// EventFunc receives the CDP events of a target.
type EventFunc func(method string, params json.RawMessage)

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cdpError) Error() string { return fmt.Sprintf("cdp: %s (%d)", e.Message, e.Code) }

// conn is a CDP connection to a browser; pages are flattened sessions on it.
type conn struct {
	ws      *websocket.Conn
	next    atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan cdpMessage
	events  map[string]EventFunc // by session id; "" is the browser
	done    chan struct{}
}

func dialCDP(ctx context.Context, url string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(256 << 20)
	c := &conn{ws: ws, pending: map[int64]chan cdpMessage{}, events: map[string]EventFunc{}, done: make(chan struct{})}
	go c.read()
	return c, nil
}

func (c *conn) read() {
	defer close(c.done)
	for {
		_, raw, err := c.ws.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		var m cdpMessage
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		c.mu.Lock()
		if m.ID != 0 {
			if ch, ok := c.pending[m.ID]; ok {
				delete(c.pending, m.ID)
				ch <- m
			}
			c.mu.Unlock()
			continue
		}
		fn := c.events[m.SessionID]
		c.mu.Unlock()
		if fn != nil {
			fn(m.Method, m.Params)
		}
	}
}

func (c *conn) on(session string, fn EventFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn == nil {
		delete(c.events, session)
		return
	}
	c.events[session] = fn
}

func (c *conn) call(ctx context.Context, session, method string, params any) (json.RawMessage, error) {
	id := c.next.Add(1)
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if params == nil {
		raw = []byte("{}")
	}
	ch := make(chan cdpMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	out, _ := json.Marshal(cdpMessage{ID: id, Method: method, Params: raw, SessionID: session})
	if err := c.ws.Write(ctx, websocket.MessageText, out); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, errors.New(i18n.Text("browser.cdp_navegador_cerro"))
		}
		if m.Error != nil {
			return nil, m.Error
		}
		return m.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *conn) close() { c.ws.Close(websocket.StatusNormalClosure, "") }

// session is a page target of the headless browser.
type session struct {
	c        *conn
	id       string
	targetID string
}

func (s *session) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return s.c.call(ctx, s.id, method, params)
}

func (s *session) Close() {
	s.c.on(s.id, nil)
	ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
	defer cancel()
	s.c.call(ctx, "", "Target.closeTarget", map[string]string{"targetId": s.targetID})
}
