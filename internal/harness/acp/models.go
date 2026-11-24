package acp

import (
	"context"

	"github.com/migueel15/dory/internal/harness"
)

type choice struct {
	Value   string
	Name    string
	Options []choice
}

type configOption struct {
	ID           string
	Category     string
	CurrentValue string
	Options      []choice
}

func (c configOption) choices() []choice {
	var out []choice
	for _, ch := range c.Options {
		if ch.Value != "" {
			out = append(out, ch)
		} else {
			out = append(out, ch.Options...)
		}
	}
	return out
}

// sessionInfo is what session/new, session/load and session/set_config_option answer.
type sessionInfo struct {
	SessionID     string
	ConfigOptions []configOption
	Modes         struct{ CurrentModeID string }
	Models        struct {
		CurrentModelID  string
		AvailableModels []struct{ ModelID, Name string }
	}
}

func (s sessionInfo) option(category string) configOption {
	for _, option := range s.ConfigOptions {
		if option.Category == category || option.ID == category {
			return option
		}
	}
	return configOption{}
}

func (s sessionInfo) models() []harness.Model {
	var models []harness.Model
	effort := s.option("thought_level")
	var efforts []string
	for _, ch := range effort.choices() {
		efforts = append(efforts, ch.Value)
	}
	model := s.option("model")
	for _, ch := range model.choices() {
		m := harness.Model{Slug: ch.Value, Name: ch.Name}
		if ch.Value == model.CurrentValue {
			m.Efforts = efforts
		}
		models = append(models, m)
	}
	if len(models) == 0 {
		for _, ch := range s.Models.AvailableModels {
			models = append(models, harness.Model{Slug: ch.ModelID, Name: ch.Name})
		}
	}
	return models
}

func (s *session) configure(ctx context.Context, info sessionInfo) error {
	if mode := info.option("mode"); mode.ID != "" && mode.CurrentValue != "default" {
		if err := s.setOption(ctx, &info, mode.ID, "default"); err != nil {
			return err
		}
	} else if mode.ID == "" && info.Modes.CurrentModeID != "" && info.Modes.CurrentModeID != "default" {
		if err := s.conn.call(ctx, "session/set_mode", params{"sessionId": s.id, "modeId": "default"}, nil, s.handle); err != nil {
			return err
		}
	}
	model := info.option("model").CurrentValue
	if model == "" {
		model = info.Models.CurrentModelID
	}
	if requested := s.turn.Model; requested != "" && requested != s.defaultModel && requested != model {
		if option := info.option("model"); option.ID != "" {
			if err := s.setOption(ctx, &info, option.ID, requested); err != nil {
				return err
			}
		} else if err := s.conn.call(ctx, "session/set_model", params{"sessionId": s.id, "modelId": requested}, nil, s.handle); err != nil {
			return err
		}
		model = requested
	}
	effort := info.option("thought_level")
	actual := effort.CurrentValue
	if requested := s.turn.Effort; effort.ID != "" && requested != "" && requested != actual {
		if err := s.setOption(ctx, &info, effort.ID, requested); err != nil {
			return err
		}
		actual = requested
	}
	s.emit(harness.SettingsReported{Model: model, Effort: actual})
	return nil
}

func (s *session) setOption(ctx context.Context, info *sessionInfo, id, value string) error {
	var result sessionInfo
	if err := s.conn.call(ctx, "session/set_config_option", params{"sessionId": s.id, "configId": id, "value": value}, &result, s.handle); err != nil {
		return err
	}
	if result.ConfigOptions != nil {
		info.ConfigOptions = result.ConfigOptions
	}
	return nil
}
