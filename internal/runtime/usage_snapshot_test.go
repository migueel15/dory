package runtime

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

// snapshotProvider reports usage as a snapshot, like Antigravity's quota.
type snapshotProvider struct{ windows []Window }

func (snapshotProvider) Info() harness.Info {
	return harness.Info{ID: "snapshot", Driver: "snapshot", Name: "Snapshot",
		Capabilities: harness.Capabilities{UsageReport: true, UsageSnapshot: true}}
}

func (snapshotProvider) Detect(context.Context) Status { return Status{ID: "snapshot"} }

func (snapshotProvider) Start(context.Context, Turn) (Session, error) { return nil, ErrNoRequest }

func (p snapshotProvider) Usage(context.Context) ([]Window, error) { return p.windows, nil }

func TestSnapshotUsageIsStoredAndFiltered(t *testing.T) {
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	weekly := reset.Add(72 * time.Hour)
	want := []Window{
		{Kind: "gemini-models-5h", Label: "Gemini Models · 5 horas", Minutes: 300, Utilization: 0.75, ResetsAt: reset},
		{Kind: "gemini-models-weekly", Label: "Gemini Models · Semanal", Minutes: 10080, Utilization: 0.25, ResetsAt: weekly},
	}
	p := snapshotProvider{windows: append(append([]Window{}, want...), Window{Kind: "expired", ResetsAt: reset.Add(-2 * time.Hour), Utilization: 0.5})}
	dir := t.TempDir()
	state := testNodeState(t)
	m := NewManager(nil, nil, "node", dir, state, nil, p)
	m.RefreshUsage(t.Context())
	restarted := NewManager(nil, nil, "node", dir, state, nil, p)
	if got := restarted.Usage()["snapshot"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored and visible windows = %+v, want %+v", got, want)
	}
}
