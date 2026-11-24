package runtime

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type fixtureQuotaProvider struct {
	Antigravity
	windows []Window
}

func (p fixtureQuotaProvider) Usage(context.Context) ([]Window, error) { return p.windows, nil }

func TestAntigravityQuotaStore(t *testing.T) {
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	weekly := reset.Add(72 * time.Hour)
	tsv := "Gemini Models\tFive Hour Limit Remaining\t25%\t" + reset.Format(time.RFC3339) + "\n" +
		"Gemini Models\tWeekly Limit Remaining\t75%\t" + weekly.Format(time.RFC3339) + "\n" +
		"Gemini Models\tWeekly Limit Remaining\t10%\t" + weekly.Format(time.RFC3339) + "\n" +
		"Remaining credits\t0\n" +
		"Claude and GPT models\tLímite semanal\tabc%\t" + weekly.Format(time.RFC3339) + "\n"
	got := parseAgyQuota([]byte(tsv))
	want := []Window{
		{Kind: "gemini-models-5h", Label: "Gemini Models · 5 horas", Minutes: 300, Utilization: 0.75, ResetsAt: reset},
		{Kind: "gemini-models-weekly", Label: "Gemini Models · Semanal", Minutes: 10080, Utilization: 0.25, ResetsAt: weekly},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quota windows = %+v, want %+v", got, want)
	}
	p := fixtureQuotaProvider{windows: append(got, Window{Kind: "expired", ResetsAt: reset.Add(-2 * time.Hour), Utilization: 0.5})}
	dir := t.TempDir()
	state := testNodeState(t)
	m := NewManager(nil, nil, "node", dir, state, p)
	m.RefreshUsage(t.Context())
	restarted := NewManager(nil, nil, "node", dir, state, p)
	if got := restarted.Usage()[antigravityID]; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored and visible windows = %+v, want %+v", got, want)
	}
}

func TestAntigravityQuotaFormats(t *testing.T) {
	weekly := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	translated := parseAgyQuota([]byte("Gemini\tLímite semanal restante\t40%\t" + weekly.Format(time.RFC3339) + "\n"))
	if len(translated) != 1 || translated[0].Minutes != 10080 || translated[0].Kind != "gemini-weekly" {
		t.Errorf("translated label = %+v", translated)
	}
	for _, raw := range []string{"", "unknown format\n", "{\"buckets\":[]}"} {
		if got := parseAgyQuota([]byte(raw)); len(got) != 0 {
			t.Errorf("parseAgyQuota(%q) = %+v", raw, got)
		}
	}
}
