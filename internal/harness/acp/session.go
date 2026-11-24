package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

type event = harness.Event

type session struct {
	ctx  context.Context
	conn *conn
	lv   *liveProc
	turn harness.Turn
	// defaultModel is the slug that asks for the agent's own default model.
	defaultModel string
	id           string
	live         bool
	events       chan event
	chunk        *kit.Text
	seq          int
	tools        map[string]harness.Message
	mu           sync.Mutex
	questions    map[string]chan [][]string
}

func newSession(ctx context.Context, c *conn, t harness.Turn, defaultModel string) *session {
	return &session{ctx: ctx, conn: c, turn: t, defaultModel: defaultModel, events: make(chan event, 64), tools: map[string]harness.Message{}, questions: map[string]chan [][]string{}}
}

func (s *session) Events() <-chan event { return s.events }

func (s *session) Respond(id string, answers [][]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending := s.questions[id]; pending != nil {
		delete(s.questions, id)
		pending <- answers
		return nil
	}
	return apperr.New(apperr.NoRequest, "the request is no longer pending")
}

func (s *session) emit(e event) {
	select {
	case s.events <- e:
	case <-s.ctx.Done():
	}
}

func (s *session) handle(frame rpcFrame) error {
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

type location struct{ Path string }
type permissionOption struct{ Kind, OptionID, Name string }
type toolCall struct {
	ToolCallID string
	Kind       string
	Title      string
	Status     string
	Locations  []location
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

type updateParams struct {
	SessionID string
	Update    struct {
		Content        json.RawMessage
		SessionUpdate  string
		CurrentModelID string
		ConfigOptions  []configOption
		Entries        []struct{ Content, Status string }
	}
}

func (s *session) update(raw json.RawMessage) {
	var e updateParams
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
			kind := harness.TextKind(role)
			if role == "user" {
				// A replayed user message is not part of a live turn.
				s.finishChunk()
				s.seq++
				s.emit(harness.MessageReported{Message: harness.Message{ID: fmt.Sprintf("%s:acp:%d", s.turn.RunID, s.seq), RunID: s.turn.RunID, Role: "user", Text: content.Text, CreatedAt: kit.Now()}})
				return
			}
			if s.chunk == nil || s.chunk.Kind != kind {
				s.finishChunk()
				s.seq++
				s.chunk = kit.NewText(fmt.Sprintf("%s:acp:%d", s.turn.RunID, s.seq), s.turn.RunID, kind)
			}
			s.chunk.Append(content.Text, s.emit)
		}
		return
	}
	switch u.SessionUpdate {
	case "tool_call", "tool_call_update":
		var params struct{ Update toolCall }
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
			msg = harness.Message{ID: s.turn.RunID + ":tool:" + u.ToolCallID, RunID: s.turn.RunID, Role: "tool", CreatedAt: kit.Now(), Tool: &harness.ToolCall{Kind: "other"}}
		}
		tool := *msg.Tool
		if u.Title != "" {
			msg.Title, tool.Name = u.Title, u.Title
		}
		if kind := map[string]string{"read": "read", "edit": "edit", "delete": "edit", "move": "edit", "search": "search", "execute": "command", "fetch": "web", "think": "other"}[u.Kind]; kind != "" {
			tool.Kind = kind
		}
		if server, name, ok := kit.McpName(tool.Name); ok {
			tool.Kind, tool.Server, tool.Tool = "mcp", server, name
		}
		if len(u.RawInput) > 0 && string(u.RawInput) != "null" {
			var in struct {
				Command, Cmd, CommandLine, Path, Query, Pattern, URL string
				AbsolutePath                                         string `json:"absolute_path"`
			}
			_ = json.Unmarshal(u.RawInput, &in)
			msg.InputChars = len(u.RawInput)
			tool.Input = kit.Clip(string(u.RawInput), 4000)
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
		var files []harness.FileEdit
		var detail []string
		for _, c := range u.Content {
			if c.Type == "diff" {
				files = append(files, kit.LineDiff(c.Path, c.OldText, c.NewText))
			} else if c.Content.Text != "" {
				detail = append(detail, c.Content.Text)
			}
		}
		if len(files) > 0 {
			tool.Kind, tool.Files = "edit", files
		} else if tool.Kind == "edit" && len(tool.Files) == 0 {
			for _, location := range u.Locations {
				tool.Files = append(tool.Files, harness.FileEdit{Path: location.Path})
			}
		}
		if len(detail) > 0 {
			output := strings.Join(detail, "\n")
			msg.ResultChars = len(output)
			msg.Detail = kit.Clip(output, 12000)
		} else if len(u.RawOutput) > 0 && string(u.RawOutput) != "null" {
			msg.ResultChars = len(u.RawOutput)
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
				msg.Detail = kit.Clip(output, 12000)
			}
		}
		if u.Status != "" {
			msg.Status = u.Status
			if msg.Status == "in_progress" || msg.Status == "pending" {
				msg.Status = "running"
			}
		}
		msg.Tool = &tool
		_, seen := s.tools[u.ToolCallID]
		s.tools[u.ToolCallID] = msg
		switch {
		case msg.Status == "completed" || msg.Status == "failed":
			s.emit(kit.FinishTool(&msg))
		case !seen:
			s.emit(kit.StartTool(&msg))
		default:
			s.emit(kit.UpdateTool(&msg))
		}
	case "plan":
		s.finishChunk()
		tool := &harness.ToolCall{Kind: "todo", Name: "Plan"}
		var plan strings.Builder
		for _, entry := range u.Entries {
			check := " "
			if entry.Status == "completed" {
				check = "x"
			}
			fmt.Fprintf(&plan, "- [%s] %s\n", check, entry.Content)
			tool.Todos = append(tool.Todos, harness.Todo{Text: entry.Content, Status: entry.Status})
		}
		s.emit(kit.Plan(&harness.Message{ID: s.turn.RunID + ":acp-plan", RunID: s.turn.RunID, Role: "plan", Title: "Plan", Text: plan.String(), CreatedAt: kit.Now(), Tool: tool}))
	case "config_option_update":
		info := sessionInfo{ConfigOptions: u.ConfigOptions}
		s.emit(harness.SettingsReported{Model: info.option("model").CurrentValue, Effort: info.option("thought_level").CurrentValue})
	case "current_model_update":
		s.emit(harness.SettingsReported{Model: u.CurrentModelID})
	}
}

func (s *session) finishChunk() {
	if s.chunk == nil {
		return
	}
	s.emit(s.chunk.Done(s.chunk.String()))
	s.chunk = nil
}

func (s *session) permission(frame rpcFrame) error {
	var p struct {
		SessionID string
		ToolCall  toolCall
		Options   []permissionOption
	}
	if err := json.Unmarshal(frame.Params, &p); err != nil {
		return err
	}
	outcome := params{"outcome": "cancelled"}
	if s.live && s.id == p.SessionID && strings.HasPrefix(p.ToolCall.ToolCallID, "interaction_") {
		return s.permissionQuestion(frame, p.ToolCall, p.Options)
	}
	allowed := s.live && s.id == p.SessionID && (s.turn.FullAccess || worktreeEdit(s.turn.Dir, p.ToolCall))
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
				return s.conn.reply(frame.ID, params{"outcome": params{"outcome": "selected", "optionId": option.OptionID}})
			}
		}
	}
	return s.conn.reply(frame.ID, params{"outcome": outcome})
}

// Require concrete targets. An unfamiliar input shape or an unresolved path
// cannot establish that an edit stays in the worktree.
func worktreeEdit(dir string, tool toolCall) bool {
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
		if !inside(root, path) {
			return false
		}
	}
	return true
}

func inside(root, path string) bool {
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

func (s *session) elicit(frame rpcFrame) error {
	action := func(value params) error {
		if frame.Method == "session/elicitation" {
			value = params{"action": value}
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
		return action(params{"action": "decline"})
	}
	var ids []string
	for id, field := range p.RequestedSchema.Properties {
		if field.Type != "string" && field.Type != "array" {
			return action(params{"action": "decline"})
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var questions []harness.Question
	for _, id := range ids {
		field := p.RequestedSchema.Properties[id]
		q := harness.Question{ID: id, Header: field.Title, Question: field.Description, MultiSelect: field.Type == "array"}
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
			q.Options = append(q.Options, harness.Option{Label: value})
		}
		questions = append(questions, q)
	}
	answers, err := s.ask(frame, p.Message, questions)
	if err != nil {
		return err
	}
	content := params{}
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
	return action(params{"action": "accept", "content": content})
}

func (s *session) permissionQuestion(frame rpcFrame, tool toolCall, options []permissionOption) error {
	q := harness.Question{ID: tool.ToolCallID, Question: tool.Title}
	if q.Question == "" {
		q.Question = i18n.Text("acp.elige_opcion")
	}
	ids := map[string]string{}
	for _, o := range options {
		label := o.Name
		if label == "" {
			label = o.OptionID
		}
		if _, exists := ids[label]; exists || label == "" || o.OptionID == "" {
			return s.conn.reply(frame.ID, params{"outcome": params{"outcome": "cancelled"}})
		}
		ids[label] = o.OptionID
		q.Options = append(q.Options, harness.Option{Label: label})
	}
	outcome := params{"outcome": "cancelled"}
	if len(q.Options) > 0 {
		answers, err := s.ask(frame, q.Question, []harness.Question{q})
		if err != nil {
			return err
		}
		if len(answers) == 1 && len(answers[0]) == 1 && ids[answers[0][0]] != "" {
			outcome = params{"outcome": "selected", "optionId": ids[answers[0][0]]}
		}
	}
	return s.conn.reply(frame.ID, params{"outcome": outcome})
}

func (s *session) ask(frame rpcFrame, title string, questions []harness.Question) ([][]string, error) {
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
	s.emit(kit.StartTool(&harness.Message{ID: id, RunID: s.turn.RunID, Role: "tool", Title: title, CreatedAt: kit.Now(), Tool: &harness.ToolCall{Kind: "question", Questions: questions}}))
	s.emit(harness.RequestOpened{Request: harness.Request{ID: id, Kind: "question", MessageID: id, Questions: questions, CreatedAt: kit.Now()}})
	select {
	case answers := <-pending:
		return answers, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
