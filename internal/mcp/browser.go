package mcp

import (
	"context"
	"errors"

	"github.com/migueel15/dory/internal/api"
)

// pngImage is a tool result shown to the agent as an image.
type pngImage []byte

type browserTab struct {
	TabID string `json:"tab_id,omitempty" jsonschema:"Tab to use; empty for the active tab"`
}

// addBrowser registers the tools of the thread's browser. They run on the
// node of the thread, whether the tab lives in the node's headless Chromium
// or in the desktop app where the user watches it.
func addBrowser(srv *sdkServer, s *api.Server) {
	thread := func(ctx context.Context) (string, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return "", errors.New("el navegador solo funciona dentro de un hilo de Dory")
		}
		if s.Browser == nil {
			return "", errors.New("este nodo no tiene navegador")
		}
		return id, nil
	}
	add(srv, "browser_open", "Open a URL in this thread's browser, in the active tab or a new one. The browser persists across turns and the user sees the same tabs in the thread's side panel. Returns the tab.", func(ctx context.Context, in struct {
		URL    string `json:"url"`
		NewTab bool   `json:"new_tab,omitempty" jsonschema:"Open in a new tab instead of the active one"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Open(ctx, id, in.URL, in.NewTab)
	})
	add(srv, "browser_tabs", "List the tabs of this thread's browser and which one is active.", func(ctx context.Context, _ struct{}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.State(ctx, id)
	})
	add(srv, "browser_activate", "Bring a tab of this thread's browser to the front.", func(ctx context.Context, in struct {
		TabID string `json:"tab_id"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Activate(ctx, id, in.TabID))
	})
	add(srv, "browser_close", "Close a tab of this thread's browser.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Close(ctx, id, in.TabID))
	})
	add(srv, "browser_navigate", "Navigate a tab to a URL, or back, forward or reload.", func(ctx context.Context, in struct {
		browserTab
		To string `json:"to" jsonschema:"A URL, or back, forward or reload"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Navigate(ctx, id, in.TabID, in.To)
	})
	add(srv, "browser_screenshot", "Capture a tab as a PNG image: the visible viewport, or the whole page.", func(ctx context.Context, in struct {
		browserTab
		FullPage bool `json:"full_page,omitempty"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		png, err := s.Browser.Screenshot(ctx, id, in.TabID, in.FullPage)
		return pngImage(png), err
	})
	add(srv, "browser_read", "Read a tab: URL, title, viewport, visible text and the interactive elements in view with the x, y of their center to click.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Read(ctx, id, in.TabID)
	})
	add(srv, "browser_console", "Read the last console messages and exceptions of a tab.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Console(ctx, id, in.TabID)
	})
	add(srv, "browser_click", "Click at x, y (CSS pixels of the viewport) or on the element a CSS selector matches.", func(ctx context.Context, in struct {
		browserTab
		X        float64 `json:"x,omitempty"`
		Y        float64 `json:"y,omitempty"`
		Selector string  `json:"selector,omitempty"`
		Double   bool    `json:"double,omitempty"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Click(ctx, id, in.TabID, in.X, in.Y, in.Selector, in.Double))
	})
	add(srv, "browser_type", "Type text where the focus is, or in the element a CSS selector matches.", func(ctx context.Context, in struct {
		browserTab
		Text     string `json:"text"`
		Selector string `json:"selector,omitempty"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Type(ctx, id, in.TabID, in.Text, in.Selector))
	})
	add(srv, "browser_key", "Press a key, optionally with modifiers: Enter, Tab, Escape, Backspace, ArrowDown, Control+a, Shift+Tab...", func(ctx context.Context, in struct {
		browserTab
		Key string `json:"key"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Key(ctx, id, in.TabID, in.Key))
	})
	add(srv, "browser_scroll", "Scroll a tab by dx, dy pixels at x, y (the middle of the viewport by default).", func(ctx context.Context, in struct {
		browserTab
		X  float64 `json:"x,omitempty"`
		Y  float64 `json:"y,omitempty"`
		DX float64 `json:"dx,omitempty"`
		DY float64 `json:"dy,omitempty" jsonschema:"Positive scrolls down"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Scroll(ctx, id, in.TabID, in.X, in.Y, in.DX, in.DY))
	})
	add(srv, "browser_resize", "Set the viewport size of a tab running on the node.", func(ctx context.Context, in struct {
		browserTab
		Width  int `json:"width"`
		Height int `json:"height"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Resize(ctx, id, in.TabID, in.Width, in.Height))
	})
}

func ok(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
