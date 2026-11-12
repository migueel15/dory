package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Gap is a period in which the heartbeat goroutine did not run.
type Gap struct {
	At      time.Time `json:"at"`
	Seconds float64   `json:"seconds"`
}

// Heartbeat measures whether Go keeps running in the background. A goroutine
// wakes every Interval; if it wakes much later than expected the process was
// frozen/throttled (Doze, app standby, cached-process freezer).
type Heartbeat struct {
	Interval time.Duration
	Tolerate time.Duration

	mu      sync.Mutex
	started time.Time
	last    time.Time
	beats   int
	maxGap  time.Duration
	gaps    []Gap
	logPath string
	stop    chan struct{}
}

func NewHeartbeat(env *Env) *Heartbeat {
	return &Heartbeat{Interval: 5 * time.Second, Tolerate: 15 * time.Second,
		logPath: filepath.Join(env.DataDir, "heartbeat.log")}
}

func (h *Heartbeat) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stop != nil {
		return
	}
	h.started = time.Now()
	h.last = h.started
	h.stop = make(chan struct{})
	go h.loop(h.stop)
}

func (h *Heartbeat) loop(stop chan struct{}) {
	t := time.NewTicker(h.Interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			now = time.Now() // t.C carries the scheduled time; we want the real wake-up
			h.mu.Lock()
			gap := now.Sub(h.last)
			h.last = now
			h.beats++
			if gap > h.maxGap {
				h.maxGap = gap
			}
			if gap > h.Tolerate {
				h.gaps = append(h.gaps, Gap{At: now, Seconds: gap.Seconds()})
				if f, err := os.OpenFile(h.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
					fmt.Fprintf(f, "%s gap=%.1fs\n", now.Format(time.RFC3339), gap.Seconds())
					f.Close()
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Heartbeat) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
}

// Snapshot is the JSON shown by the page.
type Snapshot struct {
	Running     bool    `json:"running"`
	UptimeSec   float64 `json:"uptimeSec"`
	Beats       int     `json:"beats"`
	MaxGapSec   float64 `json:"maxGapSec"`
	Gaps        []Gap   `json:"gaps"`
	TolerateSec float64 `json:"tolerateSec"`
}

func (h *Heartbeat) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := Snapshot{Running: h.stop != nil, Beats: h.beats, MaxGapSec: h.maxGap.Seconds(),
		Gaps: append([]Gap{}, h.gaps...), TolerateSec: h.Tolerate.Seconds()}
	if !h.started.IsZero() {
		s.UptimeSec = time.Since(h.started).Seconds()
	}
	return s
}

// Verdict turns a snapshot into the check result: OK when the process never
// stalled for longer than Tolerate since Start.
func (h *Heartbeat) Verdict() (string, error) {
	s := h.Snapshot()
	if !s.Running {
		return "", fmt.Errorf("heartbeat not running")
	}
	msg := fmt.Sprintf("%d beats in %.0fs, max gap %.1fs", s.Beats, s.UptimeSec, s.MaxGapSec)
	if len(s.Gaps) > 0 {
		return "", fmt.Errorf("%s; process was frozen %d time(s), first at %s for %.0fs",
			msg, len(s.Gaps), s.Gaps[0].At.Format("15:04:05"), s.Gaps[0].Seconds)
	}
	if s.UptimeSec < 120 {
		return msg + " (too short: leave the app in background >= 5 min, then come back)", nil
	}
	return msg, nil
}
