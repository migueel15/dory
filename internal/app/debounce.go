package app

import (
	"sync"
	"time"
)

// debounce runs a function once after a quiet period: calls that arrive
// before it runs push it back and fold into that one run.
type debounce struct {
	mu    sync.Mutex
	timer *time.Timer
}

func (d *debounce) call(delay time.Duration, f func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(delay, f)
}
