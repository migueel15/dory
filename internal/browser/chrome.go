package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ErrNoChromium tells the agent that this node cannot run a browser.
var ErrNoChromium = apperr.New(apperr.NoChromium, "no Chromium on this node: install chromium (or google-chrome) or set DORY_CHROMIUM to its path")

var devtoolsLine = regexp.MustCompile(`DevTools listening on (ws://\S+)`)

// findChromium returns the browser binary of this node.
func findChromium() (string, error) {
	if p := os.Getenv("DORY_CHROMIUM"); p != "" {
		return p, nil
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", ErrNoChromium
}

// chrome is the headless Chromium of the node. It starts on demand with a
// persistent profile, so cookies are shared by every thread and survive.
type chrome struct {
	cmd  *exec.Cmd
	conn *conn
}

func startChrome(profile string, onEvent EventFunc) (*chrome, error) {
	bin, err := findChromium()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, err
	}
	// A cold start on a loaded machine can stall once; a second launch with
	// the same profile usually starts at once.
	cmd, url, err := launchChrome(bin, profile)
	if err != nil {
		cmd, url, err = launchChrome(bin, profile)
	}
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := dialCDP(ctx, url)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	c.on("", onEvent)
	go func() { cmd.Wait(); c.close() }()
	if _, err := c.call(ctx, "", "Target.setDiscoverTargets", map[string]bool{"discover": true}); err != nil {
		c.close()
		cmd.Process.Kill()
		return nil, err
	}
	return &chrome{cmd: cmd, conn: c}, nil
}

// launchChrome starts Chromium and waits for its DevTools URL. On failure the
// error carries the last lines Chromium wrote, which explain the cause.
func launchChrome(bin, profile string) (*exec.Cmd, string, error) {
	cmd := exec.Command(bin, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--disable-gpu", "--hide-scrollbars", "--mute-audio",
		"--disable-background-timer-throttling", "--disable-renderer-backgrounding", "--window-size=1280,800", "about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", err
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("arrancar Chromium: %w", err)
	}
	found := make(chan string, 1)
	var mu sync.Mutex
	var tail []string
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			if m := devtoolsLine.FindStringSubmatch(line); m != nil {
				select {
				case found <- m[1]:
				default:
				}
				continue
			}
			mu.Lock()
			if tail = append(tail, line); len(tail) > 5 {
				tail = tail[1:]
			}
			mu.Unlock()
		}
	}()
	select {
	case url := <-found:
		return cmd, url, nil
	case <-time.After(20 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		mu.Lock()
		defer mu.Unlock()
		return nil, "", fmt.Errorf("Chromium no abrió el puerto de depuración: %s", strings.Join(tail, " | "))
	}
}

func (ch *chrome) alive() bool {
	select {
	case <-ch.conn.done:
		return false
	default:
		return true
	}
}

// newPage opens a page, or attaches to targetID when the page already exists
// (a window opened by another page).
func (ch *chrome) newPage(ctx context.Context, url, targetID string, onEvent EventFunc) (*session, error) {
	if targetID == "" {
		raw, err := ch.conn.call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
		if err != nil {
			return nil, err
		}
		var r struct {
			TargetID string `json:"targetId"`
		}
		json.Unmarshal(raw, &r)
		targetID = r.TargetID
	}
	raw, err := ch.conn.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(raw, &r)
	s := &session{c: ch.conn, id: r.SessionID, targetID: targetID}
	ch.conn.on(s.id, onEvent)
	if url != "" {
		if _, err := s.Call(ctx, "Page.navigate", map[string]string{"url": url}); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (ch *chrome) stop() {
	ch.conn.close()
	if ch.cmd.Process != nil {
		ch.cmd.Process.Kill()
	}
}
