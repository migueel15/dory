package runtime

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"slices"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/workspace"
)

// turnOutcome is how a turn ended, for the actor.
type turnOutcome struct {
	status, failure string
	// interrupted says the turn's context was cancelled.
	interrupted bool
}

// record stores what a turn emits until it ends: messages, usage, the diff of
// the turn and the branch. It touches no state of the actor; opened tells the
// actor about a request of the harness.
func (m *Manager) record(ctx context.Context, turn startedTurn, opened func(harness.Request)) turnOutcome {
	t, marker, change, usage, run, root, base := turn.thread, turn.marker, turn.change, turn.usage, turn.run, turn.root, turn.base
	events := turn.session.Events()
	bg := context.WithoutCancel(ctx)
	status, failure := "idle", ""
	settings := turnSettings{Provider: t.Provider, Configured: ReportedSettings{Model: t.Model, Effort: t.Effort}}
	messages := turnMessages{run: run, items: map[string]Message{}}
	checkpoints := make(map[string]textCheckpoint)
	flush := func() {
		for _, id := range slices.Sorted(maps.Keys(checkpoints)) {
			msg := messages.items[id]
			msg.ThreadID, msg.Streaming = t.ID, false
			m.save(bg, msg)
			delete(checkpoints, id)
		}
	}
	m.emit(bg, ThreadEvent{ThreadID: t.ID, RunID: run, Type: EventTurnStarted, Payload: json.RawMessage(`{}`)})
	for event := range events {
		if msg, ok := messages.apply(event); ok {
			msg.ThreadID = t.ID
			usage.message(msg)
			switch e := event.(type) {
			case harness.TextDelta:
				at := time.Now()
				last, exists := checkpoints[msg.ID]
				if !exists || last.due(at, len(msg.Text)) {
					typ := ""
					if !exists {
						typ = EventItemStarted
					}
					if m.persist(bg, &msg, typ, "") {
						checkpoints[msg.ID] = textCheckpoint{at: at, size: len(msg.Text)}
					}
				}
				if _, exists := checkpoints[msg.ID]; !exists {
					checkpoints[msg.ID] = textCheckpoint{at: at}
				}
				m.publish(Update{ThreadID: t.ID, Message: &msg})
				m.liveText(msg, e.Delta, e.Replace)
			default:
				m.doneWriting(t.ID, msg.ID)
				delete(checkpoints, msg.ID)
				m.save(bg, msg)
			}
			continue
		}
		switch e := event.(type) {
		case harness.StepUsage:
			usage.step(e)
		case harness.StepsUsage:
			usage.batch(e.Steps)
		case harness.SessionBound:
			if _, err := m.db.ExecContext(bg, `UPDATE thread_runtime SET session_id = ?, session_provider = ? WHERE thread_id = ?`, e.ID, t.Provider, t.ID); err != nil {
				log.Printf("runtime: save session: %v", err)
			} else {
				m.emitJSON(bg, t.ID, EventSessionBound, map[string]string{"provider": t.Provider, "session_id": e.ID})
			}
		case harness.SettingsReported:
			previous := settings.Reported
			if e.Model != "" {
				settings.Reported.Model = e.Model
				if change.To.ReportedModel != e.Model {
					change.To.ReportedModel = e.Model
					m.saveTurnModel(bg, t.ID, change.To)
					if marker.ID != "" {
						m.saveModelChange(bg, marker, change)
					}
				}
			}
			if e.Effort != "" {
				settings.Reported.Effort = e.Effort
			}
			if settings.Reported != previous {
				raw, _ := json.Marshal(settings)
				if _, err := m.db.ExecContext(bg, `UPDATE thread_runtime SET reported_settings = ? WHERE thread_id = ?`, string(raw), t.ID); err != nil {
					log.Printf("runtime: save reported settings: %v", err)
				} else {
					m.emitJSON(bg, t.ID, EventSettings, settings)
				}
			}
		case harness.UsageReported:
			m.saveUsage(bg, t.Provider, e.Windows)
		case harness.RequestOpened:
			m.request(bg, t.ID, e.Request)
			opened(e.Request)
		case harness.TurnEnded:
			flush()
			m.emitJSON(bg, t.ID, EventTurnEnded, map[string]any{"run_id": run, "reason": e.Reason})
			switch {
			case e.Reason == harness.Interrupted || (e.Reason != harness.Completed && ctx.Err() != nil):
				status, failure = "idle", ""
				m.save(bg, Message{ID: newRunID(), ThreadID: t.ID, Role: "interrupt", Code: "turn.interrupted", CreatedAt: now()})
			case e.Reason == harness.Failed || e.Reason == harness.NoResponse:
				status, failure = "error", turnError(e)
				m.save(bg, Message{ID: newRunID(), ThreadID: t.ID, Role: "system", Code: turnFailureCode(e), Text: turnFailureText(e), Status: "failed", CreatedAt: now()})
			}
		}
	}

	flush()
	m.doneWriting(t.ID, "")
	if err := usage.save(bg, m.db, t.ID, run, t.Provider, time.Now()); err != nil {
		log.Printf("runtime: save usage: %v", err)
	}
	// The process is gone: nothing can answer its requests any more.
	if _, err := m.db.ExecContext(bg, `DELETE FROM thread_requests WHERE thread_id = ?`, t.ID); err != nil {
		log.Printf("runtime: drop requests: %v", err)
	}
	if files := workspace.TreeChanges(bg, root, base, workspace.Snapshot(bg, root)); len(files) > 0 {
		m.save(bg, Message{ID: run + ":changes", ThreadID: t.ID, RunID: run, Role: "changes", Tool: &ToolCall{Kind: "changes", Name: "changes", Files: files}, CreatedAt: now()})
	}
	m.syncBranch(bg, t.ID)
	return turnOutcome{status: status, failure: failure}
}
