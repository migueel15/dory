package runtime

import (
	"github.com/migueel15/dory/internal/harness"
)

// ErrNoRequest means the request is not pending: it was answered already or
// its turn ended.
var ErrNoRequest = harness.ErrNoRequest

type Session = harness.Session

type Request = harness.Request
