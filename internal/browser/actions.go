package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"strings"
	"time"
)

func evaluate(ctx context.Context, t Target, expr string, out any) error {
	raw, err := t.Call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true})
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.ExceptionDetails != nil {
		return fmt.Errorf("javascript: %s %s", r.ExceptionDetails.Text, r.ExceptionDetails.Exception.Description)
	}
	if out == nil || len(r.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(r.Result.Value, out)
}

// waitLoad waits a little for the page to finish loading; slow pages are
// returned as they are.
func waitLoad(ctx context.Context, t Target) {
	deadline := time.Now().Add(10 * time.Second)
	time.Sleep(150 * time.Millisecond)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		var state string
		if evaluate(ctx, t, "document.readyState", &state) == nil && state == "complete" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func navigate(ctx context.Context, t Target, url string) error {
	raw, err := t.Call(ctx, "Page.navigate", map[string]string{"url": url})
	if err != nil {
		return err
	}
	var r struct {
		ErrorText string `json:"errorText"`
	}
	json.Unmarshal(raw, &r)
	if r.ErrorText != "" {
		return fmt.Errorf(i18n.Text("browser.no_pudo_abrir"), url, r.ErrorText)
	}
	waitLoad(ctx, t)
	return nil
}

// use returns the live page of a tab for reading.
func (m *Manager) use(ctx context.Context, threadID, tabID string) (*page, error) {
	t, err := m.tab(ctx, threadID, tabID)
	if err != nil {
		return nil, err
	}
	return m.ensure(ctx, threadID, t.ID)
}

// act returns the live page of a tab for an action of the agent, refused
// while the user has the control, and shows the agent cursor at x, y.
func (m *Manager) act(ctx context.Context, threadID, tabID string) (*page, error) {
	m.mu.Lock()
	uc := m.threadLocked(threadID).userControl
	m.mu.Unlock()
	if uc {
		return nil, ErrUserControl
	}
	return m.use(ctx, threadID, tabID)
}

// cursor draws the agent's virtual mouse in the page and moves it to x, y,
// so whoever watches sees where the agent acts, in either engine.
func (m *Manager) cursor(ctx context.Context, p *page, x, y float64) {
	for _, c := range m.clients(p.thread) {
		c.send(map[string]any{"t": "agent", "tab": p.tab, "x": x, "y": y})
	}
	js := fmt.Sprintf(`(() => {
  const id = "__dory_agent_cursor";
  let c = document.getElementById(id);
  if (!c) {
    c = document.createElement("div");
    c.id = id;
    c.style.cssText = "position:fixed;left:0;top:0;z-index:2147483647;pointer-events:none;transition:transform .3s ease,opacity .3s;will-change:transform";
    c.innerHTML = '<svg width="22" height="22" viewBox="0 0 24 24"><path d="M3 2l7.5 19 2.6-7.7L21 10.6z" fill="#6d5dfc" stroke="white" stroke-width="1.6" stroke-linejoin="round"/></svg>';
    document.documentElement.appendChild(c);
  }
  c.style.opacity = "1";
  c.style.transform = "translate(%fpx, %fpx)";
  clearTimeout(window.__doryCursor);
  window.__doryCursor = setTimeout(() => { c.style.opacity = "0"; }, 4000);
})()`, x, y)
	if evaluate(ctx, p.target, js, nil) == nil {
		time.Sleep(320 * time.Millisecond)
	}
}

const hideCursor = `(() => { const c = document.getElementById("__dory_agent_cursor"); if (c) { c.style.transition = "none"; c.style.opacity = "0"; } })()`

// Navigate moves the tab to url, or back, forward or reload.
func (m *Manager) Navigate(ctx context.Context, threadID, tabID, to string) (Tab, error) {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return Tab{}, err
	}
	return m.move(ctx, p, to)
}

func (m *Manager) move(ctx context.Context, p *page, to string) (Tab, error) {
	var err error
	switch to {
	case "back":
		err = evaluate(ctx, p.target, "history.back()", nil)
		waitLoad(ctx, p.target)
	case "forward":
		err = evaluate(ctx, p.target, "history.forward()", nil)
		waitLoad(ctx, p.target)
	case "reload":
		_, err = p.target.Call(ctx, "Page.reload", nil)
		waitLoad(ctx, p.target)
	default:
		err = navigate(ctx, p.target, normalizeURL(to))
	}
	if err != nil {
		return Tab{}, err
	}
	return m.tab(ctx, p.thread, p.tab)
}

// Screenshot captures the tab as PNG, the visible part or the whole page.
func (m *Manager) Screenshot(ctx context.Context, threadID, tabID string, fullPage bool) ([]byte, error) {
	p, err := m.use(ctx, threadID, tabID)
	if err != nil {
		return nil, err
	}
	evaluate(ctx, p.target, hideCursor, nil)
	params := map[string]any{"format": "png"}
	if fullPage {
		raw, err := p.target.Call(ctx, "Page.getLayoutMetrics", nil)
		if err != nil {
			return nil, err
		}
		var lm struct {
			CSSContentSize struct {
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"cssContentSize"`
		}
		json.Unmarshal(raw, &lm)
		params["captureBeyondViewport"] = true
		params["clip"] = map[string]any{"x": 0, "y": 0, "width": lm.CSSContentSize.Width, "height": min(lm.CSSContentSize.Height, 16000), "scale": 1}
	}
	raw, err := p.target.Call(ctx, "Page.captureScreenshot", params)
	if err != nil {
		return nil, err
	}
	var r struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(r.Data)
}

// Page is what browser_read returns.
type Page struct {
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Viewport [2]int    `json:"viewport"`
	Text     string    `json:"text"`
	Elements []Element `json:"elements"`
}

// Element is an interactive element in the viewport, with the center to click.
type Element struct {
	Tag   string `json:"tag"`
	Role  string `json:"role,omitempty"`
	Text  string `json:"text,omitempty"`
	Href  string `json:"href,omitempty"`
	Value string `json:"value,omitempty"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

const readJS = `(() => {
  const clip = (s, n) => (s || "").replace(/\s+/g, " ").trim().slice(0, n);
  const els = [];
  for (const e of document.querySelectorAll("a[href],button,input,textarea,select,summary,[role=button],[role=link],[role=tab],[role=menuitem],[role=checkbox],[contenteditable=true],[onclick]")) {
    const r = e.getBoundingClientRect();
    if (r.width < 2 || r.height < 2 || r.bottom < 0 || r.right < 0 || r.top > innerHeight || r.left > innerWidth) continue;
    const st = getComputedStyle(e);
    if (st.visibility === "hidden" || st.display === "none") continue;
    els.push({ tag: e.tagName.toLowerCase(), role: e.getAttribute("role") || e.type || "", text: clip(e.innerText || e.getAttribute("aria-label") || e.placeholder || e.title, 80), href: e.href ? String(e.href).slice(0, 200) : "", value: "value" in e && e.type !== "password" ? clip(String(e.value), 80) : "", x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) });
    if (els.length >= 150) break;
  }
  return { url: location.href, title: document.title, viewport: [innerWidth, innerHeight], text: clip(document.body ? document.body.innerText : "", 20000), elements: els };
})()`

// Read returns the text of the page and the interactive elements in view.
func (m *Manager) Read(ctx context.Context, threadID, tabID string) (Page, error) {
	var out Page
	p, err := m.use(ctx, threadID, tabID)
	if err != nil {
		return out, err
	}
	err = evaluate(ctx, p.target, readJS, &out)
	return out, err
}

// Console returns the last console lines of the page.
func (m *Manager) Console(ctx context.Context, threadID, tabID string) ([]string, error) {
	p, err := m.use(ctx, threadID, tabID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, p.console...), nil
}

// center returns the middle of the element that selector matches, scrolled
// into view.
func center(ctx context.Context, t Target, selector string) (float64, float64, error) {
	sel, _ := json.Marshal(selector)
	var r *struct{ X, Y float64 }
	err := evaluate(ctx, t, fmt.Sprintf(`(() => { const e = document.querySelector(%s); if (!e) return null; e.scrollIntoView({block: "center", inline: "center"}); const r = e.getBoundingClientRect(); return { X: r.left + r.width / 2, Y: r.top + r.height / 2 }; })()`, sel), &r)
	if err != nil {
		return 0, 0, err
	}
	if r == nil {
		return 0, 0, fmt.Errorf(i18n.Text("browser.ningun_elemento_coincide"), selector)
	}
	return r.X, r.Y, nil
}

func mouse(ctx context.Context, t Target, kind string, x, y float64, extra map[string]any) error {
	params := map[string]any{"type": kind, "x": x, "y": y, "button": "left", "clickCount": 1}
	for k, v := range extra {
		params[k] = v
	}
	if kind == "mouseMoved" || kind == "mouseWheel" {
		params["button"] = "none"
	}
	_, err := t.Call(ctx, "Input.dispatchMouseEvent", params)
	return err
}

// Click clicks at x, y or on the element that selector matches.
func (m *Manager) Click(ctx context.Context, threadID, tabID string, x, y float64, selector string, double bool) error {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return err
	}
	if selector != "" {
		if x, y, err = center(ctx, p.target, selector); err != nil {
			return err
		}
	}
	m.cursor(ctx, p, x, y)
	if err := mouse(ctx, p.target, "mouseMoved", x, y, nil); err != nil {
		return err
	}
	clicks := 1
	if double {
		clicks = 2
	}
	for i := 1; i <= clicks; i++ {
		if err := mouse(ctx, p.target, "mousePressed", x, y, map[string]any{"clickCount": i}); err != nil {
			return err
		}
		if err := mouse(ctx, p.target, "mouseReleased", x, y, map[string]any{"clickCount": i}); err != nil {
			return err
		}
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// Type writes text where the focus is, or in the element of selector.
func (m *Manager) Type(ctx context.Context, threadID, tabID, text, selector string) error {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return err
	}
	if selector != "" {
		x, y, err := center(ctx, p.target, selector)
		if err != nil {
			return err
		}
		m.cursor(ctx, p, x, y)
		sel, _ := json.Marshal(selector)
		if err := evaluate(ctx, p.target, fmt.Sprintf(`document.querySelector(%s).focus()`, sel), nil); err != nil {
			return err
		}
	}
	_, err = p.target.Call(ctx, "Input.insertText", map[string]string{"text": text})
	return err
}

var keys = map[string]struct {
	code string
	num  int
}{
	"Enter": {"Enter", 13}, "Tab": {"Tab", 9}, "Escape": {"Escape", 27}, "Backspace": {"Backspace", 8}, "Delete": {"Delete", 46},
	"ArrowUp": {"ArrowUp", 38}, "ArrowDown": {"ArrowDown", 40}, "ArrowLeft": {"ArrowLeft", 37}, "ArrowRight": {"ArrowRight", 39},
	"Home": {"Home", 36}, "End": {"End", 35}, "PageUp": {"PageUp", 33}, "PageDown": {"PageDown", 34}, " ": {"Space", 32},
}

// Key presses a key with optional modifiers, such as "Enter" or "Control+a".
func (m *Manager) Key(ctx context.Context, threadID, tabID, combo string) error {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return err
	}
	parts := strings.Split(combo, "+")
	key := parts[len(parts)-1]
	if key == "" && strings.HasSuffix(combo, "+") {
		key = "+"
	}
	mods := 0
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(mod) {
		case "alt":
			mods |= 1
		case "ctrl", "control":
			mods |= 2
		case "meta", "cmd", "command":
			mods |= 4
		case "shift":
			mods |= 8
		}
	}
	if strings.EqualFold(key, "space") {
		key = " "
	}
	down := map[string]any{"type": "keyDown", "key": key, "modifiers": mods}
	if k, ok := keys[key]; ok {
		down["code"], down["windowsVirtualKeyCode"] = k.code, k.num
		if key == "Enter" {
			down["text"] = "\r"
		} else if key == " " {
			down["text"] = " "
		}
	} else if len([]rune(key)) == 1 {
		r := []rune(key)[0]
		up := strings.ToUpper(key)
		down["windowsVirtualKeyCode"] = int([]rune(up)[0])
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			down["code"] = "Key" + up
		}
		if mods&(2|4|1) == 0 {
			down["text"] = key
		}
	} else {
		return fmt.Errorf("tecla desconocida %q", key)
	}
	if _, err := p.target.Call(ctx, "Input.dispatchKeyEvent", down); err != nil {
		return err
	}
	upEv := map[string]any{"type": "keyUp", "key": key, "modifiers": mods, "code": down["code"], "windowsVirtualKeyCode": down["windowsVirtualKeyCode"]}
	_, err = p.target.Call(ctx, "Input.dispatchKeyEvent", upEv)
	return err
}

// Scroll scrolls by dx, dy pixels at x, y (the middle of the view by default).
func (m *Manager) Scroll(ctx context.Context, threadID, tabID string, x, y, dx, dy float64) error {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return err
	}
	if x == 0 && y == 0 {
		var vp [2]float64
		evaluate(ctx, p.target, "[innerWidth, innerHeight]", &vp)
		x, y = vp[0]/2, vp[1]/2
	}
	m.cursor(ctx, p, x, y)
	err = mouse(ctx, p.target, "mouseWheel", x, y, map[string]any{"deltaX": dx, "deltaY": dy})
	time.Sleep(200 * time.Millisecond)
	return err
}

// Resize sets the viewport of a headless page.
func (m *Manager) Resize(ctx context.Context, threadID, tabID string, width, height int) error {
	p, err := m.act(ctx, threadID, tabID)
	if err != nil {
		return err
	}
	if p.backend == "desktop" {
		return errors.New(i18n.Text("browser.pestana_muestra_app_escritorio"))
	}
	if width < 200 || height < 200 || width > 4000 || height > 4000 {
		return errors.New(i18n.Text("browser.tamano_debe_estar_entre"))
	}
	_, err = p.target.Call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": false})
	return err
}
