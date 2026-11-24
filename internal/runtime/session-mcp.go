package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
)

// legacyDoryInstructions was prepended to user messages before the guidance
// moved to the system prompt; history recovery still strips it.
const legacyDoryInstructions = `Estás en un hilo gestionado por Dory. Usa el servidor MCP dory para consultar el contexto del proyecto (session_start), gestionar tareas y operar sobre este hilo. Descubre las herramientas disponibles en el catálogo MCP del arnés y sigue sus esquemas. Para mostrar un mockup web en el panel del hilo, abre su URL con browser_open y comprueba la pestaña activa con browser_tabs. Para confirmar cambios de tareas o planes, consulta get_task o plan_read.`

// prepareMCP uses the same connection and guidance for new and resumed turns.
// The guidance, built from the tools the catalog offers, travels in Turn.Instructions; each provider delivers it once per
// session through its system channel, so user messages stay clean.
func (m *Manager) prepareMCP(ctx context.Context, thread data.Thread, turn *Turn) error {
	provider := thread.Provider
	if m.MCPURL == "" {
		return nil
	}
	endpoint, err := url.Parse(m.MCPURL)
	if err != nil {
		return fmt.Errorf("%s: URL del MCP de Dory no válida", provider)
	}
	query := endpoint.Query()
	query.Set("thread", turn.ThreadID)
	endpoint.RawQuery = query.Encode()
	turn.MCPURL = endpoint.String()
	turn.MCPToken = auth.AgentMCPToken(m.MCPToken, turn.ThreadID)
	tools, err := checkMCP(ctx, turn.MCPURL, turn.MCPToken)
	if err != nil {
		return fmt.Errorf("%s: MCP de Dory: %w", provider, err)
	}
	turn.Instructions = buildInstructions(provider, turn.Model, tools, m.registry.Capabilities(provider).DeferredTools)
	if card := m.threadContext(ctx, thread, turn.Dir); card != "" {
		turn.Instructions += "\n\n" + card
	}
	return nil
}

func checkMCP(ctx context.Context, endpoint, token string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Dory's stateless JSON transport accepts tools/list without a session.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		return nil, fmt.Errorf("URL no válida")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("el servidor no responde")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("el catálogo devolvió HTTP %d", res.StatusCode)
	}
	var catalog struct {
		Result struct {
			Tools []struct{ Name string }
		}
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&catalog); err != nil || len(catalog.Result.Tools) == 0 {
		return nil, fmt.Errorf("el catálogo de herramientas no está disponible")
	}
	names := make([]string, 0, len(catalog.Result.Tools))
	for _, tool := range catalog.Result.Tools {
		names = append(names, tool.Name)
	}
	return names, nil
}
