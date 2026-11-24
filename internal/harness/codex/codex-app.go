package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

// Start runs a turn over "codex app-server", Codex's JSON-RPC protocol on
// stdio: initialize, start or resume the thread, start the turn. Its
// request_user_input pauses the turn until the user answers. The process
// stays open for the next turn of the thread (see live.go), which only
// starts a turn, so what the agent left running survives.
func (c Codex) Start(ctx context.Context, t harness.Turn) (harness.Session, error) {
	key := strings.Join([]string{t.Model, fmt.Sprint(t.FullAccess), t.Dir, c.Home, t.MCPURL}, "\x00")
	p := kit.ReuseStdio(c.pool, t.ThreadID, c.ID(), key)
	if p == nil {
		args := []string{"app-server", "--enable", "default_mode_request_user_input"}
		cmd := kit.ProviderCommand(context.Background(), t.ThreadID, c.bin(), args...)
		cmd.Dir = t.Dir
		cmd.Env = kit.ThreadEnv(c.Environ(), t.ThreadID)
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
		if p, err = kit.OpenStdio(c.pool, cmd, t.ThreadID, c.ID(), key); err != nil {
			return nil, err
		}
	}
	s := newCodexSession(t, p.Input)
	s.proc = p
	turn := p.Begin(t.ThreadID, s.handle)
	s.events = turn.Events
	// Call ids stay unique on the process across turns.
	s.next = p.Turns * 1_000_000
	p.Watch(ctx, t.ThreadID, c.ID(), turn, s.interrupt)
	var err error
	if thread := p.Get("thread"); thread != "" {
		err = s.startTurn(thread)
	} else {
		err = s.call("initialize", map[string]any{
			"clientInfo":   map[string]any{"name": "dory", "title": "Dory", "version": "2"},
			"capabilities": map[string]any{"experimentalApi": true, "requestAttestation": false},
		})
	}
	if err != nil {
		p.Abort(turn)
		return nil, err
	}
	return s, nil
}

// codexSession drives one turn of codex app-server. Responses to its own
// calls move the handshake on; server requests ask for the user.
type codexSession struct {
	turn harness.Turn

	events    <-chan harness.Event
	proc      *kit.StdioProc
	turnID    string
	lastUsage *codexTokenUsage

	mu      sync.Mutex
	in      io.WriteCloser
	closed  bool
	next    int
	calls   map[int]string // call id → method
	pending map[string]*codexAsk
	texts   map[string]*codexText
	failure string
}

// codexAsk is a request_user_input call waiting for the user.
type codexAsk struct {
	rpcID   json.RawMessage
	message *harness.Message
	answers [][]string
}

func newCodexSession(t harness.Turn, in io.WriteCloser) *codexSession {
	return &codexSession{turn: t, in: in,
		calls: map[int]string{}, pending: map[string]*codexAsk{}, texts: map[string]*codexText{}}
}

func (s *codexSession) Events() <-chan harness.Event { return s.events }

// Respond replies to request_user_input with the answers by question id.
func (s *codexSession) Respond(id string, answers [][]string) error {
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
		return harness.ErrNoRequest
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

func (s *codexSession) handle(line []byte, emit func(harness.Event)) {
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
func (s *codexSession) response(r codexRPC, emit func(harness.Event)) {
	var id int
	_ = json.Unmarshal(r.ID, &id)
	s.mu.Lock()
	method := s.calls[id]
	delete(s.calls, id)
	s.mu.Unlock()
	if r.Error != nil {
		s.finish(emit, harness.TurnEnded{Reason: harness.Failed, Error: method + ": " + r.Error.Message})
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
		emit(harness.SessionBound{ID: res.Thread.ID})
		if res.Model != "" || res.ReasoningEffort != "" {
			emit(harness.SettingsReported{Model: res.Model, Effort: res.ReasoningEffort})
		}
		s.proc.Set("thread", res.Thread.ID)
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
func (s *codexSession) request(r codexRPC, emit func(harness.Event)) {
	switch r.Method {
	case "item/tool/requestUserInput":
		var p struct {
			ItemID    string          `json:"itemId"`
			Questions json.RawMessage `json:"questions"`
		}
		_ = json.Unmarshal(r.Params, &p)
		qs := kit.ParseQuestions(json.RawMessage(`{"questions":` + string(p.Questions) + `}`))
		m := &harness.Message{ID: s.turn.RunID + ":" + p.ItemID, RunID: s.turn.RunID, Role: "tool", Title: "request_user_input",
			Status: "running", CreatedAt: kit.Now(), Tool: &harness.ToolCall{Kind: "question", Name: "request_user_input", Questions: qs}}
		s.mu.Lock()
		s.pending[p.ItemID] = &codexAsk{rpcID: r.ID, message: m}
		s.mu.Unlock()
		emit(kit.StartTool(m))
		emit(harness.RequestOpened{Request: harness.Request{ID: p.ItemID, Kind: "question", MessageID: m.ID, Questions: qs, CreatedAt: kit.Now()}})
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		s.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": map[string]any{"decision": "decline"}})
	default:
		s.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "error": map[string]any{"code": -32601, "message": "Dory does not support " + r.Method}})
	}
}

// codexAppItem is an item of the app-server protocol.
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

type codexTokenUsage struct {
	Input     int64 `json:"inputTokens"`
	Read      int64 `json:"cachedInputTokens"`
	Write     int64 `json:"cacheWriteInputTokens"`
	Output    int64 `json:"outputTokens"`
	Reasoning int64 `json:"reasoningOutputTokens"`
	Total     int64 `json:"totalTokens"`
}

func (s *codexSession) notification(r codexRPC, emit func(harness.Event)) {
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
		emit(harness.StepUsage{Model: s.turn.Model, Input: max(0, u.Input-u.Read-u.Write), CacheRead: u.Read, CacheWrite: u.Write, Output: u.Output, Reasoning: u.Reasoning})
	case "item/started", "item/completed":
		done := r.Method == "item/completed"
		if p.Item.Type == "agentMessage" || p.Item.Type == "reasoning" {
			s.textItem(p.Item, done, emit)
			return
		}
		if m := itemMessage(s.turn.RunID, p.Item, done); m != nil {
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
					var res struct {
						Content []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
					}
					if m.Text == "" && json.Unmarshal(p.Item.Result, &res) == nil {
						var texts []string
						for _, c := range res.Content {
							if c.Type == "text" {
								texts = append(texts, c.Text)
							}
						}
						m.Text = kit.Clip(strings.Join(texts, "\n"), 4000)
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
			if done {
				emit(kit.FinishTool(m))
			} else {
				emit(kit.StartTool(m))
			}
		}
	case "item/agentMessage/delta", "item/reasoning/summaryTextDelta":
		kind := harness.Assistant
		if r.Method == "item/reasoning/summaryTextDelta" {
			kind = harness.Reasoning
		}
		s.text(p.ItemID, kind).append(p.Delta, emit)
	case "turn/plan/updated":
		m := &harness.Message{ID: s.turn.RunID + ":plan", RunID: s.turn.RunID, Role: "plan", CreatedAt: kit.Now(),
			Tool: &harness.ToolCall{Kind: "todo", Name: "update_plan"}}
		var lines []string
		for _, x := range p.Plan {
			st := map[string]string{"inProgress": "in_progress"}[x.Status]
			if st == "" {
				st = x.Status
			}
			m.Tool.Todos = append(m.Tool.Todos, harness.Todo{Text: x.Step, Status: st})
			lines = append(lines, x.Step)
		}
		m.Detail = strings.Join(lines, "\n")
		emit(kit.Plan(m))
	case "serverRequest/resolved":
		// The answer went through: the card shows it.
		s.mu.Lock()
		for id, ask := range s.pending {
			if ask.answers != nil {
				ask.message.Status = "completed"
				ask.message.Tool.Questions = kit.Answered(ask.message.Tool.Questions, ask.answers)
				emit(kit.FinishTool(ask.message))
				delete(s.pending, id)
			}
		}
		s.mu.Unlock()
	case "error":
		if !p.WillRetry {
			s.failure = p.Error.Message
		}
	case "turn/completed":
		end := harness.TurnEnded{Reason: harness.Completed}
		switch p.Turn.Status {
		case "interrupted":
			end.Reason = harness.Interrupted
		case "failed":
			end.Reason, end.Error = harness.Failed, s.failure
			if p.Turn.Error != nil && p.Turn.Error.Message != "" {
				end.Error = p.Turn.Error.Message
			}
			if end.Error == "" {
				end.Error = p.Turn.Status
			}
		}
		s.finish(emit, end)
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
	_ = s.call("turn/interrupt", map[string]any{"threadId": s.proc.Get("thread"), "turnId": turn})
}

// finish ends the turn; the process stays open for the next one.
func (s *codexSession) finish(emit func(harness.Event), end harness.TurnEnded) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	for _, id := range slices.Sorted(maps.Keys(s.texts)) {
		text := s.texts[id]
		if text.String() != "" {
			emit(text.Done(text.String()))
		}
	}
	emit(end)
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
func withDiff(f harness.FileEdit, diff string) harness.FileEdit {
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
	f.Diff = kit.Clip(strings.Join(body, "\n"), 12000)
	return f
}
