package runtime

import "context"

type acpChoice struct {
	Value   string
	Name    string
	Options []acpChoice
}

type acpConfig struct {
	ID           string
	Category     string
	CurrentValue string
	Options      []acpChoice
}

func (c acpConfig) choices() []acpChoice {
	var out []acpChoice
	for _, choice := range c.Options {
		if choice.Value != "" {
			out = append(out, choice)
		} else {
			out = append(out, choice.Options...)
		}
	}
	return out
}

type acpSessionInfo struct {
	SessionID     string
	ConfigOptions []acpConfig
	Modes         struct{ CurrentModeID string }
	Models        struct {
		CurrentModelID  string
		AvailableModels []struct{ ModelID, Name string }
	}
}

func (s acpSessionInfo) option(category string) acpConfig {
	for _, option := range s.ConfigOptions {
		if option.Category == category || option.ID == category {
			return option
		}
	}
	return acpConfig{}
}

func (s acpSessionInfo) models() []Model {
	var models []Model
	effort := s.option("thought_level")
	var efforts []string
	for _, choice := range effort.choices() {
		efforts = append(efforts, choice.Value)
	}
	model := s.option("model")
	for _, choice := range model.choices() {
		m := Model{Slug: choice.Value, Name: choice.Name}
		if choice.Value == model.CurrentValue {
			m.Efforts = efforts
		}
		models = append(models, m)
	}
	if len(models) == 0 {
		for _, choice := range s.Models.AvailableModels {
			models = append(models, Model{Slug: choice.ModelID, Name: choice.Name})
		}
	}
	return models
}

func (s *antigravitySession) configure(ctx context.Context, info acpSessionInfo) error {
	if mode := info.option("mode"); mode.ID != "" && mode.CurrentValue != "default" {
		if err := s.setOption(ctx, &info, mode.ID, "default"); err != nil {
			return err
		}
	} else if mode.ID == "" && info.Modes.CurrentModeID != "" && info.Modes.CurrentModeID != "default" {
		if err := s.conn.call(ctx, "session/set_mode", acpParams{"sessionId": s.id, "modeId": "default"}, nil, s.handle); err != nil {
			return err
		}
	}
	model := info.option("model").CurrentValue
	if model == "" {
		model = info.Models.CurrentModelID
	}
	if requested := s.turn.Model; requested != "" && requested != "antigravity-default" && requested != model {
		if option := info.option("model"); option.ID != "" {
			if err := s.setOption(ctx, &info, option.ID, requested); err != nil {
				return err
			}
		} else if err := s.conn.call(ctx, "session/set_model", acpParams{"sessionId": s.id, "modelId": requested}, nil, s.handle); err != nil {
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
	s.emit(Event{Settings: &ReportedSettings{Model: model, Effort: actual}})
	return nil
}

func (s *antigravitySession) setOption(ctx context.Context, info *acpSessionInfo, id, value string) error {
	var result acpSessionInfo
	if err := s.conn.call(ctx, "session/set_config_option", acpParams{"sessionId": s.id, "configId": id, "value": value}, &result, s.handle); err != nil {
		return err
	}
	if result.ConfigOptions != nil {
		info.ConfigOptions = result.ConfigOptions
	}
	return nil
}
