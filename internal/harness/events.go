package harness

import "github.com/migueel15/dory/internal/apperr"

// Event is a change in the state of a turn.
type Event interface{ isEvent() }

type TextKind string

const (
	Assistant TextKind = "assistant"
	Reasoning TextKind = "reasoning"
)

// ItemMeta preserves adapter item identity and timestamps during translation.
type ItemMeta struct {
	RunID, CreatedAt string
}

type TextDelta struct {
	ItemID string
	Kind   TextKind
	Delta  string
	// Replace handles legacy snapshots that revise already emitted text.
	Replace bool
	Meta    ItemMeta
}
type TextDone struct {
	ItemID string
	Kind   TextKind
	Text   string
	Meta   ItemMeta
}

type ToolStarted struct {
	ItemID        string
	Call          ToolCall
	Title, Detail string
	InputChars    int
	Meta          ItemMeta
}
type ToolUpdated struct {
	ItemID                        string
	Call                          *ToolCall
	Title, Detail, Status, Result string
	InputChars, ResultChars       int
	Streaming                     bool
	Meta                          ItemMeta
}
type ToolDone struct {
	ItemID, Status, Result string
	ResultChars            int
	Call                   *ToolCall
	// Update carries metadata from a final legacy snapshot, including tools
	// that emit their first and only message at completion.
	Update *ToolUpdated
}
type PlanUpdated struct {
	ItemID              string
	Todos               []Todo
	Text, Title, Detail string
	Call                *ToolCall
	Meta                ItemMeta
}

type RequestOpened struct{ Request Request }
type SessionBound struct{ ID string }
type SettingsReported ReportedSettings
type UsageReported struct{ Windows []Window }
type StepsUsage struct{ Steps []StepUsage }

// MessageReported carries non-streaming conversation roles such as a history
// message replayed by the adapter when it attaches to an existing session.
type MessageReported struct{ Message Message }

type EndReason string

const (
	Completed   EndReason = "completed"
	Interrupted EndReason = "interrupted"
	Failed      EndReason = "failed"
	NoResponse  EndReason = "no_response"
)

type TurnEnded struct {
	Reason EndReason
	Error  string
}

func (TextDelta) isEvent()        {}
func (TextDone) isEvent()         {}
func (ToolStarted) isEvent()      {}
func (ToolUpdated) isEvent()      {}
func (ToolDone) isEvent()         {}
func (PlanUpdated) isEvent()      {}
func (RequestOpened) isEvent()    {}
func (SessionBound) isEvent()     {}
func (SettingsReported) isEvent() {}
func (UsageReported) isEvent()    {}
func (StepUsage) isEvent()        {}
func (StepsUsage) isEvent()       {}
func (MessageReported) isEvent()  {}
func (TurnEnded) isEvent()        {}

// ErrNoRequest indicates that a response no longer has a pending request.
var ErrNoRequest = apperr.New(apperr.NoRequest, "the request is no longer pending")
