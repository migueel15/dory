// Package harness defines the contract between Dory and its agent adapters.
package harness

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/migueel15/dory/internal/harness/procpool"
)

// ErrNoUsage is what Usage returns when a harness does not report its usage.
var ErrNoUsage = errors.New("runtime: provider does not report usage")

type Driver interface {
	Info() Info
	Detect(context.Context) Status
	Start(context.Context, Turn) (Session, error)
}

type Session interface {
	Events() <-chan Event
	Respond(requestID string, answers [][]string) error
}

type Info struct {
	ID, Driver, Name string
	Order            int
	Capabilities     Capabilities
}

type Capabilities struct {
	Resume, MidTurnRequests, UsageReport, UsageSnapshot, SystemPrompt bool
	DeferredTools, LoginCode, Effort, OfflineHistory                  bool
}

func (c Capabilities) Names() []string {
	names := []string{"interrupt"}
	for _, feature := range []struct {
		enabled bool
		name    string
	}{
		{c.Resume, "resume"}, {c.MidTurnRequests, "mid_turn_requests"},
		{c.UsageReport, "usage"}, {c.Effort, "effort"}, {c.UsageSnapshot, "usage_snapshot"},
		{c.SystemPrompt, "system_prompt"}, {c.LoginCode, "login_code"},
	} {
		if feature.enabled {
			names = append(names, feature.name)
		}
	}
	return names
}

type Usage interface {
	Usage(context.Context) ([]Window, error)
}

type Admin interface {
	Session(context.Context) (bool, string)
	LoginCommand() []string
	UpdateCommand() []string
	InstallCommand() []string
}

type ExternalSessions interface {
	List(context.Context, Paths) []External
	Read(context.Context, External) ([]Message, error)
	SessionFile(Paths, string) string
}

type Commands interface {
	Commands(Paths, string) (commands, skills []Entry)
	SkillDir(Paths) SkillLocation
}

type AccountSupport interface {
	WithAccount(Account) Driver
	ConfigEnv() string
	DefaultHome(home string) string
	AccountHome() string
	Environ() []string
}

type HistorySource interface {
	UsageHistory(Paths, time.Time) []UsageEvent
}

type LatestVersion interface {
	LatestVersion(context.Context) string
}

type controlProcess interface{ IsControlProcess(string) bool }

type processNames interface{ ProcessNames() []string }
type pooled interface{ WithPool(*procpool.Pool) Driver }

type EntryPoint struct {
	Driver
	Usage          Usage
	Admin          Admin
	External       ExternalSessions
	Commands       Commands
	Accounts       AccountSupport
	Versions       LatestVersion
	History        HistorySource
	ProcessNames   []string
	ControlProcess func(string) bool
}

func entry(d Driver) EntryPoint {
	e := EntryPoint{Driver: d}
	e.Usage, _ = d.(Usage)
	e.Admin, _ = d.(Admin)
	e.External, _ = d.(ExternalSessions)
	e.Commands, _ = d.(Commands)
	e.Accounts, _ = d.(AccountSupport)
	e.Versions, _ = d.(LatestVersion)
	e.History, _ = d.(HistorySource)
	if p, ok := d.(processNames); ok {
		e.ProcessNames = p.ProcessNames()
	}
	if p, ok := d.(controlProcess); ok {
		e.ControlProcess = p.IsControlProcess
	}
	return e
}

type Paths struct {
	Home   string
	Config func(driver string) string
}

func (p Paths) Dir(driver string) string {
	if p.Config != nil {
		if dir := p.Config(driver); dir != "" {
			return dir
		}
	}
	return DefaultConfigDir(p.Home, driver)
}

func DefaultConfigDir(home, driver string) string {
	return filepath.Join(home, "."+driver)
}

// SkillLocation describes where the catalog installs a driver’s skills.
type SkillLocation struct{ Dir, ManagedSubdir string }
