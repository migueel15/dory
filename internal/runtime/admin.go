package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/procpool"
)

type Job = procpool.Job

var ErrJobInputClosed = procpool.ErrJobInputClosed

func (m *Manager) StartJob(provider, kind string) (*Job, error) {
	p, ok := m.registry.Get(provider)
	if !ok || p.Admin == nil {
		return nil, ErrProvider
	}
	var args []string
	switch kind {
	case "login":
		args = p.Admin.LoginCommand()
	case "update":
		args = p.Admin.UpdateCommand()
	case "install":
		args = p.Admin.InstallCommand()
	default:
		return nil, errors.New("unknown job " + kind)
	}
	return m.pool.StartJob(provider, kind, args, accountEnvironment(p.Accounts),
		kind == "login" && p.Info().Capabilities.LoginCode, m.invalidate)
}

func (m *Manager) SubmitLoginCode(id, code string) error { return m.pool.SubmitLoginCode(id, code) }
func (m *Manager) JobState(id string) (Job, bool)        { return m.pool.JobState(id) }

func accountEnvironment(a harness.AccountSupport) []string {
	if a == nil {
		return nil
	}
	return a.Environ()
}

// UpdateAll runs the update command of every installed harness, one after
// another, and returns the error of each one that failed.
func (m *Manager) UpdateAll(ctx context.Context) map[string]string {
	failed := map[string]string{}
	for _, st := range m.Providers(ctx) {
		p, ok := m.registry.Get(st.ID)
		if !ok || p.Admin == nil || !st.Installed {
			continue
		}
		args := p.Admin.UpdateCommand()
		if len(args) == 0 {
			continue
		}
		run, cancel := context.WithTimeout(ctx, 10*time.Minute)
		upd := cmdexec.CommandContext(run, args[0], args[1:]...)
		upd.Env = accountEnvironment(p.Accounts)
		out, err := upd.CombinedOutput()
		cancel()
		if err != nil {
			failed[st.ID] = strings.TrimSpace(string(out))
		}
	}
	return failed
}
