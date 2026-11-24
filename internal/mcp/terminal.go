package mcp

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/terminal"
)

type terminalRef struct {
	Terminal string `json:"terminal" jsonschema:"Id or name of the terminal; prefer the id, which never changes"`
}

// addTerminalTools registers the tools of the thread's terminals: real
// shells on this node that the user sees as tabs of the side panel.
func addTerminalTools(srv *sdkServer, s *Deps) {
	thread := func(ctx context.Context) (string, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return "", apperr.New(apperr.Invalid, "terminals only work inside a Dory thread")
		}
		if s.Terminals == nil {
			return "", apperr.New(apperr.Unavailable, "this node has no terminals")
		}
		return id, nil
	}
	// pick finds the terminal of the call.
	pick := func(ctx context.Context, ref string) (string, *terminal.Session, error) {
		id, err := thread(ctx)
		if err != nil {
			return "", nil, err
		}
		sess, err := s.Terminals.Get(id, ref)
		return id, sess, err
	}
	who := func(sess *terminal.Session) map[string]any {
		i := sess.Info()
		return map[string]any{"id": i.ID, "name": i.Name}
	}
	add(srv, "terminal_open", "Open a terminal (a real shell on this node, in this thread's workspace) and show it in the side panel. Returns its id, name and cwd.", func(ctx context.Context, in struct {
		Name string `json:"name,omitempty" jsonschema:"Name of the terminal; unique in the thread; default Terminal N"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		info, err := s.Terminals.Open(ctx, id, in.Name, 120, 30)
		if err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("terminal", id, map[string]string{"action": "open", "terminal": info.ID})
		}
		return info, nil
	})
	add(srv, "terminal_list", "List this thread's terminals with their id, name, cwd and whether the shell is running.", func(ctx context.Context, _ struct{}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Terminals.List(id), nil
	})
	add(srv, "terminal_rename", "Rename a terminal; names are unique in the thread.", func(ctx context.Context, in struct {
		terminalRef
		Name string `json:"name"`
	}) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		return s.Terminals.Rename(id, in.Terminal, in.Name)
	})
	add(srv, "terminal_read", "Read the last lines of a terminal's output as plain text, without escape codes.", func(ctx context.Context, in struct {
		terminalRef
		Lines int `json:"lines,omitempty" jsonschema:"How many lines; default 50"`
	}) (any, error) {
		_, sess, err := pick(ctx, in.Terminal)
		if err != nil {
			return nil, err
		}
		n := in.Lines
		if n <= 0 {
			n = 50
		}
		out := who(sess)
		out["running"] = sess.Running()
		out["output"] = sess.Text(n)
		return out, nil
	})
	add(srv, "terminal_write", "Type text into a terminal as the keyboard would, without waiting for output. Keys are control characters: \\u0003 is Ctrl+C, \\u0004 Ctrl+D, \\u001a Ctrl+Z. Use terminal_run for a command whose output you want.", func(ctx context.Context, in struct {
		terminalRef
		Text  string `json:"text"`
		Enter bool   `json:"enter,omitempty" jsonschema:"Press Enter after the text"`
	}) (any, error) {
		_, sess, err := pick(ctx, in.Terminal)
		if err != nil {
			return nil, err
		}
		text := in.Text
		if in.Enter {
			text += terminal.Enter()
		}
		if err := sess.Write([]byte(text)); err != nil {
			return nil, err
		}
		return who(sess), nil
	})
	add(srv, "terminal_run", "Type a command in a terminal and wait until its output stays quiet or the timeout passes; returns what it printed. For long jobs use a bigger timeout, then terminal_read.", func(ctx context.Context, in struct {
		terminalRef
		Command string `json:"command"`
		Timeout int    `json:"timeout,omitempty" jsonschema:"Seconds to wait at most; default 30"`
	}) (any, error) {
		_, sess, err := pick(ctx, in.Terminal)
		if err != nil {
			return nil, err
		}
		timeout := time.Duration(in.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		out, err := sess.Run(in.Command, timeout)
		if err != nil {
			return nil, err
		}
		res := who(sess)
		res["output"] = out
		res["running"] = sess.Running()
		return res, nil
	})
	add(srv, "terminal_close", "Close a terminal and kill its shell.", func(ctx context.Context, in terminalRef) (any, error) {
		id, err := thread(ctx)
		if err != nil {
			return nil, err
		}
		info, err := s.Terminals.Close(id, in.Terminal)
		if err != nil {
			return nil, err
		}
		if s.Notify != nil {
			s.Notify("terminal", id, map[string]string{"action": "close", "terminal": info.ID})
		}
		return map[string]any{"id": info.ID, "name": info.Name, "closed": true}, nil
	})
}
