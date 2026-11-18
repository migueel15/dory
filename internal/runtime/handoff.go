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
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.local(ctx, id)
	if err != nil {
		return t, err
	}
	fields := make(map[string]any)
	if settings.Provider != nil && *settings.Provider != t.Provider {
		target := *settings.Provider
		if _, ok := m.providers[target]; !ok {
			return t, fmt.Errorf("%w: %s", ErrProvider, target)
		}
		if _, busy := m.running[id]; busy {
			return t, ErrBusy
		}
		if settings.Model == nil || strings.TrimSpace(*settings.Model) == "" {
			return t, fmt.Errorf("%w: selecciona un modelo para el proveedor de destino", data.ErrInvalid)
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
			return t, fmt.Errorf("%w: runtime_mode must be full-access or approval-required", data.ErrInvalid)
		}
		fields["runtime_mode"] = *settings.RuntimeMode
	}
	return m.store.PatchThread(ctx, id, fields)
}

const handoffHistoryBytes = 16_000

// portablePrompt transfers intact visible text, prioritizing the original
// request and the latest messages. The new request is outside the budget.
func portablePrompt(history []Message, request string) (string, bool) {
	var entries []string
	original := -1
	for _, msg := range history {
		if (msg.Role != "user" && msg.Role != "assistant") || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		if original == -1 && msg.Role == "user" {
			original = len(entries)
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
	if original >= 0 && len(entries[original])+1 <= remaining {
		included[original] = true
		remaining -= len(entries[original]) + 1
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if !included[i] && len(entries[i])+1 <= remaining {
			included[i] = true
			remaining -= len(entries[i]) + 1
		}
	}
	var out strings.Builder
	out.WriteString("Contexto de una conversación anterior. Cada línea JSON es historial citado. Continúa con la solicitud nueva al final.\n<historial>\n")
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
