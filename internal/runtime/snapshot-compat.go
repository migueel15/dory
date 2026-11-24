package runtime

import (
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

// Event is the temporary snapshot format of the Antigravity adapter.
type Event struct {
	Message  *Message
	Settings *ReportedSettings
	// Session is the harness id of the conversation, used to resume it.
	Session string
	Usage   []Window
	// Step is the usage of one model call of the turn.
	Step  *StepUsage
	Steps []StepUsage
	// Request pauses the turn until the user answers it.
	Request *Request
	// Done ends the turn; Err is set when it failed.
	Done bool
	Err  string
}

type antigravityEvents struct {
	events  <-chan harness.Event
	respond func(string, [][]string) error
}

func (s antigravityEvents) Events() <-chan harness.Event { return s.events }
func (s antigravityEvents) Respond(id string, answers [][]string) error {
	if s.respond == nil {
		return ErrNoRequest
	}
	return s.respond(id, answers)
}

func adaptAntigravity(events <-chan Event, respond func(string, [][]string) error) Session {
	return antigravityEvents{upgradeAntigravity(events), respond}
}

// upgradeAntigravity bridges the snapshot protocol until Antigravity moves to ACP.
func upgradeAntigravity(in <-chan Event) <-chan harness.Event {
	out := make(chan harness.Event, 64)
	go func() {
		defer close(out)
		texts := map[string]string{}
		tools := map[string]bool{}
		for e := range in {
			switch {
			case e.Message != nil:
				m := *e.Message
				meta := harness.ItemMeta{m.RunID, m.CreatedAt}
				switch m.Role {
				case string(harness.Assistant), string(harness.Reasoning):
					kind := harness.TextKind(m.Role)
					if !m.Streaming {
						delete(texts, m.ID)
						out <- harness.TextDone{m.ID, kind, m.Text, meta}
						continue
					}
					delta, appendOnly := strings.CutPrefix(m.Text, texts[m.ID])
					texts[m.ID] = m.Text
					out <- harness.TextDelta{m.ID, kind, delta, !appendOnly, meta}
				case "tool":
					update := harness.ToolUpdated{m.ID, m.Tool, m.Title, m.Detail, m.Status, m.Text, m.InputChars, m.ResultChars, m.Streaming, meta}
					switch {
					case m.Status != "running" && !m.Streaming:
						delete(tools, m.ID)
						out <- harness.ToolDone{m.ID, m.Status, m.Text, m.ResultChars, m.Tool, &update}
					case !tools[m.ID] && m.Tool != nil && m.Status == "running" && !m.Streaming && m.Text == "" && m.ResultChars == 0:
						tools[m.ID] = true
						out <- harness.ToolStarted{m.ID, *m.Tool, m.Title, m.Detail, m.InputChars, meta}
					default:
						tools[m.ID] = true
						out <- update
					}
				case "plan":
					var todos []Todo
					if m.Tool != nil {
						todos = m.Tool.Todos
					}
					out <- harness.PlanUpdated{ItemID: m.ID, Todos: todos, Text: m.Text, Title: m.Title, Detail: m.Detail, Call: m.Tool, Meta: meta}
				default:
					out <- harness.MessageReported{m}
				}
			case e.Step != nil:
				out <- *e.Step
			case len(e.Steps) > 0:
				out <- harness.StepsUsage{e.Steps}
			case e.Session != "":
				out <- harness.SessionBound{e.Session}
			case e.Settings != nil:
				out <- harness.SettingsReported(*e.Settings)
			case e.Usage != nil:
				out <- harness.UsageReported{e.Usage}
			case e.Request != nil:
				out <- harness.RequestOpened{*e.Request}
			case e.Done:
				reason := harness.Completed
				switch e.Err {
				case interrupted:
					reason = harness.Interrupted
				case noResponse:
					reason = harness.NoResponse
				case "":
				default:
					reason = harness.Failed
				}
				out <- harness.TurnEnded{reason, e.Err}
			}
		}
	}()
	return out
}
