package acp

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

// The methods below are the optional interfaces of the harness package. A
// profile without the matching hook answers with the neutral value.

func (r rich) Session(ctx context.Context) (bool, string) {
	if !r.p.Sessions {
		return false, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if _, err := r.list(ctx, 1); err != nil {
		return false, ""
	}
	return true, r.p.Account
}

func (r rich) LoginCommand() []string {
	if r.p.Login == nil {
		return nil
	}
	return append([]string{r.bin()}, r.p.Login...)
}

func (r rich) UpdateCommand() []string {
	if r.p.Update == nil {
		return nil
	}
	return append([]string{r.bin()}, r.p.Update...)
}

func (r rich) InstallCommand() []string { return r.p.Install }

func (r rich) Usage(ctx context.Context) ([]harness.Window, error) {
	if r.p.Usage == nil {
		return nil, harness.ErrNoUsage
	}
	return r.p.Usage(ctx, r.bin())
}

func (r rich) UsageHistory(p harness.Paths, from time.Time) []harness.UsageEvent {
	if r.p.History == nil {
		return nil
	}
	return r.p.History(p, from)
}

func (r rich) LatestVersion(ctx context.Context) string {
	if r.p.LatestVersion == nil {
		return ""
	}
	return r.p.LatestVersion(ctx)
}

func (r rich) Commands(p harness.Paths, dir string) (commands, skills []harness.Entry) {
	if r.p.Commands == nil {
		return nil, nil
	}
	return r.p.Commands(p, dir)
}

func (r rich) SkillDir(p harness.Paths) harness.SkillLocation {
	if r.p.SkillDir == nil {
		return harness.SkillLocation{}
	}
	return r.p.SkillDir(p)
}
