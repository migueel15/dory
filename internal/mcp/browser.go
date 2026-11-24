package mcp

import (
	"github.com/migueel15/dory/internal/apperr"
	"context"

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
			return "", apperr.New(apperr.Invalid, "the browser only works inside a Dory thread")
		}
		if s.Browser == nil {
			return "", apperr.New(apperr.Unavailable, "this node has no browser")
		}
		return id, nil
	}
	add(srv, "browser_open", "Open a URL in this thread's browser and show it in the side panel, e.g. to show a mockup or test a page.", func(ctx context.Context, in struct {
		URL    string `json:"url"`
		NewTab bool   `json:"new_tab,omitempty" jsonschema:"Open in a new tab instead of the active one"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		tab, err := s.Browser.Open(ctx, id, in.URL, in.NewTab)
		if err != nil {
			return nil, err
		}
		if s.Events != nil {
			s.Events.Notify(api.Change{Entity: "browser", ID: id, Data: map[string]string{"action": "open"}})
		}
		return tab, nil
	})
	add(srv, "browser_tabs", "List this thread's browser tabs and the active one.", func(ctx context.Context, _ struct{}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.State(ctx, id)
	})
	add(srv, "browser_activate", "Bring a browser tab to the front.", func(ctx context.Context, in struct {
		TabID string `json:"tab_id"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Activate(ctx, id, in.TabID))
	})
	add(srv, "browser_close", "Close a browser tab.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Close(ctx, id, in.TabID))
	})
	add(srv, "browser_navigate", "Go to a URL, back, forward or reload in a tab.", func(ctx context.Context, in struct {
		browserTab
		To string `json:"to" jsonschema:"A URL, or back, forward or reload"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Navigate(ctx, id, in.TabID, in.To)
	})
	add(srv, "browser_screenshot", "Capture a tab as PNG, the viewport or the whole page.", func(ctx context.Context, in struct {
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
	add(srv, "browser_read", "Read a tab as text: URL, title, visible text and clickable elements with their x, y. Prefer it to a screenshot.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Read(ctx, id, in.TabID)
	})
	add(srv, "browser_console", "Read a tab's last console messages and exceptions.", func(ctx context.Context, in browserTab) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Browser.Console(ctx, id, in.TabID)
	})
	add(srv, "browser_click", "Click at x, y or on the element a CSS selector matches.", func(ctx context.Context, in struct {
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
	add(srv, "browser_type", "Type text into the focus or the element a CSS selector matches.", func(ctx context.Context, in struct {
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
	add(srv, "browser_key", "Press a key, with modifiers: Enter, Tab, Escape, Control+a…", func(ctx context.Context, in struct {
		browserTab
		Key string `json:"key"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return ok(s.Browser.Key(ctx, id, in.TabID, in.Key))
	})
	add(srv, "browser_scroll", "Scroll a tab by dx, dy.", func(ctx context.Context, in struct {
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
	add(srv, "browser_resize", "Set a tab's viewport size.", func(ctx context.Context, in struct {
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
