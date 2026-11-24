package browser

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/migueel15/dory/internal/i18n"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Client is a browser panel open on a thread. A desktop client shows the
// tab in a native view and relays CDP to it; any other client watches the
// headless page as a screencast and sends its input back.
type Client struct {
	m       *Manager
	thread  string
	ws      *websocket.Conn
	desktop bool
	wmu     sync.Mutex
	ctx     context.Context
}

type clientMessage struct {
	T       string          `json:"t"`
	Tab     string          `json:"tab"`
	URL     string          `json:"url"`
	To      string          `json:"to"`
	New     bool            `json:"new"`
	On      bool            `json:"on"`
	Width   int             `json:"width"`
	Height  int             `json:"height"`
	Scale   float64         `json:"scale"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
	Desktop bool            `json:"desktop"`
}

func (c *Client) send(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	if c.ws.Write(ctx, websocket.MessageText, raw) != nil {
		c.ws.Close(websocket.StatusGoingAway, "")
	}
}

func (c *Client) fail(err error) {
	c.send(map[string]any{"t": "error", "message": err.Error()})
}

// Serve runs the panel channel of a thread until the client leaves.
// desktop marks the desktop app, which hosts tabs in native views.
func (m *Manager) Serve(ctx context.Context, ws *websocket.Conn, threadID string, desktop bool) {
	ws.SetReadLimit(64 << 20)
	c := &Client{m: m, thread: threadID, ws: ws, desktop: desktop, ctx: ctx}
	m.mu.Lock()
	m.threadLocked(threadID).clients[c] = struct{}{}
	m.mu.Unlock()
	defer m.leave(c)
	if st, err := m.State(ctx, threadID); err == nil {
		c.send(map[string]any{"t": "state", "state": st})
	}
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var msg clientMessage
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		m.handle(c, msg)
	}
}

func (m *Manager) leave(c *Client) {
	m.mu.Lock()
	delete(m.threadLocked(c.thread).clients, c)
	var detached []*page
	for id, p := range m.live {
		if dt, ok := p.target.(*desktopTarget); ok && dt.c == c {
			delete(m.live, id)
			detached = append(detached, p)
		}
	}
	m.mu.Unlock()
	for _, p := range detached {
		p.target.(*desktopTarget).fail()
	}
	m.stopCast(c)
	if len(detached) > 0 {
		m.notify(c.thread)
	}
}

func (m *Manager) handle(c *Client, msg clientMessage) {
	ctx := c.ctx
	switch msg.T {
	case "cdp_result":
		m.desktopResult(c, msg)
	case "cdp_event":
		m.mu.Lock()
		p := m.live[msg.Tab]
		m.mu.Unlock()
		if p != nil {
			if dt, ok := p.target.(*desktopTarget); ok && dt.c == c && dt.onEvent != nil {
				dt.onEvent(msg.Method, msg.Params)
			}
		}
	case "ready":
		m.mu.Lock()
		p := m.live[msg.Tab]
		m.mu.Unlock()
		if p != nil {
			if dt, ok := p.target.(*desktopTarget); ok && dt.c == c {
				dt.markReady()
				go func() {
					sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()
					setup(sctx, dt, false)
				}()
			}
		}
	case "view":
		go func() {
			var err error
			if c.desktop {
				err = m.attachDesktop(c, msg.Tab, msg.Width, msg.Height, msg.Scale)
			} else {
				err = m.startCast(c, msg.Tab, msg.Width, msg.Height, msg.Scale)
			}
			if err != nil {
				c.fail(err)
			}
		}()
	case "hide":
		// A hidden native view stays parked in the client with its page;
		// it goes back to the node when its tab closes or the client leaves.
		m.stopCast(c)
	case "release":
		m.detachDesktop(c, msg.Tab)
	case "open":
		go func() {
			if msg.New {
				if _, err := m.Add(ctx, c.thread, msg.URL); err != nil {
					c.fail(err)
				}
				return
			}
			if _, err := m.userNavigate(ctx, c.thread, msg.Tab, m.address(msg.URL)); err != nil {
				c.fail(err)
			}
		}()
	case "nav":
		go func() {
			if _, err := m.userNavigate(ctx, c.thread, msg.Tab, msg.To); err != nil {
				c.fail(err)
			}
		}()
	case "activate":
		if err := m.Activate(ctx, c.thread, msg.Tab); err != nil {
			c.fail(err)
		}
	case "close":
		if err := m.Close(ctx, c.thread, msg.Tab); err != nil {
			c.fail(err)
		}
	case "control":
		m.SetUserControl(c.thread, msg.On)
	case "input":
		switch msg.Method {
		case "Input.dispatchMouseEvent", "Input.dispatchKeyEvent", "Input.insertText":
		default:
			return
		}
		m.mu.Lock()
		p := m.live[msg.Tab]
		if p != nil {
			p.used = time.Now()
		}
		m.mu.Unlock()
		if p != nil && p.backend == "node" {
			go p.target.Call(ctx, msg.Method, msg.Params)
		}
	}
}

// userNavigate moves a tab for the user; it is never refused.
func (m *Manager) userNavigate(ctx context.Context, threadID, tabID, to string) (Tab, error) {
	p, err := m.use(ctx, threadID, tabID)
	if err != nil {
		return Tab{}, err
	}
	return m.move(ctx, p, to)
}

// startCast streams a headless page to a client at the size of its panel.
func (m *Manager) startCast(c *Client, tabID string, width, height int, scale float64) error {
	m.stopCast(c)
	p, err := m.use(c.ctx, c.thread, tabID)
	if err != nil {
		return err
	}
	if p.backend != "node" {
		return nil
	}
	m.mu.Lock()
	p.viewers[c] = struct{}{}
	p.casting = true
	m.mu.Unlock()
	// Frames come at the pixel density of the client's screen, so the page
	// looks as sharp as the native view that replaces it.
	if scale < 1 || scale > 3 {
		scale = 1
	}
	if width >= 200 && height >= 200 {
		p.target.Call(c.ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": scale, "mobile": false})
	} else {
		width, height = 1280, 800
	}
	_, err = p.target.Call(c.ctx, "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 85, "maxWidth": int(float64(width) * scale), "maxHeight": int(float64(height) * scale), "everyNthFrame": 1})
	return err
}

func (m *Manager) stopCast(c *Client) {
	m.mu.Lock()
	var stop []*page
	for _, p := range m.live {
		if _, ok := p.viewers[c]; ok {
			delete(p.viewers, c)
			if len(p.viewers) == 0 && p.casting {
				p.casting = false
				stop = append(stop, p)
			}
		}
	}
	m.mu.Unlock()
	for _, p := range stop {
		ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
		p.target.Call(ctx, "Page.stopScreencast", nil)
		cancel()
	}
}

// frame sends a screencast frame to the viewers and acknowledges it after,
// so a single frame is in flight.
func (m *Manager) frame(p *page, params json.RawMessage) {
	var ev struct {
		Data      string `json:"data"`
		SessionID int    `json:"sessionId"`
		Metadata  struct {
			DeviceWidth  float64 `json:"deviceWidth"`
			DeviceHeight float64 `json:"deviceHeight"`
		} `json:"metadata"`
	}
	if json.Unmarshal(params, &ev) != nil {
		return
	}
	m.mu.Lock()
	viewers := make([]*Client, 0, len(p.viewers))
	for c := range p.viewers {
		viewers = append(viewers, c)
	}
	m.mu.Unlock()
	go func() {
		for _, c := range viewers {
			c.send(map[string]any{"t": "frame", "tab": p.tab, "data": ev.Data, "w": ev.Metadata.DeviceWidth, "h": ev.Metadata.DeviceHeight})
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
		defer cancel()
		p.target.Call(ctx, "Page.screencastFrameAck", map[string]int{"sessionId": ev.SessionID})
	}()
}

// attachDesktop moves a tab to the native view of a desktop client, also in
// the middle of a turn: whoever looks at a tab sees it native and sharp, and
// the agent's next actions go to that view. The page reloads its address
// there, so state the agent left unsaved in the headless page is lost.
func (m *Manager) attachDesktop(c *Client, tabID string, width, height int, scale float64) error {
	t, err := m.tab(c.ctx, c.thread, tabID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	p := m.live[t.ID]
	if p != nil {
		if dt, ok := p.target.(*desktopTarget); ok && dt.c == c {
			m.mu.Unlock()
			// The view was parked: show it again as it was.
			c.send(map[string]any{"t": "attach", "tab": t.ID, "url": t.URL})
			return nil
		}
	}
	if p != nil {
		delete(m.live, t.ID)
	}
	m.mu.Unlock()
	if p != nil {
		if dt, ok := p.target.(*desktopTarget); ok {
			dt.c.send(map[string]any{"t": "detach", "tab": t.ID})
			dt.fail()
		} else {
			p.target.Close()
		}
	}
	m.stopCast(c)
	dt := &desktopTarget{c: c, tab: t.ID, ready: make(chan struct{}), closed: make(chan struct{}), pending: map[int64]chan clientMessage{}}
	np := &page{tab: t.ID, thread: c.thread, backend: "desktop", target: dt, used: time.Now(), viewers: map[*Client]struct{}{}}
	if p != nil {
		np.console = p.console
	}
	dt.onEvent = m.pageEvent(np)
	m.mu.Lock()
	m.live[t.ID] = np
	m.mu.Unlock()
	c.send(map[string]any{"t": "attach", "tab": t.ID, "url": t.URL})
	m.notify(c.thread)
	return nil
}

// detachDesktop gives a tab back to the node when its native view hides; the
// headless page reopens its address when somebody uses it.
func (m *Manager) detachDesktop(c *Client, tabID string) {
	m.mu.Lock()
	p := m.live[tabID]
	var dt *desktopTarget
	if p != nil {
		if d, ok := p.target.(*desktopTarget); ok && d.c == c {
			dt = d
			delete(m.live, tabID)
		}
	}
	m.mu.Unlock()
	if dt != nil {
		dt.fail()
		m.notify(c.thread)
	}
}

func (m *Manager) desktopResult(c *Client, msg clientMessage) {
	m.mu.Lock()
	var dt *desktopTarget
	for _, p := range m.live {
		if d, ok := p.target.(*desktopTarget); ok && d.c == c && d.tab == msg.Tab {
			dt = d
		}
	}
	m.mu.Unlock()
	if dt == nil {
		return
	}
	dt.mu.Lock()
	ch := dt.pending[msg.ID]
	delete(dt.pending, msg.ID)
	dt.mu.Unlock()
	if ch != nil {
		ch <- msg
	}
}

// desktopTarget is a native view of the desktop app; CDP goes through the
// panel channel to the view's debugger.
type desktopTarget struct {
	c       *Client
	tab     string
	onEvent EventFunc
	ready   chan struct{}
	closed  chan struct{}
	once    sync.Once
	rOnce   sync.Once
	mu      sync.Mutex
	next    int64
	pending map[int64]chan clientMessage
}

func (d *desktopTarget) markReady() { d.rOnce.Do(func() { close(d.ready) }) }

func (d *desktopTarget) fail() {
	d.once.Do(func() {
		close(d.closed)
		d.mu.Lock()
		for id, ch := range d.pending {
			close(ch)
			delete(d.pending, id)
		}
		d.mu.Unlock()
	})
}

var errDesktopGone = errors.New(i18n.Text("browser.vista_escritorio_cerro_repite"))

func (d *desktopTarget) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-d.ready:
	case <-d.closed:
		return nil, errDesktopGone
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		return nil, errors.New(i18n.Text("browser.vista_escritorio_no_respondio"))
	}
	d.mu.Lock()
	select {
	case <-d.closed:
		d.mu.Unlock()
		return nil, errDesktopGone
	default:
	}
	d.next++
	id := d.next
	ch := make(chan clientMessage, 1)
	d.pending[id] = ch
	d.mu.Unlock()
	if params == nil {
		params = map[string]any{}
	}
	d.c.send(map[string]any{"t": "cdp", "tab": d.tab, "id": id, "method": method, "params": params})
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errDesktopGone
		}
		if msg.Error != "" {
			return nil, errors.New("cdp: " + msg.Error)
		}
		return msg.Result, nil
	case <-ctx.Done():
		d.mu.Lock()
		delete(d.pending, id)
		d.mu.Unlock()
		return nil, ctx.Err()
	}
}

// Close is called when the tab closes: the desktop app drops its view.
func (d *desktopTarget) Close() {
	d.c.send(map[string]any{"t": "detach", "tab": d.tab})
	d.fail()
}
