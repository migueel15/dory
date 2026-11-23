package runtime

import (
	"context"
	"errors"
)

// ErrNoRequest means the request is not pending: it was answered already or
// its turn ended.
var ErrNoRequest = errors.New("la petición ya no está pendiente")

// Runner is a provider that runs each turn as one process that cannot be
// spoken to while it works.
type Runner interface {
	// Run starts a turn and streams its events until a Done event; the
	// channel is closed afterwards. Cancelling ctx interrupts the turn.
	Run(ctx context.Context, t Turn) (<-chan Event, error)
}

// Interactive is a provider whose turn keeps the harness process open, so
// the agent can stop and wait for the user in the middle of it.
type Interactive interface {
	// Start runs a turn as a session. Cancelling ctx interrupts it.
	Start(ctx context.Context, t Turn) (Session, error)
}

// Session is a running turn of an interactive harness.
type Session interface {
	// Events streams the turn until a Done event; then the channel closes.
	Events() <-chan Event
	// Answer resolves a pending request of the agent. answers[i] holds the
	// chosen labels or free text for request question i.
	Answer(requestID string, answers [][]string) error
}

// Request is something the agent asks in the middle of a turn; the turn
// waits until the user answers it.
type Request struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id,omitempty"`
	// Kind is question or secret; permission will follow.
	Kind string `json:"kind"`
	// MessageID is the tool message that shows the request in the chat.
	MessageID string     `json:"message_id,omitempty"`
	Questions []Question `json:"questions"`
	// Secret is the form of a secret request (kind secret), without values.
	Secret    *SecretPrompt `json:"secret,omitempty"`
	CreatedAt string        `json:"created_at"`
}
