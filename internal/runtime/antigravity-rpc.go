package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

type acpParams map[string]any

type acpFrame struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  *acpError
}

type acpError struct {
	Code    int
	Message string
}

func (e *acpError) Error() string { return fmt.Sprintf("Antigravity (%d): %s", e.Code, e.Message) }

type acpConn struct {
	in     io.WriteCloser
	frames chan acpFrame
	done   chan struct{}
	cancel context.CancelFunc
	mu     sync.Mutex
	next   int
	err    error // read after frames closes
}

func openACP(ctx context.Context, bin, dir string, sandbox bool, threadID ...string) (*acpConn, error) {
	ctx, cancel := context.WithCancel(ctx)
	args := []string{"acp"}
	if sandbox {
		args = append(args, "--sandbox")
	}
	cmd := cmdexec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if len(threadID) > 0 {
		cmd.Env = threadEnv(os.Environ(), threadID[0])
	}
	interruptible(cmd)
	cmd.WaitDelay = 3 * time.Second
	var stderr strings.Builder
	cmd.Stderr = &limited{w: &stderr, n: 8 << 10}
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
	c := &acpConn{in: in, frames: make(chan acpFrame, 32), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(c.done)
		defer close(c.frames)
		scan := bufio.NewScanner(out)
		scan.Buffer(make([]byte, 64<<10), 32<<20)
		for scan.Scan() {
			var frame acpFrame
			if json.Unmarshal(scan.Bytes(), &frame) != nil {
				continue // Some versions print login instructions on stdout.
			}
			select {
			case c.frames <- frame:
			case <-ctx.Done():
				cmd.Wait()
				c.err = ctx.Err()
				return
			}
		}
		if scan.Err() != nil {
			cancel()
		}
		c.err = cmd.Wait()
		if scan.Err() != nil {
			c.err = scan.Err()
		} else if msg := short(stderr.String(), 500); c.err != nil && msg != "" {
			c.err = fmt.Errorf("Antigravity: %s", msg)
		}
	}()
	return c, nil
}

func (c *acpConn) close() {
	c.in.Close()
	select {
	case <-c.done:
	case <-time.After(200 * time.Millisecond):
		c.cancel()
		<-c.done
	}
	c.cancel()
}

func (c *acpConn) send(frame acpParams) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame["jsonrpc"] = "2.0"
	return json.NewEncoder(c.in).Encode(frame)
}

func (c *acpConn) reply(id json.RawMessage, result any) error {
	return c.send(acpParams{"id": id, "result": result})
}

// call is driven by a single reader. Incoming requests can be answered while
// waiting for a reply, including requests that pause the current prompt.
func (c *acpConn) call(ctx context.Context, method string, params any, result any, handle func(acpFrame) error) error {
	c.next++
	id := c.next
	if err := c.send(acpParams{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame, ok := <-c.frames:
			if !ok {
				if c.err != nil {
					return c.err
				}
				return fmt.Errorf("Antigravity: %s: %w", method, io.ErrUnexpectedEOF)
			}
			if frame.Method != "" {
				if handle != nil {
					if err := handle(frame); err != nil {
						return err
					}
				} else if len(frame.ID) != 0 {
					if err := c.unsupported(frame); err != nil {
						return err
					}
				}
				continue
			}
			if string(frame.ID) != itoa(id) {
				continue
			}
			if frame.Error != nil {
				return frame.Error
			}
			if result != nil {
				return json.Unmarshal(frame.Result, result)
			}
			return nil
		}
	}
}

func (c *acpConn) unsupported(frame acpFrame) error {
	return c.send(acpParams{"id": frame.ID, "error": acpParams{"code": -32601, "message": "Unsupported client method"}})
}

type acpInitialize struct {
	ProtocolVersion   int
	AgentInfo         struct{ Name string }
	AgentCapabilities struct {
		LoadSession        bool
		MCPCapabilities    struct{ HTTP bool }
		PromptCapabilities struct{ Image, EmbeddedContext bool }
	}
	AuthMethods []struct{ ID string }
}

func (c *acpConn) initialize(ctx context.Context) (acpInitialize, error) {
	var info acpInitialize
	err := c.call(ctx, "initialize", acpParams{
		"protocolVersion":    1,
		"clientInfo":         acpParams{"name": "dory", "title": "Dory", "version": "1"},
		"clientCapabilities": acpParams{"elicitation": acpParams{"form": acpParams{}}},
	}, &info, nil)
	if err != nil {
		return info, err
	}
	switch info.AgentInfo.Name {
	case "antigravity-acp", "Antigravity ACP Server":
	default:
		return info, fmt.Errorf("servidor ACP inesperado: %q", info.AgentInfo.Name)
	}
	if info.ProtocolVersion != 1 || !info.AgentCapabilities.LoadSession {
		return info, fmt.Errorf("esta versión de Antigravity no admite reanudar sesiones con ACP v1")
	}
	return info, nil
}
