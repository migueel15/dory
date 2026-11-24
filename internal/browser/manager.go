// Package browser gives every thread of a node its own browser: tabs the
// agent opens, drives and captures with or without anybody watching. A tab
// lives in the node's headless Chromium, or in a native view of the desktop
// app while the user has it on screen; the agent drives both through CDP.
package browser

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/i18n"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	timeoutShort = 5 * time.Second
	maxLive      = 8
	idleTab      = 10 * time.Minute
	maxConsole   = 200
	// desktopWait is how long a tab waits for the desktop app to show it.
	desktopWait = 5 * time.Second
)

// ErrUserControl refuses the agent's actions while the user drives the tab.
var ErrUserControl = apperr.New(apperr.UserControl, "the user took control of the browser: you can read and capture, but not act until they hand it back")

// Tab is one tab of a thread's browser.
type Tab struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Position int    `json:"position"`
	Active   bool   `json:"active"`
	// Backend is where the page runs now: node, desktop, or empty when closed.
	Backend string `json:"backend"`
}

// State is what the panel draws.
type State struct {
	Tabs        []Tab `json:"tabs"`
	UserControl bool  `json:"user_control"`
}

// Manager owns the browsers of the threads of this node.
type Manager struct {
	DB *sql.DB
	// Profile is the directory of the headless Chromium profile.
	Profile string
	// Changed tells the clients of the node that a thread's browser changed.
	Changed func(threadID string)
	// SearchEngine returns the id of the engine the address bar searches
	// with; nil or an unknown id is Google.
	SearchEngine func() string

	mu      sync.Mutex
	openMu  sync.Mutex
	chrome  *chrome
	live    map[string]*page
	threads map[string]*thread
}

type thread struct {
	userControl bool
	clients     map[*Client]struct{}
}

// page is a live tab.
type page struct {
	tab, thread string
	target      Target
	backend     string
	targetID    string
	used        time.Time
	console     []string
	viewers     map[*Client]struct{}
	casting     bool
}

func (m *Manager) init() {
	if m.live == nil {
		m.live = map[string]*page{}
		m.threads = map[string]*thread{}
	}
}

func (m *Manager) threadLocked(id string) *thread {
	m.init()
	t := m.threads[id]
	if t == nil {
		t = &thread{clients: map[*Client]struct{}{}}
		m.threads[id] = t
	}
	return t
}

// Run closes idle pages and the headless browser when nothing uses it.
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			if m.chrome != nil {
				m.chrome.stop()
			}
			m.mu.Unlock()
			return
		case <-tick.C:
			m.reap()
		}
	}
}

func (m *Manager) reap() {
	m.mu.Lock()
	m.init()
	var idle []*page
	headless := 0
	for _, p := range m.live {
		if p.backend != "node" {
			continue
		}
		if len(p.viewers) == 0 && time.Since(p.used) > idleTab {
			idle = append(idle, p)
			delete(m.live, p.tab)
			continue
		}
		headless++
	}
	var stop *chrome
	if headless == 0 && m.chrome != nil {
		stop, m.chrome = m.chrome, nil
	}
	m.mu.Unlock()
	for _, p := range idle {
		p.target.Close()
		m.notify(p.thread)
	}
	if stop != nil {
		stop.stop()
	}
}

// State returns the browser of a thread.
func (m *Manager) State(ctx context.Context, threadID string) (State, error) {
	tabs, err := m.Tabs(ctx, threadID)
	m.mu.Lock()
	uc := m.threadLocked(threadID).userControl
	m.mu.Unlock()
	return State{Tabs: tabs, UserControl: uc}, err
}

// Tabs lists the tabs of a thread in order.
func (m *Manager) Tabs(ctx context.Context, threadID string) ([]Tab, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT id, url, title, position, active FROM thread_browser_tabs WHERE thread_id = ? ORDER BY position`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tab{}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	for rows.Next() {
		var t Tab
		if err := rows.Scan(&t.ID, &t.URL, &t.Title, &t.Position, &t.Active); err != nil {
			return nil, err
		}
		if p := m.live[t.ID]; p != nil {
			t.Backend = p.backend
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (m *Manager) tab(ctx context.Context, threadID, id string) (Tab, error) {
	tabs, err := m.Tabs(ctx, threadID)
	if err != nil {
		return Tab{}, err
	}
	for _, t := range tabs {
		if (id == "" && t.Active) || (id != "" && t.ID == id) {
			return t, nil
		}
	}
	if id == "" {
		return Tab{}, errors.New(i18n.Text("browser.hilo_no_tiene_pestanas"))
	}
	return Tab{}, fmt.Errorf(i18n.Text("browser.no_existe_pestana_este"), id)
}

func (m *Manager) insert(ctx context.Context, threadID, url string) (Tab, error) {
	t := Tab{ID: rand.Text()[:12], URL: url, Active: true}
	_, err := m.DB.ExecContext(ctx, `INSERT INTO thread_browser_tabs(id, thread_id, url, position, active, created_at)
		VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM thread_browser_tabs WHERE thread_id = ?), 0, ?)`,
		t.ID, threadID, url, threadID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return t, err
	}
	return t, m.activate(ctx, threadID, t.ID)
}

func (m *Manager) activate(ctx context.Context, threadID, id string) error {
	_, err := m.DB.ExecContext(ctx, `UPDATE thread_browser_tabs SET active = (id = ?) WHERE thread_id = ?`, id, threadID)
	return err
}

func (m *Manager) setURL(tabID, url, title string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
	defer cancel()
	var threadID string
	if url != "" {
		m.DB.QueryRowContext(ctx, `UPDATE thread_browser_tabs SET url = ? WHERE id = ? RETURNING thread_id`, url, tabID).Scan(&threadID)
	}
	if title != "" {
		m.DB.QueryRowContext(ctx, `UPDATE thread_browser_tabs SET title = ? WHERE id = ? RETURNING thread_id`, title, tabID).Scan(&threadID)
	}
	if threadID != "" {
		m.notify(threadID)
	}
}

// Open loads url in the active tab, or in a new one when asked or when the
// thread has none. The page opens in the engine that shows the tab.
func (m *Manager) Open(ctx context.Context, threadID, url string, newTab bool) (Tab, error) {
	url = normalizeURL(url)
	var t Tab
	var err error
	fresh := false
	if !newTab {
		t, err = m.tab(ctx, threadID, "")
	}
	if newTab || err != nil {
		if t, err = m.insert(ctx, threadID, url); err != nil {
			return t, err
		}
		fresh = true
		m.notify(threadID)
	}
	m.mu.Lock()
	m.init()
	live := m.live[t.ID] != nil
	m.mu.Unlock()
	p, err := m.ensure(ctx, threadID, t.ID)
	if err != nil {
		return t, err
	}
	// A page opened just now already loads the address of its tab.
	if live || !fresh {
		if err := navigate(ctx, p.target, url); err != nil {
			return t, err
		}
	} else {
		waitLoad(ctx, p.target)
	}
	return m.tab(ctx, threadID, t.ID)
}

// Add creates a tab without loading it; the panel or the agent loads it.
func (m *Manager) Add(ctx context.Context, threadID, url string) (Tab, error) {
	t, err := m.insert(ctx, threadID, normalizeURL(url))
	if err == nil {
		m.notify(threadID)
	}
	return t, err
}

// Activate brings a tab to the front.
func (m *Manager) Activate(ctx context.Context, threadID, id string) error {
	if _, err := m.tab(ctx, threadID, id); err != nil {
		return err
	}
	if err := m.activate(ctx, threadID, id); err != nil {
		return err
	}
	m.notify(threadID)
	return nil
}

// Close closes a tab (the active one when id is empty).
func (m *Manager) Close(ctx context.Context, threadID, id string) error {
	t, err := m.tab(ctx, threadID, id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.init()
	p := m.live[t.ID]
	delete(m.live, t.ID)
	m.mu.Unlock()
	if p != nil {
		p.target.Close()
	}
	if _, err := m.DB.ExecContext(ctx, `DELETE FROM thread_browser_tabs WHERE id = ?`, t.ID); err != nil {
		return err
	}
	if t.Active {
		m.DB.ExecContext(ctx, `UPDATE thread_browser_tabs SET active = 1 WHERE id = (SELECT id FROM thread_browser_tabs WHERE thread_id = ? ORDER BY position DESC LIMIT 1)`, threadID)
	}
	m.notify(threadID)
	return nil
}

// Forget drops the browser of a thread, such as when it is deleted.
func (m *Manager) Forget(ctx context.Context, threadID string) {
	tabs, _ := m.Tabs(ctx, threadID)
	for _, t := range tabs {
		m.Close(ctx, threadID, t.ID)
	}
}

// SetUserControl gives the tab to the user, or back to the agent.
func (m *Manager) SetUserControl(threadID string, on bool) {
	m.mu.Lock()
	m.threadLocked(threadID).userControl = on
	m.mu.Unlock()
	m.notify(threadID)
}

// ensure returns the live page of a tab, opening it in the headless
// browser when no engine has it.
func (m *Manager) ensure(ctx context.Context, threadID, tabID string) (*page, error) {
	m.openMu.Lock()
	defer m.openMu.Unlock()
	m.mu.Lock()
	m.init()
	if p := m.live[tabID]; p != nil {
		p.used = time.Now()
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()
	t, err := m.tab(ctx, threadID, tabID)
	if err != nil {
		return nil, err
	}
	// The desktop app shows the active tab in a native view: give it time
	// to take the tab, so the node needs no Chromium of its own.
	if p := m.awaitDesktop(ctx, threadID, t); p != nil {
		return p, nil
	}
	m.mu.Lock()
	ch := m.chrome
	m.mu.Unlock()
	if ch == nil || !ch.alive() {
		if ch, err = startChrome(m.Profile, m.browserEvent); err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.chrome = ch
		m.mu.Unlock()
	}
	p := &page{tab: tabID, thread: threadID, backend: "node", used: time.Now(), viewers: map[*Client]struct{}{}}
	s, err := ch.newPage(ctx, "", "", m.pageEvent(p))
	if err != nil {
		return nil, err
	}
	p.target, p.targetID = s, s.targetID
	if err := setup(ctx, p.target, true); err != nil {
		s.Close()
		return nil, err
	}
	if t.URL != "" && t.URL != "about:blank" {
		navigate(ctx, p.target, t.URL)
	}
	m.adoptLive(p)
	return p, nil
}

// awaitDesktop waits for a desktop panel open on the thread to attach the
// active tab t. It returns nil when there is no such panel or it does not
// attach in time.
func (m *Manager) awaitDesktop(ctx context.Context, threadID string, t Tab) *page {
	if !t.Active {
		return nil
	}
	deadline := time.Now().Add(desktopWait)
	for {
		desktop := false
		for _, c := range m.clients(threadID) {
			desktop = desktop || c.desktop
		}
		if !desktop {
			return nil
		}
		m.mu.Lock()
		p := m.live[t.ID]
		if p != nil {
			p.used = time.Now()
		}
		m.mu.Unlock()
		if p != nil {
			return p
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// adoptLive registers a live page, closing the least used headless pages
// over the limit.
func (m *Manager) adoptLive(p *page) {
	m.mu.Lock()
	m.live[p.tab] = p
	var evict []*page
	for {
		n := 0
		var oldest *page
		for _, q := range m.live {
			if q.backend != "node" {
				continue
			}
			n++
			if q != p && len(q.viewers) == 0 && (oldest == nil || q.used.Before(oldest.used)) {
				oldest = q
			}
		}
		if n <= maxLive || oldest == nil {
			break
		}
		delete(m.live, oldest.tab)
		evict = append(evict, oldest)
	}
	m.mu.Unlock()
	for _, q := range evict {
		q.target.Close()
	}
	m.notify(p.thread)
}

// browserEvent adopts windows opened by a page of the headless browser as
// new tabs of the same thread.
func (m *Manager) browserEvent(method string, params json.RawMessage) {
	if method != "Target.targetCreated" {
		return
	}
	var ev struct {
		TargetInfo struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			OpenerID string `json:"openerId"`
			URL      string `json:"url"`
		} `json:"targetInfo"`
	}
	if json.Unmarshal(params, &ev) != nil || ev.TargetInfo.Type != "page" || ev.TargetInfo.OpenerID == "" {
		return
	}
	m.mu.Lock()
	var opener *page
	for _, p := range m.live {
		if p.targetID == ev.TargetInfo.OpenerID {
			opener = p
		}
	}
	ch := m.chrome
	m.mu.Unlock()
	if opener == nil || ch == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		t, err := m.insert(ctx, opener.thread, ev.TargetInfo.URL)
		if err != nil {
			return
		}
		p := &page{tab: t.ID, thread: opener.thread, backend: "node", used: time.Now(), viewers: map[*Client]struct{}{}}
		s, err := ch.newPage(ctx, "", ev.TargetInfo.TargetID, m.pageEvent(p))
		if err != nil {
			return
		}
		p.target, p.targetID = s, s.targetID
		setup(ctx, p.target, true)
		m.adoptLive(p)
	}()
}

// pageEvent follows a page: its address, title, console and screencast.
// It runs on the reader of the connection, so CDP calls go elsewhere.
func (m *Manager) pageEvent(p *page) EventFunc {
	return func(method string, params json.RawMessage) {
		switch method {
		case "Page.frameNavigated":
			var ev struct {
				Frame struct {
					ParentID string `json:"parentId"`
					URL      string `json:"url"`
				} `json:"frame"`
			}
			if json.Unmarshal(params, &ev) == nil && ev.Frame.ParentID == "" {
				go m.setURL(p.tab, ev.Frame.URL, "")
			}
		case "Page.navigatedWithinDocument":
			var ev struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(params, &ev) == nil {
				go m.setURL(p.tab, ev.URL, "")
			}
		case "Page.domContentEventFired", "Page.loadEventFired":
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
				defer cancel()
				var title string
				if evaluate(ctx, p.target, "document.title", &title) == nil && title != "" {
					m.setURL(p.tab, "", title)
				}
			}()
		case "Runtime.consoleAPICalled":
			var ev struct {
				Type string `json:"type"`
				Args []struct {
					Value       any    `json:"value"`
					Description string `json:"description"`
				} `json:"args"`
			}
			if json.Unmarshal(params, &ev) == nil {
				parts := make([]string, 0, len(ev.Args))
				for _, a := range ev.Args {
					if a.Value != nil {
						parts = append(parts, fmt.Sprint(a.Value))
					} else {
						parts = append(parts, a.Description)
					}
				}
				m.log(p, ev.Type+": "+strings.Join(parts, " "))
			}
		case "Runtime.exceptionThrown":
			var ev struct {
				ExceptionDetails struct {
					Text      string `json:"text"`
					Exception struct {
						Description string `json:"description"`
					} `json:"exception"`
				} `json:"exceptionDetails"`
			}
			if json.Unmarshal(params, &ev) == nil {
				m.log(p, "exception: "+ev.ExceptionDetails.Text+" "+ev.ExceptionDetails.Exception.Description)
			}
		case "Page.screencastFrame":
			m.frame(p, params)
		}
	}
}

func (m *Manager) log(p *page, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.console = append(p.console, line)
	if len(p.console) > maxConsole {
		p.console = p.console[len(p.console)-maxConsole:]
	}
}

// setup enables the domains Dory follows on a page; headless pages get a
// default viewport.
func setup(ctx context.Context, t Target, headless bool) error {
	for _, method := range []string{"Page.enable", "Runtime.enable"} {
		if _, err := t.Call(ctx, method, nil); err != nil {
			return err
		}
	}
	if headless {
		_, err := t.Call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 1280, "height": 800, "deviceScaleFactor": 1, "mobile": false})
		return err
	}
	return nil
}

// SearchEngines maps the id of a search engine to its query URL.
var SearchEngines = map[string]string{
	"google":     "https://www.google.com/search?q=",
	"duckduckgo": "https://duckduckgo.com/?q=",
	"bing":       "https://www.bing.com/search?q=",
	"brave":      "https://search.brave.com/search?q=",
	"startpage":  "https://www.startpage.com/do/search?q=",
}

var addressLike = regexp.MustCompile(`^(localhost|\[[0-9a-fA-F:]+\]|[^\s/:]+\.[a-zA-Z]{2,}|\d{1,3}(\.\d{1,3}){3})(:\d+)?(/\S*)?$`)

// address turns what the user typed in the address bar into a URL: an
// address as is, anything else a search with the node's engine.
func (m *Manager) address(text string) string {
	t := strings.TrimSpace(text)
	if t == "" || strings.Contains(t, "://") || strings.HasPrefix(t, "about:") || strings.HasPrefix(t, "data:") || addressLike.MatchString(t) {
		return normalizeURL(t)
	}
	engine := ""
	if m.SearchEngine != nil {
		engine = m.SearchEngine()
	}
	base, ok := SearchEngines[engine]
	if !ok {
		base = SearchEngines["google"]
	}
	return base + url.QueryEscape(t)
}

func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return "about:blank"
	}
	if !strings.Contains(u, "://") && !strings.HasPrefix(u, "about:") && !strings.HasPrefix(u, "data:") {
		if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.") || strings.HasPrefix(u, "[::1]") {
			return "http://" + u
		}
		return "https://" + u
	}
	return u
}

func (m *Manager) clients(threadID string) []*Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.threadLocked(threadID)
	out := make([]*Client, 0, len(t.clients))
	for c := range t.clients {
		out = append(out, c)
	}
	return out
}

// notify pushes the new state to the panels open on the thread.
func (m *Manager) notify(threadID string) {
	if m.Changed != nil {
		m.Changed(threadID)
	}
	cs := m.clients(threadID)
	if len(cs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutShort)
	defer cancel()
	st, err := m.State(ctx, threadID)
	if err != nil {
		return
	}
	for _, c := range cs {
		c.send(map[string]any{"t": "state", "state": st})
	}
}
