package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// TitleModel is the provider and model that write the titles of new ideas.
// An empty provider means titles are not generated.
type TitleModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (m *Manager) titleModelFile() string { return filepath.Join(m.dataDir, "title-model.json") }

// TitleModel returns the model chosen in the settings of this node.
func (m *Manager) TitleModel() TitleModel {
	var t TitleModel
	if raw, err := os.ReadFile(m.titleModelFile()); err == nil {
		_ = json.Unmarshal(raw, &t)
	}
	return t
}

// SetTitleModel saves the model that writes idea titles; an empty provider
// turns titling off.
func (m *Manager) SetTitleModel(t TitleModel) error {
	if t.Provider != "" {
		if _, ok := m.provider(t.Provider); !ok {
			return fmt.Errorf("%w: %s", ErrProvider, t.Provider)
		}
	}
	raw, _ := json.Marshal(t)
	return os.WriteFile(m.titleModelFile(), raw, 0o600)
}

// OneShot runs a single turn outside any thread: nothing is stored, it runs
// in an empty temporary directory without the Dory MCP, and it returns the
// last answer of the agent. Cancelling ctx stops the harness.
func (m *Manager) OneShot(ctx context.Context, provider, model, prompt string) (string, error) {
	p, ok := m.provider(provider)
	if !ok || !m.Enabled(provider) {
		return "", fmt.Errorf("%w: %s", ErrProvider, provider)
	}
	dir, err := os.MkdirTemp("", "dory-oneshot-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	turn := Turn{RunID: newRunID(), Dir: dir, Text: prompt, Model: model}
	var events <-chan Event
	switch p := p.(type) {
	case Interactive:
		s, err := p.Start(ctx, turn)
		if err != nil {
			return "", err
		}
		events = s.Events()
	case Runner:
		if events, err = p.Run(ctx, turn); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("%w: %s cannot run turns", ErrProvider, provider)
	}
	var answer, failure string
	for e := range events {
		switch {
		case e.Message != nil && e.Message.Role == "assistant":
			answer = e.Message.Text
		case e.Request != nil:
			// Nobody can answer: stop the turn.
			failure = "the agent asked for input"
			cancel()
		case e.Done && e.Err != "":
			failure = e.Err
		}
	}
	if ctx.Err() != nil && failure == "" {
		failure = ctx.Err().Error()
	}
	if failure != "" {
		return "", errors.New(failure)
	}
	return answer, nil
}
