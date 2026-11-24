package harness

import (
	"strings"

	"github.com/migueel15/dory/internal/apperr"
)

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

type legacySession struct {
	events  <-chan Event
	respond func(string, [][]string) error
}

func (s legacySession) Events() <-chan Event { return s.events }
func (s legacySession) Respond(id string, answers [][]string) error {
	if s.respond == nil {
		return apperr.New(apperr.NoRequest, "the request is no longer pending")
	}
	return s.respond(id, answers)
}

func Adapt(events <-chan LegacyEvent, respond func(string, [][]string) error) Session {
	return legacySession{Upgrade(events), respond}
}

// Upgrade translates accumulated legacy snapshots to typed events in order.
func Upgrade(in <-chan LegacyEvent) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		texts := map[string]string{}
		tools := map[string]bool{}
		for e := range in {
			switch {
			case e.Message != nil:
				m := *e.Message
				meta := ItemMeta{m.RunID, m.CreatedAt}
				switch m.Role {
				case string(Assistant), string(Reasoning):
					kind := TextKind(m.Role)
					if !m.Streaming {
						delete(texts, m.ID)
						out <- TextDone{m.ID, kind, m.Text, meta}
						continue
					}
					delta, appendOnly := strings.CutPrefix(m.Text, texts[m.ID])
					texts[m.ID] = m.Text
					out <- TextDelta{m.ID, kind, delta, !appendOnly, meta}
				case "tool":
					update := ToolUpdated{m.ID, m.Tool, m.Title, m.Detail, m.Status, m.Text, m.InputChars, m.ResultChars, m.Streaming, meta}
					switch {
					case m.Status != "running" && !m.Streaming:
						delete(tools, m.ID)
						out <- ToolDone{m.ID, m.Status, m.Text, m.ResultChars, m.Tool, &update}
					case !tools[m.ID] && m.Tool != nil && m.Status == "running" && !m.Streaming && m.Text == "" && m.ResultChars == 0:
						tools[m.ID] = true
						out <- ToolStarted{m.ID, *m.Tool, m.Title, m.Detail, m.InputChars, meta}
					default:
						tools[m.ID] = true
						out <- update
					}
				case "plan":
					var todos []Todo
					if m.Tool != nil {
						todos = m.Tool.Todos
					}
					out <- PlanUpdated{ItemID: m.ID, Todos: todos, Text: m.Text, Title: m.Title, Detail: m.Detail, Call: m.Tool, Meta: meta}
				default:
					out <- MessageReported{m}
				}
			case e.Step != nil:
				out <- *e.Step
			case len(e.Steps) > 0:
				out <- StepsUsage{e.Steps}
			case e.Session != "":
				out <- SessionBound{e.Session}
			case e.Settings != nil:
				out <- SettingsReported(*e.Settings)
			case e.Usage != nil:
				out <- UsageReported{e.Usage}
			case e.Request != nil:
				out <- RequestOpened{*e.Request}
			case e.Done:
				reason := Completed
				switch e.Err {
				case "interrumpido":
					reason = Interrupted
				case "el agente terminó sin respuesta":
					reason = NoResponse
				case "":
				default:
					reason = Failed
				}
				out <- TurnEnded{reason, e.Err}
			}
		}
	}()
	return out
}

// ErrNoRequest indicates that a response no longer has a pending request.
var ErrNoRequest = apperr.New(apperr.NoRequest, "the request is no longer pending")
