package runtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Start runs a turn on the "opencode serve" of its thread: Dory opens the
// session, follows its events and sends the prompt over the HTTP API. The
// question tool opens a form that pauses the turn until the user replies.
// The server stays open for the next turn of the thread (see live.go), so
// what the agent left running survives the turn.
func (o OpenCode) Start(ctx context.Context, t Turn) (Session, error) {
	srv, _ := liveGet(t.ThreadID, "opencode").(*openCodeServer)
	if srv != nil && (srv.key != t.Dir || srv.Busy() || srv.exited()) {
		liveDrop(t.ThreadID, "opencode", srv)
		go srv.Close()
		srv = nil
	}
	if srv == nil {
		var err error
		if srv, err = o.serve(t); err != nil {
			return nil, err
		}
		if t.ThreadID != "" {
			liveSet(t.ThreadID, "opencode", srv)
		}
	}
	srv.mu.Lock()
	srv.busy = true
	srv.mu.Unlock()
	events := make(chan Event, 64)
	s := &openCodeSession{api: srv.api, turn: t, events: events,
		parser: newOpenCodeEvents(t.RunID)}
	go func() {
		s.run(ctx, events)
		srv.mu.Lock()
		srv.busy, srv.idle = false, time.Now()
		srv.mu.Unlock()
		if t.ThreadID == "" {
			srv.Close()
		}
	}()
	return s, nil
}

// serve starts an "opencode serve" for the thread.
func (o OpenCode) serve(t Turn) (*openCodeServer, error) {
	password := rand.Text()
	cmd := providerCommand(context.Background(), t.ThreadID, o.bin(), "serve", "--hostname", "127.0.0.1", "--port", "0")
	cmd.Dir = t.Dir
	cmd.Env = append(os.Environ(), "OPENCODE_SERVER_PASSWORD="+password)
	cmd.Env = threadEnv(cmd.Env, t.ThreadID)
	interruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limited{w: &stderr, n: 8 << 10}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	base, err := listening(out, 30*time.Second)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("opencode serve: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	go io.Copy(io.Discard, out)
	srv := &openCodeServer{key: t.Dir, cmd: cmd, api: &openCodeAPI{base: base, password: password}, dead: make(chan struct{}), idle: time.Now()}
	go func() {
		cmd.Wait()
		close(srv.dead)
		liveDrop(t.ThreadID, "opencode", srv)
	}()
	return srv, nil
}

// openCodeServer is the "opencode serve" of a thread, kept between turns.
type openCodeServer struct {
	key  string
	cmd  *exec.Cmd
	api  *openCodeAPI
	dead chan struct{}

	mu   sync.Mutex
	busy bool
	idle time.Time
}

func (s *openCodeServer) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

func (s *openCodeServer) IdleSince() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idle
}

func (s *openCodeServer) exited() bool {
	select {
	case <-s.dead:
		return true
	default:
		return false
	}
}

// Close stops the server's process group, with what the agent started.
func (s *openCodeServer) Close() {
	stopGroup(s.cmd, syscall.SIGTERM)
	select {
	case <-s.dead:
	case <-time.After(5 * time.Second):
		stopGroup(s.cmd, syscall.SIGKILL)
	}
}

// listening reads the server banner until it says where it listens.
func listening(r io.Reader, timeout time.Duration) (string, error) {
	found := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(r)
		for scan.Scan() {
			if _, url, ok := strings.Cut(scan.Text(), "listening on "); ok {
				found <- strings.TrimSpace(url)
				return
			}
		}
		close(found)
	}()
	select {
	case url, ok := <-found:
		if !ok {
			return "", errors.New("the server exited")
		}
		return url, nil
	case <-time.After(timeout):
		return "", errors.New("the server did not start")
	}
}

// openCodeAPI calls the HTTP API of one opencode serve.
type openCodeAPI struct {
	base, password string
}

func (a *openCodeAPI) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth("opencode", a.password)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return fmt.Errorf("%s %s: %s %s", method, path, res.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

type openCodeSession struct {
	api    *openCodeAPI
	turn   Turn
	events chan Event
	parser *openCodeEvents

	mu      sync.Mutex
	session string
	forms   map[string]openCodeForm
}

// openCodeForm is a form of the question tool waiting for the user.
type openCodeForm struct {
	fields []openCodeField
}

func (s *openCodeSession) Events() <-chan Event { return s.events }

// Answer replies to the form: one value per field key, a list for
// multiselect fields.
func (s *openCodeSession) Answer(id string, answers [][]string) error {
	s.mu.Lock()
	form, ok := s.forms[id]
	delete(s.forms, id)
	session := s.session
	s.mu.Unlock()
	if !ok {
		return ErrNoRequest
	}
	answer := map[string]any{}
	for i, f := range form.fields {
		var a []string
		if i < len(answers) {
			a = answers[i]
		}
		if f.Type == "multiselect" {
			answer[f.Key] = append([]string{}, a...)
		} else {
			answer[f.Key] = strings.Join(a, ", ")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.api.do(ctx, "POST", "/api/session/"+session+"/form/"+id+"/reply", map[string]any{"answer": answer}, nil)
}

// run opens or resumes the session, follows its events, sends the prompt
// and stops the server when the turn ends.
func (s *openCodeSession) run(ctx context.Context, events chan<- Event) {
	defer close(events)
	// The stream outlives a cancellation until the session confirms it stopped.
	streamCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	defer stop()
	fail := func(err error) {
		msg := err.Error()
		if ctx.Err() != nil {
			msg = "interrumpido"
		}
		events <- Event{Done: true, Err: msg}
	}
	t := s.turn
	session := t.Session
	if session == "" {
		var res struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := s.api.do(ctx, "POST", "/api/session", map[string]any{}, &res); err != nil {
			fail(err)
			return
		}
		session = res.Data.ID
	}
	s.mu.Lock()
	s.session, s.forms = session, map[string]openCodeForm{}
	s.mu.Unlock()
	events <- Event{Session: session}
	if provider, model, ok := strings.Cut(t.Model, "/"); ok {
		if err := s.api.do(ctx, "POST", "/api/session/"+session+"/model", map[string]any{"model": map[string]string{"providerID": provider, "id": model}}, nil); err != nil {
			fail(err)
			return
		}
	}
	req, err := http.NewRequestWithContext(streamCtx, "GET", s.api.base+"/api/event", nil)
	if err != nil {
		fail(err)
		return
	}
	req.SetBasicAuth("opencode", s.api.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(err)
		return
	}
	defer res.Body.Close()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-ctx.Done():
			// Interrupting is a request to the server, which stays open.
			bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if s.api.do(bg, "POST", "/api/session/"+session+"/interrupt", map[string]any{}, nil) != nil {
				stop()
			}
			cancel()
			select {
			case <-streamCtx.Done():
			case <-time.After(10 * time.Second):
				stop()
			}
		}
	}()
	go func() {
		if err := s.api.do(streamCtx, "POST", "/api/session/"+session+"/prompt", map[string]any{"text": t.Text}, nil); err != nil {
			s.parser.failed(err.Error())
			stop()
		}
	}()
	scan := bufio.NewScanner(res.Body)
	scan.Buffer(make([]byte, 64<<10), 32<<20)
	done := false
	for scan.Scan() && !done {
		data, ok := strings.CutPrefix(scan.Text(), "data:")
		if !ok {
			continue
		}
		s.parser.handle(session, []byte(strings.TrimSpace(data)), func(e Event) {
			if e.Request != nil {
				s.mu.Lock()
				s.forms[e.Request.ID] = openCodeForm{fields: s.parser.fields[e.Request.ID]}
				s.mu.Unlock()
			}
			done = done || e.Done
			events <- e
		})
	}
	if !done {
		s.parser.mu.Lock()
		msg := s.parser.failure
		s.parser.mu.Unlock()
		if ctx.Err() != nil {
			msg = "interrumpido"
		}
		if msg == "" {
			msg = "el agente terminó sin respuesta"
		}
		events <- Event{Done: true, Err: msg}
	}
}

// openCodeEvents turns the events of one session of opencode serve into
// Dory's messages.
type openCodeEvents struct {
	run     string
	names   map[string]string // tool call id → tool name
	tools   map[string]*Message
	texts   map[string]*Message
	fields  map[string][]openCodeField // form id → fields
	mu      sync.Mutex
	failure string
}

type openCodeField struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Options     []struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

func newOpenCodeEvents(run string) *openCodeEvents {
	return &openCodeEvents{run: run, names: map[string]string{}, tools: map[string]*Message{}, texts: map[string]*Message{}, fields: map[string][]openCodeField{}}
}

func (p *openCodeEvents) failed(msg string) {
	p.mu.Lock()
	p.failure = msg
	p.mu.Unlock()
}

type openCodeEvent struct {
	Type string `json:"type"`
	Data struct {
		SessionID          string          `json:"sessionID"`
		AssistantMessageID string          `json:"assistantMessageID"`
		Ordinal            int             `json:"ordinal"`
		Delta              string          `json:"delta"`
		Text               string          `json:"text"`
		ID                 string          `json:"id"`
		Name               string          `json:"name"`
		Input              json.RawMessage `json:"input"`
		Content            []struct {
			Text string `json:"text"`
		} `json:"content"`
		Metadata struct {
			Answers [][]string `json:"answers"`
		} `json:"metadata"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Model *struct {
			ID         string `json:"id"`
			ProviderID string `json:"providerID"`
		} `json:"model"`
		Form struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionID"`
			Metadata  struct {
				Tool struct {
					ID string `json:"id"`
				} `json:"tool"`
			} `json:"metadata"`
			Fields []openCodeField `json:"fields"`
		} `json:"form"`
	} `json:"data"`
}

func (p *openCodeEvents) handle(session string, line []byte, emit func(Event)) {
	var e openCodeEvent
	if json.Unmarshal(line, &e) != nil {
		return
	}
	d := e.Data
	if d.SessionID != session && d.Form.SessionID != session {
		return
	}
	textID := fmt.Sprintf("%s:%s:%d", p.run, d.AssistantMessageID, d.Ordinal)
	switch e.Type {
	case "session.step.started":
		if d.Model != nil && d.Model.ID != "" {
			emit(Event{Settings: &ReportedSettings{Model: d.Model.ProviderID + "/" + d.Model.ID}})
		}
	case "session.text.started", "session.reasoning.started":
		m := &Message{ID: textID, RunID: p.run, Role: "assistant", Streaming: true, CreatedAt: now()}
		if e.Type == "session.reasoning.started" {
			m.ID, m.Role, m.Title = textID+":r", "reasoning", "Pensando"
		}
		p.texts[m.ID] = m
	case "session.text.delta", "session.reasoning.delta", "session.text.ended", "session.reasoning.ended":
		id := textID
		if strings.HasPrefix(e.Type, "session.reasoning") {
			id += ":r"
		}
		m := p.texts[id]
		if m == nil {
			return
		}
		if strings.HasSuffix(e.Type, ".ended") {
			m.Text, m.Streaming = d.Text, false
		} else {
			m.Text += d.Delta
		}
		if m.Text != "" {
			emit(Event{Message: copyOf(m)})
		}
	case "session.tool.input.started":
		p.names[d.ID] = d.Name
	case "session.tool.called":
		name := p.names[d.ID]
		m := &Message{ID: p.run + ":" + d.ID, RunID: p.run, Role: "tool", Title: name, Detail: toolDetail(name, d.Input),
			Status: "running", CreatedAt: now(), Tool: toolCall(name, d.Input)}
		if m.Tool.Kind == "todo" {
			m.Role = "plan"
		}
		p.tools[d.ID] = m
		emit(Event{Message: copyOf(m)})
	case "session.tool.success", "session.tool.failed":
		m := p.tools[d.ID]
		if m == nil {
			return
		}
		var parts []string
		for _, c := range d.Content {
			parts = append(parts, c.Text)
		}
		m.Status, m.Text = "completed", clip(strings.Join(parts, "\n"), 4000)
		if e.Type == "session.tool.failed" {
			m.Status = "failed"
			if d.Error != nil && m.Text == "" {
				m.Text = d.Error.Message
			}
		}
		if m.Tool.Kind == "question" && len(d.Metadata.Answers) > 0 {
			m.Tool.Questions = answered(m.Tool.Questions, d.Metadata.Answers)
			m.Text = ""
		}
		emit(Event{Message: copyOf(m)})
	case "form.created":
		f := d.Form
		var qs []Question
		for _, x := range f.Fields {
			q := Question{ID: x.Key, Header: x.Title, Question: x.Description, MultiSelect: x.Type == "multiselect"}
			if q.Question == "" {
				q.Question, q.Header = x.Title, ""
			}
			for _, o := range x.Options {
				q.Options = append(q.Options, Option{Label: o.Label, Description: o.Description})
			}
			qs = append(qs, q)
		}
		p.fields[f.ID] = f.Fields
		msg := ""
		if f.Metadata.Tool.ID != "" {
			msg = p.run + ":" + f.Metadata.Tool.ID
		}
		emit(Event{Request: &Request{ID: f.ID, Kind: "question", MessageID: msg, Questions: qs, CreatedAt: now()}})
	case "session.execution.succeeded":
		p.close(emit, "")
	case "session.execution.failed", "session.execution.interrupted":
		msg := "interrumpido"
		if d.Error != nil {
			msg = d.Error.Message
		}
		p.close(emit, msg)
	}
}

func (p *openCodeEvents) close(emit func(Event), failure string) {
	for _, m := range p.texts {
		if m.Streaming {
			m.Streaming = false
			emit(Event{Message: copyOf(m)})
		}
	}
	emit(Event{Done: true, Err: failure})
}
