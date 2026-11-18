package runtime

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// OpenCode runs "opencode run --format json"; a later turn continues the
// same session.
type OpenCode struct{ Bin string }

func (OpenCode) ID() string { return "opencode" }

func (o OpenCode) bin() string {
	if o.Bin != "" {
		return o.Bin
	}
	return "opencode"
}

func (o OpenCode) Detect(ctx context.Context) Status {
	version, ok := detect(ctx, o.bin())
	st := Status{ID: "opencode", Name: "OpenCode", Installed: ok, Version: version, Capabilities: []string{"interrupt", "resume"}}
	if !ok {
		return st
	}
	// Models depend on the providers configured in OpenCode: ask it.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, o.bin(), "models").Output()
	st.Models = append(st.Models, Model{Slug: "", Name: "Por defecto"})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			st.Models = append(st.Models, Model{Slug: line, Name: line})
		}
	}
	return st
}

func (o OpenCode) Run(ctx context.Context, t Turn) (<-chan Event, error) {
	args := []string{"run", "--format", "json"}
	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}
	if t.Session != "" {
		args = append(args, "--session", t.Session)
	}
	cmd := exec.CommandContext(ctx, o.bin(), append(args, t.Text)...)
	cmd.Dir = t.Dir
	p := &openCodeParser{run: t.RunID}
	events, err := stream(ctx, cmd, "", p.parse)
	if err != nil {
		return nil, err
	}
	// OpenCode has no end-of-turn event: the turn ends with the process.
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		done := false
		for e := range events {
			done = done || e.Done
			out <- e
		}
		if !done {
			out <- Event{Done: true}
		}
	}()
	return out, nil
}

type openCodeParser struct {
	run     string
	session bool
}

func (p *openCodeParser) parse(line []byte, emit func(Event)) {
	var l struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionID"`
		Part      struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Text  string `json:"text"`
			Tool  string `json:"tool"`
			State struct {
				Status string          `json:"status"`
				Title  string          `json:"title"`
				Input  json.RawMessage `json:"input"`
				Output string          `json:"output"`
				Error  string          `json:"error"`
			} `json:"state"`
			Reason string `json:"reason"`
		} `json:"part"`
		Error struct {
			Data struct {
				Message string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &l) != nil {
		return
	}
	if l.SessionID != "" && !p.session {
		p.session = true
		emit(Event{Session: l.SessionID})
	}
	m := &Message{ID: p.run + ":" + l.Part.ID, RunID: p.run, CreatedAt: now()}
	switch l.Part.Type {
	case "text":
		m.Role, m.Text = "assistant", l.Part.Text
	case "reasoning":
		m.Role, m.Title, m.Text = "reasoning", "Pensando", l.Part.Text
	case "tool":
		s := l.Part.State
		m.Role, m.Title, m.Detail, m.Text = "tool", l.Part.Tool, s.Title, clip(s.Output+s.Error, 4000)
		if m.Detail == "" {
			m.Detail = toolDetail(l.Part.Tool, s.Input)
		}
		m.Tool = toolCall(l.Part.Tool, s.Input)
		if m.Tool.Kind == "todo" {
			m.Role = "plan"
		}
		m.Status = map[string]string{"completed": "completed", "error": "failed"}[s.Status]
		if m.Status == "" {
			m.Status = "running"
		}
	default:
		if l.Type == "error" {
			emit(Event{Done: true, Err: l.Error.Data.Message})
		}
		return
	}
	emit(Event{Message: m})
}
