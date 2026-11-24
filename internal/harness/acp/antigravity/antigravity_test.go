package antigravity

import (
	"database/sql"
	"encoding/binary"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

func TestAntigravityPaths(t *testing.T) {
	home, app := t.TempDir(), t.TempDir()
	for _, tt := range []struct{ os, install, root string }{
		{"linux", filepath.Join(home, ".local", "bin"), filepath.Join(app, "antigravity")},
		{"darwin", filepath.Join(home, ".local", "bin"), filepath.Join(home, "Library", "Application Support", "antigravity")},
		{"windows", filepath.Join(app, "agy", "bin"), filepath.Join(app, "antigravity")},
	} {
		if got := installDir(tt.os, home, app); got != tt.install {
			t.Errorf("%s: %q", tt.os, got)
		}
		if !strings.Contains(strings.Join(dataRoots(tt.os, home, app, app, app), "\n"), tt.root) {
			t.Errorf("missing %s root", tt.os)
		}
	}
}

func TestAntigravityUsageStore(t *testing.T) {
	v := func(tag, value uint64) []byte { return binary.AppendUvarint(binary.AppendUvarint(nil, tag<<3), value) }
	b := func(tag uint64, data []byte) []byte {
		return append(binary.AppendUvarint(binary.AppendUvarint(nil, tag<<3|2), uint64(len(data))), data...)
	}
	payload := append(v(1, 42), v(2, 100)...)
	payload = append(payload, v(3, 7)...)
	payload = append(payload, v(5, 13)...)
	payload = append(payload, v(9, 5)...)
	payload = append(payload, v(10, 9)...)
	payload = append(payload, b(11, []byte("key"))...)
	meta := append(v(3, 42), b(19, []byte("Gemini Test"))...)
	meta = append(meta, b(4, payload)...)
	path := filepath.Join(t.TempDir(), "session.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE gen_metadata(idx INTEGER, data BLOB)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO gen_metadata VALUES (?, ?)", 3, b(1, meta)); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	want := []harness.UsageEvent{{Time: at, Provider: ID, Session: "session", Model: "gemini-test", Key: "antigravity:11:key", Reasoning: 9, Tokens: harness.UsageTokens{Input: 100, Output: 14, CacheRead: 13}}}
	if actual := readUsage(path, at); !reflect.DeepEqual(actual, want) {
		t.Fatalf("got %+v, want %+v", actual, want)
	}
	for _, raw := range [][]byte{{0x80}, {0x12, 0xff, 0x07, 0x1a}, {0xff, 0xff, 0xff}} {
		if parseAgyProto(raw) != nil {
			t.Fatalf("accepted malformed protobuf %x", raw)
		}
	}
}

func TestQuotaParse(t *testing.T) {
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	weekly := reset.Add(72 * time.Hour)
	tsv := "Gemini Models\tFive Hour Limit Remaining\t25%\t" + reset.Format(time.RFC3339) + "\n" +
		"Gemini Models\tWeekly Limit Remaining\t75%\t" + weekly.Format(time.RFC3339) + "\n" +
		"Gemini Models\tWeekly Limit Remaining\t10%\t" + weekly.Format(time.RFC3339) + "\n" +
		"Remaining credits\t0\n" +
		"Claude and GPT models\tLímite semanal\tabc%\t" + weekly.Format(time.RFC3339) + "\n"
	got := parseAgyQuota([]byte(tsv))
	want := []harness.Window{
		{Kind: "gemini-models-5h", Label: "Gemini Models · 5 horas", Minutes: 300, Utilization: 0.75, ResetsAt: reset},
		{Kind: "gemini-models-weekly", Label: "Gemini Models · Semanal", Minutes: 10080, Utilization: 0.25, ResetsAt: weekly},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quota windows = %+v, want %+v", got, want)
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

func TestProfile(t *testing.T) {
	p := Profile("")
	if p.ID != "antigravity" || p.Order != 3 || !p.Capabilities.UsageSnapshot || !p.Capabilities.Resume || p.Usage == nil || p.History == nil {
		t.Fatalf("profile %+v", p)
	}
	if len(p.Login) != 2 || p.Login[1] != "--login" || p.Args[0] != "acp" || p.SandboxArgs[0] != "--sandbox" {
		t.Fatalf("commands %+v", p)
	}
	if got := Profile("/x/agy").Bins; len(got) != 1 || got[0] != "/x/agy" {
		t.Fatalf("explicit binary: %v", got)
	}
	meta, ok := systemPrompt([]byte(`{"antigravity":{"systemPrompt":{"append":true}}}`), "rules")
	if !ok || !reflect.DeepEqual(meta, map[string]any{"antigravity": map[string]any{"systemPrompt": map[string]any{"append": "rules"}}}) {
		t.Fatalf("system prompt meta %v %v", meta, ok)
	}
	if _, ok := systemPrompt([]byte(`{"antigravity":{}}`), "rules"); ok {
		t.Fatal("the prompt must go in the first message when agy cannot append it")
	}
}
