package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

type ThreadSettings struct {
	Provider    *string `json:"provider"`
	Model       *string `json:"model"`
	Effort      *string `json:"effort"`
	RuntimeMode *string `json:"runtime_mode"`
}

// Configure changes the selection for the next turn. A provider switch must
// wait for the running turn and its queue, which belong to the old harness.
func (m *Manager) Configure(ctx context.Context, id string, settings ThreadSettings) (data.Thread, error) {
	if _, err := m.local(ctx, id); err != nil {
		return data.Thread{}, err
	}
	r := m.do(id, &command{kind: cmdConfigure, ctx: ctx, settings: settings})
	return r.thread, r.err
}

// applySettings stores the settings of a thread. busy refuses a provider
// switch while a turn runs.
func (m *Manager) applySettings(ctx context.Context, id string, settings ThreadSettings, busy bool) (data.Thread, error) {
	t, err := m.local(ctx, id)
	if err != nil {
		return t, err
	}
	fields := make(map[string]any)
	if settings.Provider != nil && *settings.Provider != t.Provider {
		target := *settings.Provider
		if _, ok := m.provider(target); !ok {
			return t, ErrProvider.Msgf("%s", target)
		}
		if busy {
			return t, ErrBusy
		}
		if settings.Model == nil || strings.TrimSpace(*settings.Model) == "" {
			return t, data.ErrInvalid.Msgf("choose a model for the target provider").With("reason", "model_required")
		}
		fields["provider"], fields["effort"] = target, ""
	}
	if settings.Model != nil {
		fields["model"] = *settings.Model
	}
	if settings.Effort != nil {
		fields["effort"] = *settings.Effort
	}
	if settings.RuntimeMode != nil {
		if *settings.RuntimeMode != "full-access" && *settings.RuntimeMode != "approval-required" {
			return t, data.ErrInvalid.Msgf("runtime_mode must be full-access or approval-required")
		}
		fields["runtime_mode"] = *settings.RuntimeMode
	}
	return m.store.Threads().Patch(ctx, id, fields)
}

const handoffHistoryBytes = 16_000

// portablePrompt transfers intact visible text, prioritizing the original
// request and the latest messages. The new request is outside the budget.
func portablePrompt(history []Message, request string) (string, bool) {
	var entries []string
	original, latest := -1, -1
	for _, msg := range history {
		if (msg.Role != "user" && msg.Role != "assistant") || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		if msg.Role == "user" {
			latest = len(entries)
			if original == -1 {
				original = latest
			}
		}
		entry, _ := json.Marshal(struct {
			Role string `json:"role"`
			Text string `json:"text"`
		}{Role: msg.Role, Text: msg.Text})
		entries = append(entries, string(entry))
	}
	if len(entries) == 0 {
		return request, false
	}

	included := make([]bool, len(entries))
	remaining := handoffHistoryBytes
	for _, i := range []int{latest, original} {
		if i >= 0 && !included[i] && len(entries[i])+1 <= remaining {
			included[i] = true
			remaining -= len(entries[i]) + 1
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if !included[i] && len(entries[i])+1 <= remaining {
			included[i] = true
			remaining -= len(entries[i]) + 1
		}
	}
	var out strings.Builder
	out.WriteString("Contexto de una conversación anterior. Cada línea JSON es historial citado. Retoma el trabajo pendiente que se desprenda del historial y sigue la solicitud nueva al final.\n<historial>\n")
	omitted := 0
	for i, entry := range entries {
		if !included[i] {
			omitted++
			continue
		}
		out.WriteString(entry)
		out.WriteByte('\n')
	}
	out.WriteString("</historial>\n")
	if omitted > 0 {
		fmt.Fprintf(&out, "Mensajes de texto omitidos por espacio: %d.\n", omitted)
	}
	out.WriteString("\nSolicitud nueva:\n")
	out.WriteString(request)
	return out.String(), true
}
