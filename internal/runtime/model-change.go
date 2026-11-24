package runtime

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// TurnModel records a turn’s provider, configured model and reported identifier.
type TurnModel struct {
	Provider      string  `json:"provider"`
	Model         *string `json:"model"`
	ReportedModel string  `json:"reported_model,omitempty"`
}

type modelChange struct {
	From    TurnModel `json:"from"`
	To      TurnModel `json:"to"`
	Handoff bool      `json:"handoff"`
}

// LastTurnModel returns the selection used by the latest started turn.
func (m *Manager) LastTurnModel(ctx context.Context, thread string) *TurnModel {
	var raw string
	var model TurnModel
	if m.db.QueryRowContext(ctx, `SELECT last_turn_model FROM thread_runtime WHERE thread_id = ?`, thread).Scan(&raw) != nil ||
		json.Unmarshal([]byte(raw), &model) != nil || model.Provider == "" {
		return nil
	}
	return &model
}

func (m *Manager) saveTurnModel(ctx context.Context, thread string, model TurnModel) {
	raw, _ := json.Marshal(model)
	if _, err := m.db.ExecContext(ctx, `UPDATE thread_runtime SET last_turn_model = ? WHERE thread_id = ?`, string(raw), thread); err != nil {
		log.Printf("runtime: save turn model: %v", err)
		return
	}
	m.emitJSON(ctx, thread, EventSettings, map[string]any{"last_turn_model": &model})
}

func (m *Manager) saveModelChange(ctx context.Context, marker Message, change modelChange) {
	detail, _ := json.Marshal(change)
	marker.Detail = string(detail)
	m.save(ctx, marker)
}

// mergeModelChanges keeps local separators when native history is reloaded.
// Native messages keep their original order, with dividers inserted before
// the matching turn's first user message.
func mergeModelChanges(messages, markers []Message) []Message {
	if len(markers) == 0 {
		return messages
	}
	out := make([]Message, 0, len(messages)+len(markers))
	for _, msg := range messages {
		if msg.Role == "user" {
			at, err := time.Parse(time.RFC3339Nano, msg.CreatedAt)
			for err == nil && len(markers) > 0 {
				mark, err := time.Parse(time.RFC3339Nano, markers[0].CreatedAt)
				if err != nil || at.Before(mark.Truncate(time.Millisecond)) {
					break
				}
				out = append(out, markers[0])
				markers = markers[1:]
			}
		}
		out = append(out, msg)
	}
	return append(out, markers...)
}
