// Package acp is the generic adapter for harnesses that speak the Agent
// Client Protocol (ACP) over stdio. A Profile describes one harness; Antigravity
// is the first, and any other ACP agent can be declared without new code.
package acp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/procpool"
)

// Profile describes one ACP harness. Only ID, Name and Bins are required.
type Profile struct {
	ID, Name string
	Order    int
	// Bins are the executables to look for, in order; a bare name is searched
	// in PATH. The first Bins entry is used when none exists.
	Bins []string
	// Args follow the binary to start the ACP server; SandboxArgs are added
	// when the turn does not have full access. Env are extra KEY=VALUE pairs.
	Args, SandboxArgs, Env []string
	// Agents lists the agentInfo names the server may report; empty accepts any.
	Agents []string
	// RequireLoad refuses servers that cannot load a saved session.
	RequireLoad bool
	// Login and Update are the arguments after the binary; Install is a whole
	// command. All are optional.
	Login, Update, Install []string
	// DefaultModel is the model that leaves the choice to the agent.
	DefaultModel harness.Model
	// Catalog lists the models of the agent in a probe session when detecting it.
	Catalog bool
	// Sessions says the agent lists and replays its saved sessions (session/list
	// and session/load); Dory then offers them as external conversations.
	Sessions bool
	// Account is the name shown as the signed-in account when session/list answers.
	Account      string
	Capabilities harness.Capabilities
	ProcessNames []string

	// SystemPrompt returns the _meta of session/new that carries the session
	// instructions, from the agent's own initialize _meta. Without it the
	// instructions precede the first prompt.
	SystemPrompt func(agentMeta []byte, instructions string) (meta map[string]any, ok bool)
	// MCPConfig builds mcpServers of session/new; the standard ACP form by default.
	MCPConfig     func(info Initialize, t harness.Turn) ([]map[string]any, error)
	LatestVersion func(ctx context.Context) string
	Usage         func(ctx context.Context, bin string) ([]harness.Window, error)
	History       func(p harness.Paths, from time.Time) []harness.UsageEvent
	// TurnUsage reads the token counters of one turn after it ended.
	TurnUsage func(session string, start, end time.Time) []harness.StepUsage
	Commands  func(p harness.Paths, dir string) (commands, skills []harness.Entry)
	SkillDir  func(p harness.Paths) harness.SkillLocation
}

// Declared is the profile of an agent the user declared in config.toml. It has
// no quota, catalog or history: the agent decides its own default model.
func Declared(id, name, command string, args, env []string) Profile {
	if name == "" {
		name = id
	}
	return Profile{
		ID: id, Name: name, Order: 100,
		Bins: []string{command}, Args: args, Env: env,
		Capabilities: harness.Capabilities{Resume: true, MidTurnRequests: true},
	}
}

// probePrefix names the temporary directories of the catalog probe.
const probePrefix = "dory-acp-models-"

// legacyProbePrefix is what the first version of the adapter used.
const legacyProbePrefix = "dory-agy-models-"

const sessionListLimit = 150

type driver struct {
	p    Profile
	pool *procpool.Pool
}

// rich is the driver of a profile with administration, usage, history or
// external sessions; the registry discovers those by interface.
type rich struct{ *driver }

// New returns the driver of a profile. The registry hands it its process pool.
func New(p Profile, pool *procpool.Pool) harness.Driver {
	if p.DefaultModel.Slug == "" {
		p.DefaultModel = harness.Model{Slug: p.ID + "-default", Name: "Predeterminado de " + p.Name}
	}
	return wrap(&driver{p: p, pool: pool})
}

func wrap(d *driver) harness.Driver {
	p := d.p
	if p.Login != nil || p.Update != nil || p.Install != nil || p.Sessions || p.Usage != nil || p.History != nil ||
		p.Commands != nil || p.SkillDir != nil || p.LatestVersion != nil {
		return rich{d}
	}
	return d
}

func (d *driver) WithPool(pool *procpool.Pool) harness.Driver {
	c := *d
	c.pool = pool
	return wrap(&c)
}

func (r rich) WithPool(pool *procpool.Pool) harness.Driver { return r.driver.WithPool(pool) }

func (d *driver) Info() harness.Info {
	return harness.Info{ID: d.p.ID, Driver: d.p.ID, Name: d.p.Name, Order: d.p.Order, Capabilities: d.p.Capabilities}
}

func (d *driver) ProcessNames() []string { return d.p.ProcessNames }

func (d *driver) bin() string {
	for _, name := range d.p.Bins {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if len(d.p.Bins) > 0 {
		return d.p.Bins[0]
	}
	return d.p.ID
}

func (d *driver) args(sandbox bool) []string {
	args := append([]string{}, d.p.Args...)
	if sandbox {
		args = append(args, d.p.SandboxArgs...)
	}
	return args
}

func (d *driver) open(ctx context.Context, dir string, sandbox bool, threadID string) (*conn, error) {
	return open(ctx, d.p.Name, d.bin(), d.args(sandbox), d.p.Env, dir, threadID)
}

func (d *driver) Detect(ctx context.Context) harness.Status {
	version, installed := detectVersion(ctx, d.bin())
	s := harness.Status{ID: d.p.ID, Name: d.p.Name, Installed: installed, Version: version, Models: []harness.Model{d.p.DefaultModel}}
	if !installed || !d.p.Catalog {
		return s
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, err := d.open(probe, "", false, "")
	if err != nil {
		return s
	}
	defer c.close()
	if _, err := c.initialize(probe, &d.p); err != nil {
		return s
	}
	// Use a temporary probe directory and close the empty session afterward.
	dir, err := os.MkdirTemp("", probePrefix)
	if err != nil {
		return s
	}
	defer os.RemoveAll(dir)
	dir, err = filepath.Abs(dir)
	if err != nil {
		return s
	}
	var session sessionInfo
	if c.call(probe, "session/new", params{"cwd": dir, "mcpServers": []any{}}, &session, nil) == nil {
		s.Models = append(s.Models, session.models()...)
		if session.SessionID != "" {
			_ = c.call(probe, "session/close", params{"sessionId": session.SessionID}, nil, nil)
		}
	}
	return s
}

// Start runs a turn on the ACP server of its thread. The process and its ACP
// session stay open for the next turn, which only sends a prompt, so what the
// agent left running survives the turn.
func (d *driver) Start(ctx context.Context, t harness.Turn) (harness.Session, error) {
	pooled := d.pool != nil && t.ThreadID != ""
	key := strings.Join([]string{t.Dir, fmt.Sprint(t.FullAccess), t.MCPURL}, "\x00")
	var lv *liveProc
	if pooled {
		lv, _ = d.pool.Get(t.ThreadID, d.p.ID).(*liveProc)
		if lv != nil && (lv.key != key || lv.Busy() || lv.conn.exited()) {
			d.pool.Drop(t.ThreadID, d.p.ID, lv)
			go lv.Close()
			lv = nil
		}
	}
	if lv == nil {
		c, err := d.open(context.Background(), t.Dir, !t.FullAccess, t.ThreadID)
		if err != nil {
			return nil, err
		}
		lv = &liveProc{key: key, conn: c, idle: time.Now()}
		if pooled {
			d.pool.Set(t.ThreadID, d.p.ID, lv)
		}
	}
	lv.mu.Lock()
	lv.busy = true
	lv.mu.Unlock()
	s := newSession(ctx, lv.conn, t, d.p.DefaultModel.Slug)
	s.lv = lv
	go func() {
		defer close(s.events)
		err := d.run(s)
		lv.mu.Lock()
		lv.busy, lv.idle = false, time.Now()
		lv.mu.Unlock()
		if !pooled || lv.conn.exited() {
			if pooled {
				d.pool.Drop(t.ThreadID, d.p.ID, lv)
			}
			lv.Close()
		}
		done := event{Done: true}
		if err != nil {
			done.Err = err.Error()
		}
		s.events <- done
	}()
	return harness.Adapt(s.Events(), s.Answer), nil
}

// liveProc is the ACP process of a thread with its session, kept between turns.
type liveProc struct {
	key  string
	conn *conn

	mu      sync.Mutex
	busy    bool
	idle    time.Time
	info    Initialize
	session sessionInfo
	id      string
}

func (l *liveProc) Busy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.busy
}

func (l *liveProc) IdleSince() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.idle
}

func (l *liveProc) Close() { l.conn.stop() }

func (d *driver) run(s *session) error {
	setup, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	if s.lv == nil {
		s.lv = &liveProc{conn: s.conn}
	}
	lv := s.lv
	lv.mu.Lock()
	info, session, id := lv.info, lv.session, lv.id
	lv.mu.Unlock()
	if id == "" {
		var err error
		if info, err = s.conn.initialize(setup, &d.p); err != nil {
			return err
		}
		mcp := d.p.MCPConfig
		if mcp == nil {
			mcp = standardMCP
		}
		servers, err := mcp(info, s.turn)
		if err != nil {
			return err
		}
		sp := map[string]any{"cwd": s.turn.Dir, "mcpServers": servers}
		if s.turn.Instructions != "" && d.p.SystemPrompt != nil {
			if m, ok := d.p.SystemPrompt(info.AgentCapabilities.Meta, s.turn.Instructions); ok {
				sp["_meta"] = m
			}
		}
		method := "session/new"
		if s.turn.Session != "" && info.AgentCapabilities.LoadSession {
			method = "session/load"
			sp["sessionId"] = s.turn.Session
		}
		if err := s.conn.call(setup, method, sp, &session, s.handle); err != nil {
			var rpcErr *rpcError
			if errors.As(err, &rpcErr) && (rpcErr.Code == -32000 && strings.Contains(strings.ToLower(rpcErr.Message), "auth")) {
				if len(d.p.Login) > 0 {
					return fmt.Errorf("inicia sesión en %s con %s: %w", d.p.Name, strings.Join(append([]string{filepath.Base(d.bin())}, d.p.Login...), " "), err)
				}
				return fmt.Errorf("inicia sesión en %s: %w", d.p.Name, err)
			}
			return err
		}
		id = session.SessionID
		if id == "" && method == "session/load" {
			id = s.turn.Session
		}
		if id == "" {
			return fmt.Errorf("%s no devolvió el identificador de sesión", d.p.Name)
		}
		lv.mu.Lock()
		lv.info, lv.session, lv.id = info, session, id
		lv.mu.Unlock()
	}
	s.id = id
	s.emit(event{Session: s.id})
	if err := s.configure(setup, session); err != nil {
		return err
	}
	s.live = true
	text := d.promptText(info, s.turn)
	blocks := []params{{"type": "text", "text": text}}
	// Keep inline media bounded. The prompt also contains the attachment paths.
	remaining := int64(8 << 20)
	for _, file := range s.turn.Attachments {
		if info.AgentCapabilities.PromptCapabilities.Image && strings.HasPrefix(file.Mime, "image/") {
			if st, err := os.Stat(file.Path); err != nil || st.Size() > remaining {
				continue
			}
			if raw, err := os.ReadFile(file.Path); err == nil && int64(len(raw)) <= remaining {
				blocks = append(blocks, params{"type": "image", "mimeType": file.Mime, "data": base64.StdEncoding.EncodeToString(raw)})
				remaining -= int64(len(raw))
			}
		}
	}
	started := time.Now()
	prompt := s.conn.next + 1
	err := s.conn.call(s.ctx, "session/prompt", params{"sessionId": s.id, "prompt": blocks}, nil, s.handle)
	if s.ctx.Err() != nil {
		_ = s.conn.send(params{"method": "session/cancel", "params": params{"sessionId": s.id}})
		// The next turn must not read what is left of this prompt.
		if !s.conn.drain(prompt, 10*time.Second) {
			s.lv.conn.stop()
		}
	}
	s.finishChunk()
	if d.p.TurnUsage != nil {
		if steps := d.p.TurnUsage(s.id, started, time.Now()); len(steps) > 0 {
			s.events <- event{Steps: steps}
		}
	}
	return err
}

// promptText is the user text, with the session instructions in front when the
// agent has no channel for them and the session is new.
func (d *driver) promptText(info Initialize, t harness.Turn) string {
	if t.Instructions == "" || t.Session != "" {
		return t.Text
	}
	if d.p.SystemPrompt != nil {
		if _, ok := d.p.SystemPrompt(info.AgentCapabilities.Meta, t.Instructions); ok {
			return t.Text
		}
	}
	return "<dory_instructions>\n" + t.Instructions + "\n</dory_instructions>\n\n" + t.Text
}

// standardMCP points the agent at the Dory MCP: over HTTP when the agent
// accepts it, through "dory mcp-stdio" otherwise.
func standardMCP(info Initialize, t harness.Turn) ([]map[string]any, error) {
	servers := []map[string]any{}
	if t.MCPURL == "" {
		return servers, nil
	}
	if info.AgentCapabilities.MCPCapabilities.HTTP {
		return append(servers, map[string]any{"name": "dory", "type": "http", "url": t.MCPURL,
			"headers": []map[string]any{{"name": "Authorization", "value": "Bearer " + t.MCPToken}}}), nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return append(servers, map[string]any{"name": "dory", "command": self, "args": []string{"mcp-stdio", t.MCPURL},
		"env": []map[string]any{{"name": "DORY_MCP_TOKEN", "value": t.MCPToken}}}), nil
}
