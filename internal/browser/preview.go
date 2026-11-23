package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Preview is what html_preview returns: a capture of a page laid out at a
// width, its content height and what it wrote to the console.
type Preview struct {
	PNG            []byte
	ContentHeight  int
	CapturedHeight int
	Console        []string
}

// maxPreviewHeight bounds the capture; a taller page reports its full height.
const maxPreviewHeight = 4000

// Preview lays doc out in a hidden page of the node's Chromium at width CSS
// pixels and captures it. The page is not a tab of any thread.
func (m *Manager) Preview(ctx context.Context, doc string, width int) (Preview, error) {
	m.mu.Lock()
	m.init()
	ch := m.chrome
	m.mu.Unlock()
	if ch == nil || !ch.alive() {
		var err error
		m.openMu.Lock()
		ch, err = startChrome(m.Profile, m.browserEvent)
		if err == nil {
			m.mu.Lock()
			m.chrome = ch
			m.mu.Unlock()
		}
		m.openMu.Unlock()
		if err != nil {
			return Preview{}, err
		}
	}
	var (
		mu      sync.Mutex
		console []string
	)
	logLine := func(s string) {
		mu.Lock()
		if len(console) < 50 {
			console = append(console, s)
		}
		mu.Unlock()
	}
	s, err := ch.newPage(ctx, "", "", func(method string, params json.RawMessage) {
		switch method {
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
				logLine(ev.Type + ": " + strings.Join(parts, " "))
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
				logLine("error: " + ev.ExceptionDetails.Text + " " + ev.ExceptionDetails.Exception.Description)
			}
		}
	})
	if err != nil {
		return Preview{}, err
	}
	defer s.Close()
	metrics := func(h int) error {
		_, err := s.Call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": h, "deviceScaleFactor": 1, "mobile": width < 500})
		return err
	}
	if err := setup(ctx, s, false); err != nil {
		return Preview{}, err
	}
	if err := metrics(800); err != nil {
		return Preview{}, err
	}
	raw, err := s.Call(ctx, "Page.getFrameTree", nil)
	if err != nil {
		return Preview{}, err
	}
	var tree struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	json.Unmarshal(raw, &tree)
	if _, err := s.Call(ctx, "Page.setDocumentContent", map[string]any{"frameId": tree.FrameTree.Frame.ID, "html": doc}); err != nil {
		return Preview{}, err
	}
	// Let scripts, fonts and remote assets settle before measuring.
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	evaluate(wait, s, `new Promise(r=>{const d=()=>document.fonts.ready.then(()=>setTimeout(r,300));document.readyState==="complete"?d():addEventListener("load",d)})`, nil)
	cancel()
	var height float64
	if err := evaluate(ctx, s, `Math.ceil(document.documentElement.getBoundingClientRect().height)`, &height); err != nil {
		return Preview{}, err
	}
	p := Preview{ContentHeight: int(height), CapturedHeight: min(max(int(height), 1), maxPreviewHeight)}
	if err := metrics(p.CapturedHeight); err != nil {
		return Preview{}, err
	}
	raw, err = s.Call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return Preview{}, err
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &shot); err != nil {
		return Preview{}, err
	}
	if p.PNG, err = base64.StdEncoding.DecodeString(shot.Data); err != nil {
		return Preview{}, err
	}
	mu.Lock()
	p.Console = append([]string(nil), console...)
	mu.Unlock()
	return p, nil
}
