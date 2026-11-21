package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type antigravitySession struct {
	ctx       context.Context
	conn      *acpConn
	lv        *acpLive
	turn      Turn
	id        string
	live      bool
	events    chan Event
	chunk     Message
	seq       int
	tools     map[string]Message
	mu        sync.Mutex
	questions map[string]chan [][]string
}

func newAntigravitySession(ctx context.Context, c *acpConn, t Turn) *antigravitySession {
	return &antigravitySession{ctx: ctx, conn: c, turn: t, events: make(chan Event, 64), tools: map[string]Message{}, questions: map[string]chan [][]string{}}
}

func (s *antigravitySession) Events() <-chan Event { return s.events }
func (s *antigravitySession) Answer(id string, answers [][]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending := s.questions[id]; pending != nil {
		delete(s.questions, id)
		pending <- answers
		return nil
	}
	return ErrNoRequest
}

func (s *antigravitySession) emit(e Event) {
	select {
	case s.events <- e:
	case <-s.ctx.Done():
	}
}

func (s *antigravitySession) handle(frame acpFrame) error {
	switch frame.Method {
	case "session/update":
		if s.live {
			s.update(frame.Params)
		}
	case "session/request_permission":
		return s.permission(frame)
	case "elicitation/create", "session/elicitation":
		return s.elicit(frame)
	default:
		if len(frame.ID) != 0 {
			return s.conn.unsupported(frame)
		}
	}
	return nil
}

type acpLocation struct{ Path string }
type acpPermissionOption struct{ Kind, OptionID, Name string }
type acpTool struct {
	ToolCallID string
	Kind       string
	Title      string
	Status     string
	Locations  []acpLocation
	RawInput   json.RawMessage
	RawOutput  json.RawMessage
	Content    []struct {
		Type    string
		Path    string
		OldText string
		NewText string
		Content struct{ Text string }
	}
}

type acpUpdate struct {
	SessionID string
	Update    struct {
		Content        json.RawMessage
		SessionUpdate  string
		CurrentModelID string
		ConfigOptions  []acpConfig
		Entries        []struct{ Content, Status string }
	}
}

func (s *antigravitySession) update(raw json.RawMessage) {
	var e acpUpdate
	if json.Unmarshal(raw, &e) != nil || (s.id != "" && e.SessionID != s.id) {
		return
	}
	u := e.Update
	role := map[string]string{"agent_message_chunk": "assistant", "agent_thought_chunk": "reasoning", "user_message_chunk": "user"}[u.SessionUpdate]
	if role != "" {
		if role == "user" && s.live {
			return
		}
		var content struct{ Type, Text string }
		if json.Unmarshal(u.Content, &content) == nil && content.Type == "text" && content.Text != "" {
			if s.chunk.Role != role {
				s.finishChunk()
				s.seq++
				s.chunk = Message{ID: fmt.Sprintf("%s:acp:%d", s.turn.RunID, s.seq), RunID: s.turn.RunID, Role: role, CreatedAt: now(), Streaming: true}
			}
			s.chunk.Text += content.Text
			msg := s.chunk
			s.emit(Event{Message: &msg})
		}
		return
	}
	switch u.SessionUpdate {
	case "tool_call", "tool_call_update":
		var params struct{ Update acpTool }
		if json.Unmarshal(raw, &params) != nil {
			return
		}
		u := params.Update
		if u.ToolCallID == "" {
			return
		}
		s.finishChunk()
		msg, ok := s.tools[u.ToolCallID]
		if !ok {
			msg = Message{ID: s.turn.RunID + ":tool:" + u.ToolCallID, RunID: s.turn.RunID, Role: "tool", CreatedAt: now(), Tool: &ToolCall{Kind: "other"}}
		}
		tool := *msg.Tool
		if u.Title != "" {
			msg.Title, tool.Name = u.Title, u.Title
		}
		if kind := map[string]string{"read": "read", "edit": "edit", "delete": "edit", "move": "edit", "search": "search", "execute": "command", "fetch": "web", "think": "other"}[u.Kind]; kind != "" {
			tool.Kind = kind
		}
		if server, name, ok := mcpName(tool.Name); ok {
			tool.Kind, tool.Server, tool.Tool = "mcp", server, name
		}
		if len(u.RawInput) > 0 && string(u.RawInput) != "null" {
			var in struct {
				Command, Cmd, CommandLine, Path, Query, Pattern, URL string
				AbsolutePath                                         string `json:"absolute_path"`
			}
			_ = json.Unmarshal(u.RawInput, &in)
			tool.Input = clip(string(u.RawInput), 4000)
			tool.Command, tool.Path, tool.Query, tool.Pattern, tool.URL = in.Command, in.Path, in.Query, in.Pattern, in.URL
			if tool.Command == "" {
				tool.Command = in.Cmd
				if tool.Command == "" {
					tool.Command = in.CommandLine
				}
			}
			if tool.Path == "" {
				tool.Path = in.AbsolutePath
			}
		}
		if len(u.Locations) > 0 {
			tool.Path = u.Locations[0].Path
		}
		var files []FileEdit
		var detail []string
		for _, c := range u.Content {
			if c.Type == "diff" {
				files = append(files, lineDiff(c.Path, c.OldText, c.NewText))
			} else if c.Content.Text != "" {
				detail = append(detail, c.Content.Text)
			}
		}
		if len(files) > 0 {
			tool.Kind, tool.Files = "edit", files
		} else if tool.Kind == "edit" && len(tool.Files) == 0 {
			for _, location := range u.Locations {
				tool.Files = append(tool.Files, FileEdit{Path: location.Path})
			}
		}
		if len(detail) > 0 {
			msg.Detail = clip(strings.Join(detail, "\n"), 12000)
		} else if len(u.RawOutput) > 0 && string(u.RawOutput) != "null" {
			var output string
			if json.Unmarshal(u.RawOutput, &output) != nil {
				var result struct {
					CombinedOutput string
					ExitCode       *int
				}
				if json.Unmarshal(u.RawOutput, &result) == nil {
					output, tool.ExitCode = result.CombinedOutput, result.ExitCode
				}
			}
			if output != "" {
				msg.Detail = clip(output, 12000)
			}
		}
		if u.Status != "" {
			msg.Status = u.Status
			if msg.Status == "in_progress" || msg.Status == "pending" {
				msg.Status = "running"
			}
		}
		msg.Tool = &tool
		s.tools[u.ToolCallID] = msg
		s.emit(Event{Message: &msg})
	case "plan":
		s.finishChunk()
		tool := &ToolCall{Kind: "todo", Name: "Plan"}
		var plan strings.Builder
		for _, entry := range u.Entries {
			check := " "
			if entry.Status == "completed" {
				check = "x"
			}
			fmt.Fprintf(&plan, "- [%s] %s\n", check, entry.Content)
			tool.Todos = append(tool.Todos, Todo{Text: entry.Content, Status: entry.Status})
		}
		s.emit(Event{Message: &Message{ID: s.turn.RunID + ":acp-plan", RunID: s.turn.RunID, Role: "plan", Title: "Plan", Text: plan.String(), CreatedAt: now(), Tool: tool}})
	case "config_option_update":
		info := acpSessionInfo{ConfigOptions: u.ConfigOptions}
		s.emit(Event{Settings: &ReportedSettings{Model: info.option("model").CurrentValue, Effort: info.option("thought_level").CurrentValue}})
	case "current_model_update":
		s.emit(Event{Settings: &ReportedSettings{Model: u.CurrentModelID}})
	}
}

func (s *antigravitySession) finishChunk() {
	if s.chunk.ID == "" {
		return
	}
	msg := s.chunk
	msg.Streaming = false
	s.emit(Event{Message: &msg})
	s.chunk = Message{}
}

func (s *antigravitySession) permission(frame acpFrame) error {
	var p struct {
		SessionID string
		ToolCall  acpTool
		Options   []acpPermissionOption
	}
	if err := json.Unmarshal(frame.Params, &p); err != nil {
		return err
	}
	outcome := acpParams{"outcome": "cancelled"}
	if s.live && s.id == p.SessionID && strings.HasPrefix(p.ToolCall.ToolCallID, "interaction_") {
		return s.permissionQuestion(frame, p.ToolCall, p.Options)
	}
	allowed := s.live && s.id == p.SessionID && (s.turn.FullAccess || antigravityWorktreeEdit(s.turn.Dir, p.ToolCall))
	kinds := []string{"reject_once", "reject_always"}
	if allowed {
		kinds = []string{"allow_once"}
		if s.turn.FullAccess {
			kinds = append(kinds, "allow_always")
		}
	}
	for _, kind := range kinds {
		for _, option := range p.Options {
			if option.Kind == kind && option.OptionID != "" {
				return s.conn.reply(frame.ID, acpParams{"outcome": acpParams{"outcome": "selected", "optionId": option.OptionID}})
			}
		}
	}
	return s.conn.reply(frame.ID, acpParams{"outcome": outcome})
}

// Require concrete targets. An unfamiliar input shape or an unresolved path
// cannot establish that an edit stays in the worktree.
func antigravityWorktreeEdit(dir string, tool acpTool) bool {
	if tool.Kind != "edit" || dir == "" {
		return false
	}
	var paths []string
	for _, l := range tool.Locations {
		paths = append(paths, l.Path)
	}
	for _, c := range tool.Content {
		if c.Type == "diff" {
			paths = append(paths, c.Path)
		}
	}
	if len(tool.RawInput) > 0 && string(tool.RawInput) != "null" {
		var input struct {
			Path         string
			FilePath     string `json:"file_path"`
			TargetFile   string `json:"target_file"`
			AbsolutePath string `json:"absolute_path"`
		}
		if json.Unmarshal(tool.RawInput, &input) != nil {
			return false
		}
		for _, path := range []string{input.Path, input.FilePath, input.TargetFile, input.AbsolutePath} {
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil || len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !antigravityInside(root, path) {
			return false
		}
	}
	return true
}

func antigravityInside(root, path string) bool {
	if path == "" || (!filepath.IsAbs(path) && !filepath.IsLocal(path)) {
		return false
	}
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if component == ".." {
			return false
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	for {
		if rel, err := filepath.Rel(root, path); err != nil || rel == "." || !filepath.IsLocal(rel) {
			return false
		}
		// Resolve the closest existing parent for a file that is being added.
		if _, err := os.Lstat(path); err == nil {
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				return false
			}
			rel, err := filepath.Rel(root, real)
			return err == nil && filepath.IsLocal(rel)
		} else if !errors.Is(err, os.ErrNotExist) {
			return false
		}
		path = filepath.Dir(path)
		if path == root {
			return true
		}
	}
}

func (s *antigravitySession) elicit(frame acpFrame) error {
	action := func(value acpParams) error {
		if frame.Method == "session/elicitation" {
			value = acpParams{"action": value}
		}
		return s.conn.reply(frame.ID, value)
	}
	var p struct {
		SessionID, Mode, Message string
		RequestedSchema          struct {
			Properties map[string]struct {
				Type, Title, Description string
				Enum                     []string
				Items                    struct{ Enum []string }
			}
		}
	}
	if json.Unmarshal(frame.Params, &p) != nil || !s.live || (p.SessionID != "" && p.SessionID != s.id) || p.Mode != "form" || len(p.RequestedSchema.Properties) == 0 {
		return action(acpParams{"action": "decline"})
	}
	var ids []string
	for id, field := range p.RequestedSchema.Properties {
		if field.Type != "string" && field.Type != "array" {
			return action(acpParams{"action": "decline"})
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var questions []Question
	for _, id := range ids {
		field := p.RequestedSchema.Properties[id]
		q := Question{ID: id, Header: field.Title, Question: field.Description, MultiSelect: field.Type == "array"}
		if q.Question == "" {
			q.Question = field.Title
		}
		if q.Question == "" {
			q.Question = p.Message
		}
		values := field.Enum
		if q.MultiSelect {
			values = field.Items.Enum
		}
		for _, value := range values {
			q.Options = append(q.Options, Option{Label: value})
		}
		questions = append(questions, q)
	}
	answers, err := s.ask(frame, p.Message, questions)
	if err != nil {
		return err
	}
	content := acpParams{}
	for i, q := range questions {
		if i >= len(answers) || len(answers[i]) == 0 {
			continue
		}
		if q.MultiSelect {
			content[q.ID] = answers[i]
		} else {
			content[q.ID] = answers[i][0]
		}
	}
	return action(acpParams{"action": "accept", "content": content})
}

func (s *antigravitySession) permissionQuestion(frame acpFrame, tool acpTool, options []acpPermissionOption) error {
	q := Question{ID: tool.ToolCallID, Question: tool.Title}
	if q.Question == "" {
		q.Question = "Elige una opción"
	}
	ids := map[string]string{}
	for _, o := range options {
		label := o.Name
		if label == "" {
			label = o.OptionID
		}
		if _, exists := ids[label]; exists || label == "" || o.OptionID == "" {
			return s.conn.reply(frame.ID, acpParams{"outcome": acpParams{"outcome": "cancelled"}})
		}
		ids[label] = o.OptionID
		q.Options = append(q.Options, Option{Label: label})
	}
	outcome := acpParams{"outcome": "cancelled"}
	if len(q.Options) > 0 {
		answers, err := s.ask(frame, q.Question, []Question{q})
		if err != nil {
			return err
		}
		if len(answers) == 1 && len(answers[0]) == 1 && ids[answers[0][0]] != "" {
			outcome = acpParams{"outcome": "selected", "optionId": ids[answers[0][0]]}
		}
	}
	return s.conn.reply(frame.ID, acpParams{"outcome": outcome})
}

func (s *antigravitySession) ask(frame acpFrame, title string, questions []Question) ([][]string, error) {
	id := s.turn.RunID + ":acp-question:" + string(frame.ID)
	pending := make(chan [][]string, 1)
	s.mu.Lock()
	s.questions[id] = pending
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.questions, id)
		s.mu.Unlock()
	}()
	s.finishChunk()
	s.emit(Event{Message: &Message{ID: id, RunID: s.turn.RunID, Role: "tool", Title: title, CreatedAt: now(), Tool: &ToolCall{Kind: "question", Questions: questions}}})
	s.emit(Event{Request: &Request{ID: id, Kind: "question", MessageID: id, Questions: questions, CreatedAt: now()}})
	select {
	case answers := <-pending:
		return answers, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
