package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Start runs a turn over "codex app-server", Codex's JSON-RPC protocol on
// stdio: initialize, start or resume the thread, start the turn. Its
// request_user_input pauses the turn until the user answers. The process
// stays open for the next turn of the thread (see live.go), which only
// starts a turn, so what the agent left running survives.
func (c Codex) Start(ctx context.Context, t Turn) (Session, error) {
	key := strings.Join([]string{t.Model, fmt.Sprint(t.FullAccess), t.Dir, t.MCPURL}, "\x00")
	p := reuseStdio(t.ThreadID, "codex", key)
	if p == nil {
		args := []string{"app-server", "--enable", "default_mode_request_user_input"}
		cmd := providerCommand(context.Background(), t.ThreadID, c.bin(), args...)
		cmd.Dir = t.Dir
		cmd.Env = threadEnv(os.Environ(), t.ThreadID)
		if t.MCPURL != "" {
			cmd.Args = append(cmd.Args,
				"-c", fmt.Sprintf("mcp_servers.dory.url=%q", t.MCPURL),
				"-c", `mcp_servers.dory.bearer_token_env_var="DORY_MCP_TOKEN"`,
				// ask_user waits for the user, so a call may take long.
				"-c", "mcp_servers.dory.tool_timeout_sec=86400",
				"-c", "mcp_servers.dory.enabled=true",
				"-c", "mcp_servers.dory.required=true")
			cmd.Env = append(cmd.Env, "DORY_MCP_TOKEN="+t.MCPToken)
		}
		var err error
		if p, err = openStdio(cmd, t.ThreadID, "codex", key); err != nil {
			return nil, err
		}
	}
	s := newCodexSession(t, p.in)
	s.proc = p
	turn := p.begin(t.ThreadID, s.handle)
	s.events = turn.out
	// Call ids stay unique on the process across turns.
	s.next = p.turns * 1_000_000
	p.watch(ctx, t.ThreadID, "codex", turn, s.interrupt)
	var err error
	if thread := p.get("thread"); thread != "" {
		err = s.startTurn(thread)
	} else {
		err = s.call("initialize", map[string]any{
			"clientInfo":   map[string]any{"name": "dory", "title": "Dory", "version": "2"},
			"capabilities": map[string]any{"experimentalApi": true, "requestAttestation": false},
		})
	}
	if err != nil {
		p.abort(turn)
		return nil, err
	}
	return s, nil
}

// codexSession drives one turn of codex app-server. Responses to its own
// calls move the handshake on; server requests ask for the user.
type codexSession struct {
	turn      Turn
	parser    *codexParser
	events    <-chan Event
	proc      *stdioProc
	turnID    string
	lastUsage *codexTokenUsage

	mu      sync.Mutex
	in      io.WriteCloser
	closed  bool
	next    int
	calls   map[int]string // call id → method
	pending map[string]*codexAsk
	texts   map[string]*Message
	failure string
}

// codexAsk is a request_user_input call waiting for the user.
type codexAsk struct {
	rpcID   json.RawMessage
	message *Message
	answers [][]string
}

func newCodexSession(t Turn, in io.WriteCloser) *codexSession {
	return &codexSession{turn: t, parser: &codexParser{run: t.RunID, started: true}, in: in,
		calls: map[int]string{}, pending: map[string]*codexAsk{}, texts: map[string]*Message{}}
}

func (s *codexSession) Events() <-chan Event { return s.events }

// Answer replies to request_user_input with the answers by question id.
func (s *codexSession) Answer(id string, answers [][]string) error {
	s.mu.Lock()
	ask, ok := s.pending[id]
	if ok && ask.answers != nil {
		ok = false
	}
	if ok {
		ask.answers = answers
	}
	s.mu.Unlock()
	if !ok {
		return ErrNoRequest
	}
	byID := map[string]any{}
	for i, q := range ask.message.Tool.Questions {
		a := []string{}
		if i < len(answers) {
			a = answers[i]
		}
		byID[q.ID] = map[string]any{"answers": a}
	}
	return s.write(map[string]any{"jsonrpc": "2.0", "id": ask.rpcID, "result": map[string]any{"answers": byID}})
}

type codexRPC struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *codexSession) handle(line []byte, emit func(Event)) {
	var r codexRPC
	if json.Unmarshal(line, &r) != nil {
		return
	}
	switch {
	case r.Method != "" && len(r.ID) > 0:
		s.request(r, emit)
	case r.Method != "":
		s.notification(r, emit)
	case len(r.ID) > 0:
		s.response(r, emit)
	}
}

// response moves the handshake on: initialize, thread, turn.
func (s *codexSession) response(r codexRPC, emit func(Event)) {
	var id int
	_ = json.Unmarshal(r.ID, &id)
	s.mu.Lock()
	method := s.calls[id]
	delete(s.calls, id)
	s.mu.Unlock()
	if r.Error != nil {
		s.finish(emit, method+": "+r.Error.Message)
		return
	}
	t := s.turn
	switch method {
	case "initialize":
		s.write(map[string]any{"jsonrpc": "2.0", "method": "initialized"})
		params := map[string]any{"cwd": t.Dir, "approvalPolicy": "never", "sandbox": "workspace-write"}
		if t.FullAccess {
			params["sandbox"] = "danger-full-access"
		}
		if t.Model != "" {
			params["model"] = t.Model
		}
		if t.Instructions != "" {
			params["developerInstructions"] = t.Instructions
		}
		if t.Session != "" {
			params["threadId"] = t.Session
			s.call("thread/resume", params)
		} else {
			s.call("thread/start", params)
		}
	case "thread/start", "thread/resume":
		var res struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoningEffort"`
		}
		_ = json.Unmarshal(r.Result, &res)
		emit(Event{Session: res.Thread.ID})
		if res.Model != "" || res.ReasoningEffort != "" {
			emit(Event{Settings: &ReportedSettings{Model: res.Model, Effort: res.ReasoningEffort}})
		}
		s.proc.set("thread", res.Thread.ID)
		s.startTurn(res.Thread.ID)
	case "turn/start":
		var res struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		_ = json.Unmarshal(r.Result, &res)
		s.mu.Lock()
		s.turnID = res.Turn.ID
		s.mu.Unlock()
	}
}

// request answers what the server asks. request_user_input waits for the
// user; approvals are declined, as "never" should not raise them.
func (s *codexSession) request(r codexRPC, emit func(Event)) {
	switch r.Method {
	case "item/tool/requestUserInput":
		var p struct {
			ItemID    string          `json:"itemId"`
			Questions json.RawMessage `json:"questions"`
		}
		_ = json.Unmarshal(r.Params, &p)
		qs := parseQuestions(json.RawMessage(`{"questions":` + string(p.Questions) + `}`))
		m := &Message{ID: s.turn.RunID + ":" + p.ItemID, RunID: s.turn.RunID, Role: "tool", Title: "request_user_input",
			Status: "running", CreatedAt: now(), Tool: &ToolCall{Kind: "question", Name: "request_user_input", Questions: qs}}
		s.mu.Lock()
		s.pending[p.ItemID] = &codexAsk{rpcID: r.ID, message: m}
		s.mu.Unlock()
		emit(Event{Message: copyOf(m)})
		emit(Event{Request: &Request{ID: p.ItemID, Kind: "question", MessageID: m.ID, Questions: qs, CreatedAt: now()}})
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		s.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": map[string]any{"decision": "decline"}})
	default:
		s.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "error": map[string]any{"code": -32601, "message": "Dory does not support " + r.Method}})
	}
}

// codexAppItem is an item of app-server; it maps onto the items of
// "codex exec --json", whose parser already turns them into messages.
type codexAppItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Summary          []string        `json:"summary"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregatedOutput"`
	ExitCode         *int            `json:"exitCode"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	Arguments        json.RawMessage `json:"arguments"`
	Query            string          `json:"query"`
	Changes          []struct {
		Path string `json:"path"`
		Kind struct {
			Type string `json:"type"`
		} `json:"kind"`
		Diff string `json:"diff"`
	} `json:"changes"`
}

func (it codexAppItem) exec() codexItem {
	out := codexItem{ID: it.ID, Text: it.Text, Command: it.Command, AggregatedOutput: it.AggregatedOutput, ExitCode: it.ExitCode,
		Status: it.Status, Server: it.Server, Tool: it.Tool, Arguments: it.Arguments, Query: it.Query}
	out.Type = map[string]string{"agentMessage": "agent_message", "reasoning": "reasoning", "commandExecution": "command_execution",
		"fileChange": "file_change", "mcpToolCall": "mcp_tool_call", "webSearch": "web_search"}[it.Type]
	if it.Type == "reasoning" {
		out.Text = strings.Join(it.Summary, "\n")
	}
	for _, c := range it.Changes {
		out.Changes = append(out.Changes, struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{c.Path, c.Kind.Type})
	}
	return out
}

type codexTokenUsage struct {
	Input     int64 `json:"inputTokens"`
	Read      int64 `json:"cachedInputTokens"`
	Write     int64 `json:"cacheWriteInputTokens"`
	Output    int64 `json:"outputTokens"`
	Reasoning int64 `json:"reasoningOutputTokens"`
	Total     int64 `json:"totalTokens"`
}

func (s *codexSession) notification(r codexRPC, emit func(Event)) {
	var p struct {
		TurnID     string `json:"turnId"`
		TokenUsage *struct {
			Total codexTokenUsage `json:"total"`
			Last  codexTokenUsage `json:"last"`
		} `json:"tokenUsage"`
		Item      codexAppItem `json:"item"`
		ItemID    string       `json:"itemId"`
		Delta     string       `json:"delta"`
		WillRetry bool         `json:"willRetry"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
		Plan []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
		RequestID json.RawMessage `json:"requestId"`
	}
	_ = json.Unmarshal(r.Params, &p)
	switch r.Method {
	case "thread/tokenUsage/updated":
		if p.TokenUsage == nil || (s.turnID != "" && p.TurnID != s.turnID) {
			return
		}
		if s.lastUsage != nil && *s.lastUsage == p.TokenUsage.Total {
			return
		}
		s.lastUsage = &p.TokenUsage.Total
		u := p.TokenUsage.Last
		emit(Event{Step: &StepUsage{Model: s.turn.Model, Input: max(0, u.Input-u.Read-u.Write), CacheRead: u.Read, CacheWrite: u.Write, Output: u.Output, Reasoning: u.Reasoning}})
	case "item/started", "item/completed":
		done := r.Method == "item/completed"
		if m := s.parser.message(p.Item.exec(), done); m != nil {
			if p.Item.Type == "agentMessage" || p.Item.Type == "reasoning" {
				s.texts[p.Item.ID] = m
				if p.Item.Type == "agentMessage" && !done {
					// Keep the raw text so tags can span multiple deltas.
					raw := copyOf(m)
					raw.Text = p.Item.Text
					s.texts[p.Item.ID] = raw
				}
				if m.Text == "" && !done {
					return
				}
			}
			m.InputChars = len(p.Item.Arguments)
			if done {
				m.ResultChars = len(p.Item.AggregatedOutput)
				if p.Item.Type == "mcpToolCall" {
					raw := p.Item.Result
					if len(raw) == 0 || string(raw) == "null" {
						raw = p.Item.Error
					}
					if string(raw) != "null" {
						m.ResultChars = len(raw)
					}
				}
			}
			if p.Item.Type == "fileChange" && m.Tool != nil {
				for i, c := range p.Item.Changes {
					if i < len(m.Tool.Files) {
						m.Tool.Files[i] = withDiff(m.Tool.Files[i], c.Diff)
					}
				}
			}
			emit(Event{Message: m})
		}
	case "item/agentMessage/delta", "item/reasoning/summaryTextDelta":
		if m := s.texts[p.ItemID]; m != nil {
			m.Text += p.Delta
			snapshot := copyOf(m)
			if r.Method == "item/agentMessage/delta" {
				snapshot.Text = stripCodexMemoryCitations(m.Text, true)
			}
			emit(Event{Message: snapshot})
		}
	case "turn/plan/updated":
		m := &Message{ID: s.turn.RunID + ":plan", RunID: s.turn.RunID, Role: "plan", Title: "Plan", CreatedAt: now(),
			Tool: &ToolCall{Kind: "todo", Name: "update_plan"}}
		var lines []string
		for _, x := range p.Plan {
			st := map[string]string{"inProgress": "in_progress"}[x.Status]
			if st == "" {
				st = x.Status
			}
			m.Tool.Todos = append(m.Tool.Todos, Todo{Text: x.Step, Status: st})
			lines = append(lines, x.Step)
		}
		m.Detail = strings.Join(lines, "\n")
		emit(Event{Message: m})
	case "serverRequest/resolved":
		// The answer went through: the card shows it.
		s.mu.Lock()
		for id, ask := range s.pending {
			if ask.answers != nil {
				ask.message.Status = "completed"
				ask.message.Tool.Questions = answered(ask.message.Tool.Questions, ask.answers)
				emit(Event{Message: copyOf(ask.message)})
				delete(s.pending, id)
			}
		}
		s.mu.Unlock()
	case "error":
		if !p.WillRetry {
			s.failure = p.Error.Message
		}
	case "turn/completed":
		msg := ""
		if p.Turn.Status == "failed" || p.Turn.Status == "interrupted" {
			msg = s.failure
			if p.Turn.Error != nil && p.Turn.Error.Message != "" {
				msg = p.Turn.Error.Message
			}
			if msg == "" {
				msg = p.Turn.Status
			}
		}
		s.finish(emit, msg)
	}
}

// startTurn sends the user's text as a new turn of the thread.
func (s *codexSession) startTurn(thread string) error {
	params := map[string]any{"threadId": thread,
		"input": []any{map[string]any{"type": "text", "text": s.turn.Text, "text_elements": []any{}}}}
	if s.turn.Effort != "" {
		params["effort"] = s.turn.Effort
	}
	return s.call("turn/start", params)
}

// interrupt asks Codex to stop the running turn.
func (s *codexSession) interrupt() {
	s.mu.Lock()
	turn := s.turnID
	s.mu.Unlock()
	_ = s.call("turn/interrupt", map[string]any{"threadId": s.proc.get("thread"), "turnId": turn})
}

// finish ends the turn; the process stays open for the next one.
func (s *codexSession) finish(emit func(Event), failure string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	for _, m := range s.texts {
		if m.Streaming {
			snapshot := copyOf(m)
			snapshot.Streaming = false
			if m.Role == "assistant" {
				snapshot.Text = stripCodexMemoryCitations(m.Text, true)
			}
			emit(Event{Message: snapshot})
		}
	}
	emit(Event{Done: true, Err: failure})
}

func (s *codexSession) call(method string, params any) error {
	s.mu.Lock()
	s.next++
	id := s.next
	s.calls[id] = method
	s.mu.Unlock()
	return s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func (s *codexSession) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	_, err = s.in.Write(append(b, '\n'))
	return err
}

// answered copies the questions with the answers in the same order.
func answered(qs []Question, answers [][]string) []Question {
	out := append([]Question(nil), qs...)
	for i := range out {
		if i < len(answers) {
			out[i].Answer = answers[i]
		}
	}
	return out
}

// withDiff counts the lines of a unified diff into a file edit.
func withDiff(f FileEdit, diff string) FileEdit {
	var body []string
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"), strings.HasPrefix(l, "@@"):
		case strings.HasPrefix(l, "+"):
			f.Added++
			body = append(body, l)
		case strings.HasPrefix(l, "-"):
			f.Removed++
			body = append(body, l)
		case strings.HasPrefix(l, " "):
			body = append(body, l)
		}
	}
	f.Diff = clip(strings.Join(body, "\n"), 12000)
	return f
}
