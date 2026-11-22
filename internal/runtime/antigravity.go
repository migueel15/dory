package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const antigravityID = "antigravity"

type Antigravity struct{ Bin string }

func (Antigravity) ID() string { return antigravityID }

func antigravityInstallDir(goos, home, localAppData string) string {
	if goos == "windows" {
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "agy", "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

func (a Antigravity) bin() string {
	if a.Bin != "" {
		return a.Bin
	}
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	dir := antigravityInstallDir(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"))
	if path, err := exec.LookPath(filepath.Join(dir, "agy")); err == nil {
		return path
	}
	return "agy"
}

func (a Antigravity) Detect(ctx context.Context) Status {
	version, installed := detect(ctx, a.bin())
	s := Status{ID: a.ID(), Name: "Antigravity", Installed: installed, Version: version,
		Models:       []Model{{Slug: "antigravity-default", Name: "Predeterminado de Antigravity"}},
		Capabilities: []string{"usage"}}
	if !installed {
		return s
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, err := openACP(probe, a.bin(), "", false)
	if err != nil {
		return s
	}
	defer c.close()
	if _, err := c.initialize(probe); err != nil {
		return s
	}
	// Use a temporary probe directory and close the empty session afterward.
	dir, err := os.MkdirTemp("", "dory-agy-models-")
	if err != nil {
		return s
	}
	defer os.RemoveAll(dir)
	dir, err = filepath.Abs(dir)
	if err != nil {
		return s
	}
	var session acpSessionInfo
	if c.call(probe, "session/new", acpParams{"cwd": dir, "mcpServers": []any{}}, &session, nil) == nil {
		s.Models = append(s.Models, session.models()...)
		if session.SessionID != "" {
			_ = c.call(probe, "session/close", acpParams{"sessionId": session.SessionID}, nil, nil)
		}
	}
	return s
}

func (a Antigravity) Session(ctx context.Context) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, err := a.list(ctx, 1)
	if err != nil {
		return false, ""
	}
	return true, "Google"
}

func (a Antigravity) LoginCommand() []string  { return []string{a.bin(), "acp", "--login"} }
func (a Antigravity) UpdateCommand() []string { return []string{a.bin(), "update"} }
func (Antigravity) InstallCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"powershell", "-NoProfile", "-Command", `$installer = Join-Path ([System.IO.Path]::GetTempPath()) ("agy-" + [guid]::NewGuid() + ".cmd"); try { Invoke-WebRequest -ErrorAction Stop -UseBasicParsing https://antigravity.google/cli/install.cmd -OutFile $installer; & $installer; exit $LASTEXITCODE } finally { Remove-Item -LiteralPath $installer -ErrorAction SilentlyContinue }`}
	}
	return []string{"sh", "-c", `installer=$(mktemp) || exit; trap 'rm -f "$installer"' EXIT; curl -fsSL https://antigravity.google/cli/install.sh -o "$installer" && bash "$installer"`}
}

// Start runs a turn on the "agy acp" of its thread. The process and its ACP
// session stay open for the next turn (see live.go), which only sends a
// prompt, so what the agent left running survives the turn.
func (a Antigravity) Start(ctx context.Context, t Turn) (Session, error) {
	key := strings.Join([]string{t.Dir, fmt.Sprint(t.FullAccess), t.MCPURL}, "\x00")
	lv, _ := liveGet(t.ThreadID, "antigravity").(*acpLive)
	if lv != nil && (lv.key != key || lv.Busy() || lv.conn.exited()) {
		liveDrop(t.ThreadID, "antigravity", lv)
		go lv.Close()
		lv = nil
	}
	if lv == nil {
		c, err := openACP(context.Background(), a.bin(), t.Dir, !t.FullAccess, t.ThreadID)
		if err != nil {
			return nil, err
		}
		lv = &acpLive{key: key, conn: c, idle: time.Now()}
		if t.ThreadID != "" {
			liveSet(t.ThreadID, "antigravity", lv)
		}
	}
	lv.mu.Lock()
	lv.busy = true
	lv.mu.Unlock()
	s := newAntigravitySession(ctx, lv.conn, t)
	s.lv = lv
	go func() {
		defer close(s.events)
		err := s.run()
		lv.mu.Lock()
		lv.busy, lv.idle = false, time.Now()
		lv.mu.Unlock()
		if t.ThreadID == "" || lv.conn.exited() {
			liveDrop(t.ThreadID, "antigravity", lv)
			lv.Close()
		}
		done := Event{Done: true}
		if err != nil {
			done.Err = err.Error()
		}
		s.events <- done
	}()
	return s, nil
}

// acpLive is the ACP process of a thread with its session, kept between turns.
type acpLive struct {
	key  string
	conn *acpConn

	mu      sync.Mutex
	busy    bool
	idle    time.Time
	info    acpInitialize
	session acpSessionInfo
	id      string
}

func (l *acpLive) Busy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.busy
}

func (l *acpLive) IdleSince() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.idle
}

func (l *acpLive) Close() { l.conn.stop() }

func (s *antigravitySession) run() error {
	setup, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	if s.lv == nil {
		s.lv = &acpLive{conn: s.conn}
	}
	lv := s.lv
	lv.mu.Lock()
	info, session, id := lv.info, lv.session, lv.id
	lv.mu.Unlock()
	if id == "" {
		var err error
		if info, err = s.conn.initialize(setup); err != nil {
			return err
		}
		servers, err := antigravityMCP(info, s.turn)
		if err != nil {
			return err
		}
		params := acpParams{"cwd": s.turn.Dir, "mcpServers": servers}
		if s.turn.Instructions != "" && info.AgentCapabilities.Meta.Antigravity.SystemPrompt.Append {
			params["_meta"] = acpParams{"antigravity": acpParams{"systemPrompt": acpParams{"append": s.turn.Instructions}}}
		}
		method := "session/new"
		if s.turn.Session != "" {
			method = "session/load"
			params["sessionId"] = s.turn.Session
		}
		if err := s.conn.call(setup, method, params, &session, s.handle); err != nil {
			var rpcErr *acpError
			if errors.As(err, &rpcErr) && (rpcErr.Code == -32000 && strings.Contains(strings.ToLower(rpcErr.Message), "auth")) {
				return fmt.Errorf("inicia sesión en Antigravity con agy acp --login: %w", err)
			}
			return err
		}
		id = session.SessionID
		if id == "" {
			id = s.turn.Session
		}
		if id == "" {
			return errors.New("Antigravity no devolvió el identificador de sesión")
		}
		lv.mu.Lock()
		lv.info, lv.session, lv.id = info, session, id
		lv.mu.Unlock()
	}
	s.id = id
	s.emit(Event{Session: s.id})
	if err := s.configure(setup, session); err != nil {
		return err
	}
	s.live = true
	text := s.turn.Text
	if !info.AgentCapabilities.Meta.Antigravity.SystemPrompt.Append {
		text = promptWithInstructions(s.turn)
	}
	blocks := []acpParams{{"type": "text", "text": text}}
	// Keep inline media bounded. The prompt also contains the attachment paths.
	remaining := int64(8 << 20)
	for _, file := range s.turn.Attachments {
		if info.AgentCapabilities.PromptCapabilities.Image && strings.HasPrefix(file.Mime, "image/") {
			if st, err := os.Stat(file.Path); err != nil || st.Size() > remaining {
				continue
			}
			if raw, err := os.ReadFile(file.Path); err == nil && int64(len(raw)) <= remaining {
				blocks = append(blocks, acpParams{"type": "image", "mimeType": file.Mime, "data": base64.StdEncoding.EncodeToString(raw)})
				remaining -= int64(len(raw))
			}
		}
	}
	prompt := s.conn.next + 1
	err := s.conn.call(s.ctx, "session/prompt", acpParams{"sessionId": s.id, "prompt": blocks}, nil, s.handle)
	if s.ctx.Err() != nil {
		_ = s.conn.send(acpParams{"method": "session/cancel", "params": acpParams{"sessionId": s.id}})
		// The next turn must not read what is left of this prompt.
		if !s.conn.drain(prompt, 10*time.Second) {
			s.lv.conn.stop()
		}
	}
	s.finishChunk()
	return err
}

func antigravityMCP(info acpInitialize, t Turn) ([]acpParams, error) {
	servers := []acpParams{}
	if t.MCPURL == "" {
		return servers, nil
	}
	if info.AgentCapabilities.MCPCapabilities.HTTP {
		return append(servers, acpParams{"name": "dory", "type": "http", "url": t.MCPURL,
			"headers": []acpParams{{"name": "Authorization", "value": "Bearer " + t.MCPToken}}}), nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return append(servers, acpParams{"name": "dory", "command": self, "args": []string{"mcp-stdio", t.MCPURL},
		"env": []acpParams{{"name": "DORY_MCP_TOKEN", "value": t.MCPToken}}}), nil
}
