package runtime

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/nodestate"
)

// TitleModel is the provider and model that write the titles of new ideas.
// An empty provider means titles are not generated.
type TitleModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// TitleModel returns the model chosen in the settings of this node.
func (m *Manager) TitleModel() TitleModel {
	var t TitleModel
	if _, err := m.state.Get(context.Background(), nodestate.KeyTitleModel, &t); err != nil {
		log.Printf("runtime: load title model: %v", err)
		return TitleModel{}
	}
	return t
}

// SetTitleModel saves the model that writes idea titles; an empty provider
// turns titling off.
func (m *Manager) SetTitleModel(t TitleModel) error {
	if t.Provider != "" {
		if _, ok := m.provider(t.Provider); !ok {
			return ErrProvider.Msgf("%s", t.Provider)
		}
	}
	return m.state.Put(context.Background(), nodestate.KeyTitleModel, t)
}

// OneShot runs a single turn outside any thread: nothing is stored, it runs
// in an empty temporary directory without the Dory MCP, and it returns the
// last answer of the agent. Cancelling ctx stops the harness.
func (m *Manager) OneShot(ctx context.Context, provider, model, prompt string) (string, error) {
	p, ok := m.provider(provider)
	if !ok || !m.Enabled(provider) {
		return "", ErrProvider.Msgf("%s", provider)
	}
	dir, err := os.MkdirTemp("", "dory-oneshot-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	turn := Turn{RunID: newRunID(), Dir: dir, Text: prompt, Model: model}
	session, err := p.Start(ctx, turn)
	if err != nil {
		return "", err
	}
	var answer, failure string
	messages := turnMessages{run: turn.RunID, items: map[string]Message{}}
	for e := range session.Events() {
		if msg, ok := messages.apply(e); ok && msg.Role == "assistant" {
			answer = msg.Text
		}
		switch e := e.(type) {
		case harness.RequestOpened:
			// Nobody can answer: stop the turn.
			failure = "the agent asked for input"
			cancel()
		case harness.TurnEnded:
			if e.Error != "" {
				failure = e.Error
			}
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
