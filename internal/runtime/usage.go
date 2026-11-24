package runtime

import (
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

var ErrNoUsage = harness.ErrNoUsage

func sortWindows(ws []Window) []Window { return kit.SortWindows(ws) }
