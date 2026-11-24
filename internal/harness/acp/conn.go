package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/migueel15/dory/internal/harness/kit"
)

type params map[string]any

type rpcFrame struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  *rpcError
}

type rpcError struct {
	Name    string
	Code    int
	Message string
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d): %s", e.Name, e.Code, e.Message) }

// conn is a JSON-RPC 2.0 client over the stdio of one agent process.
type conn struct {
	name   string
	cmd    *exec.Cmd // nil when the peer is not a process (tests)
	in     io.WriteCloser
	frames chan rpcFrame
	done   chan struct{}
	cancel context.CancelFunc
	mu     sync.Mutex
	next   int
	err    error // read after frames closes
}

// open starts the agent of a profile. args are the ones after the binary.
func open(ctx context.Context, name, bin string, args, env []string, dir string, threadID string) (*conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := kit.ProviderCommand(ctx, threadID, bin, args...)
	cmd.Dir = dir
	if len(env) > 0 || threadID != "" {
		cmd.Env = kit.ThreadEnv(append(os.Environ(), env...), threadID)
	}
	interruptible(cmd)
	cmd.WaitDelay = 3 * time.Second
	var stderr strings.Builder
	cmd.Stderr = kit.LimitWriter(&stderr, 8<<10)
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		in.Close()
		cancel()
		return nil, err
	}
	c := newConn(ctx, cancel, name, in, out, func() error {
		err := cmd.Wait()
		if msg := kit.Short(stderr.String(), 500); err != nil && msg != "" {
			return fmt.Errorf("%s: %s", name, msg)
		}
		return err
	})
	c.cmd = cmd
	return c, nil
}

// newConn reads frames from out until it ends; wait reports how the peer ended.
func newConn(ctx context.Context, cancel context.CancelFunc, name string, in io.WriteCloser, out io.Reader, wait func() error) *conn {
	c := &conn{name: name, in: in, frames: make(chan rpcFrame, 32), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(c.done)
		defer close(c.frames)
		scan := bufio.NewScanner(out)
		scan.Buffer(make([]byte, 64<<10), 32<<20)
		for scan.Scan() {
			var f rpcFrame
			if json.Unmarshal(scan.Bytes(), &f) != nil {
				continue // Some versions print login instructions on stdout.
			}
			if f.Error != nil {
				f.Error.Name = name
			}
			select {
			case c.frames <- f:
			case <-ctx.Done():
				wait()
				c.err = ctx.Err()
				return
			}
		}
		if scan.Err() != nil {
			cancel()
		}
		c.err = wait()
		if scan.Err() != nil {
			c.err = scan.Err()
		}
	}()
	return c
}

func (c *conn) close() {
	c.in.Close()
	select {
	case <-c.done:
	case <-time.After(200 * time.Millisecond):
		c.cancel()
		<-c.done
	}
	c.cancel()
}

func (c *conn) exited() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// stop ends the process group, with what the agent started.
func (c *conn) stop() {
	c.in.Close()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		if c.cmd != nil {
			stopGroup(c.cmd, syscall.SIGTERM)
		}
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			if c.cmd != nil {
				stopGroup(c.cmd, syscall.SIGKILL)
			}
		}
	}
	c.cancel()
}

// drain discards frames until the response to call id, so a cancelled
// prompt leaves nothing for the next one. It reports whether it arrived.
func (c *conn) drain(id int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return false
		case f, ok := <-c.frames:
			if !ok {
				return false
			}
			if f.Method != "" && len(f.ID) != 0 {
				_ = c.unsupported(f)
			}
			if f.Method == "" && string(f.ID) == strconv.Itoa(id) {
				return true
			}
		}
	}
}

func (c *conn) send(f params) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f["jsonrpc"] = "2.0"
	return json.NewEncoder(c.in).Encode(f)
}

func (c *conn) reply(id json.RawMessage, result any) error {
	return c.send(params{"id": id, "result": result})
}

// call is driven by a single reader. Incoming requests can be answered while
// waiting for a reply, including requests that pause the current prompt.
func (c *conn) call(ctx context.Context, method string, p any, result any, handle func(rpcFrame) error) error {
	c.next++
	id := c.next
	if err := c.send(params{"id": id, "method": method, "params": p}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f, ok := <-c.frames:
			if !ok {
				if c.err != nil {
					return c.err
				}
				return fmt.Errorf("%s: %s: %w", c.name, method, io.ErrUnexpectedEOF)
			}
			if f.Method != "" {
				if handle != nil {
					if err := handle(f); err != nil {
						return err
					}
				} else if len(f.ID) != 0 {
					if err := c.unsupported(f); err != nil {
						return err
					}
				}
				continue
			}
			if string(f.ID) != strconv.Itoa(id) {
				continue
			}
			if f.Error != nil {
				return f.Error
			}
			if result != nil {
				return json.Unmarshal(f.Result, result)
			}
			return nil
		}
	}
}

func (c *conn) unsupported(f rpcFrame) error {
	return c.send(params{"id": f.ID, "error": params{"code": -32601, "message": "Unsupported client method"}})
}

// Initialize is what an agent answers to the initialize request.
type Initialize struct {
	ProtocolVersion   int
	AgentInfo         struct{ Name string }
	AgentCapabilities struct {
		LoadSession        bool
		MCPCapabilities    struct{ HTTP bool }
		PromptCapabilities struct{ Image, EmbeddedContext bool }
		// Meta is the agent's own extension block; a profile reads it.
		Meta json.RawMessage `json:"_meta"`
	}
	AuthMethods []struct{ ID string }
}

// initialize negotiates the protocol and checks the server against p.
func (c *conn) initialize(ctx context.Context, p *Profile) (Initialize, error) {
	var info Initialize
	err := c.call(ctx, "initialize", params{
		"protocolVersion":    1,
		"clientInfo":         params{"name": "dory", "title": "Dory", "version": "1"},
		"clientCapabilities": params{"elicitation": params{"form": params{}}},
	}, &info, nil)
	if err != nil {
		return info, err
	}
	if len(p.Agents) > 0 {
		known := false
		for _, name := range p.Agents {
			known = known || name == info.AgentInfo.Name
		}
		if !known {
			return info, fmt.Errorf("servidor ACP inesperado: %q", info.AgentInfo.Name)
		}
	}
	if info.ProtocolVersion != 1 || (p.RequireLoad && !info.AgentCapabilities.LoadSession) {
		return info, fmt.Errorf(i18n.Text("acp.esta_version_no_admite"), c.name)
	}
	return info, nil
}
