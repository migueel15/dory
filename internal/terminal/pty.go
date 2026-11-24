// Package terminal runs the shells of the side panel: real terminals on the
// node that runs a thread. Everything but the files named pty_<os>.go is
// common to every system and only knows the PTY interface.
package terminal

import (
	"io"
	"os"

	gopty "github.com/aymanbagabas/go-pty"
)

// PTY is a shell attached to a pseudo-terminal.
type PTY interface {
	io.ReadWriter
	Resize(cols, rows int) error
	// Wait returns when the shell exits.
	Wait() error
	// Close kills the shell and its children.
	Close() error
}

// proc is the PTY of go-pty with the system's way to kill the process tree.
type proc struct {
	pty  gopty.Pty
	cmd  *gopty.Cmd
	kill func(*os.Process)
}

func (p *proc) Read(b []byte) (int, error)  { return p.pty.Read(b) }
func (p *proc) Write(b []byte) (int, error) { return p.pty.Write(b) }
func (p *proc) Resize(cols, rows int) error { return p.pty.Resize(cols, rows) }
func (p *proc) Wait() error                 { return p.cmd.Wait() }
func (p *proc) Close() error {
	if p.cmd.Process != nil {
		p.kill(p.cmd.Process)
	}
	return p.pty.Close()
}

// launch starts shell attached to a new PTY of the given size.
func launch(shell string, args []string, cwd string, env []string, cols, rows int, kill func(*os.Process)) (PTY, error) {
	p, err := gopty.New()
	if err != nil {
		return nil, err
	}
	if err := p.Resize(cols, rows); err != nil {
		p.Close()
		return nil, err
	}
	cmd := p.Command(shell, args...)
	cmd.Dir = cwd
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		p.Close()
		return nil, err
	}
	return &proc{pty: p, cmd: cmd, kill: kill}, nil
}

// Enter is the key that sends a line to the shell of this system.
func Enter() string { return enter }
