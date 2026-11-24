package runtime

import (
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/harness"
)

// ErrNoRequest means the request is not pending: it was answered already or
// its turn ended.
var ErrNoRequest = apperr.New(apperr.NoRequest, "the request is no longer pending")

type Session = harness.Session

type legacySession interface {
	Events() <-chan Event
	Answer(string, [][]string) error
}

func legacyStart(s legacySession, err error) (Session, error) {
	if err != nil {
		return nil, err
	}
	return harness.Adapt(s.Events(), s.Answer), nil
}

type Request = harness.Request
