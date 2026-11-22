package runtime

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type acpFixtureInput struct{ bytes.Buffer }

func (*acpFixtureInput) Close() error { return nil }

func feedACP(t *testing.T, c *acpConn, name string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "antigravity", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		var frame acpFrame
		if err := json.Unmarshal(scan.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		c.frames <- frame
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestAntigravityResumeTranscript(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for _, full := range []bool{false, true} {
		input := &acpFixtureInput{}
		c := &acpConn{in: input, frames: make(chan acpFrame, 32)}
		feedACP(t, c, "initialize")
		for _, line := range []string{
			`{"id":2,"method":"session/update","params":{"sessionId":"saved","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"old message"}}}}`,
			`{"id":2,"result":{"configOptions":[{"id":"model","currentValue":"before"},{"id":"think","category":"thought_level","currentValue":"low"},{"id":"mode","currentValue":"plan"}]}}`,
			`{"id":3,"result":{}}`,
			`{"id":4,"result":{}}`,
			`{"id":5,"result":{}}`,
		} {
			var frame acpFrame
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatal(err)
			}
			c.frames <- frame
		}
		feedACP(t, c, "turn")
		s := newAntigravitySession(ctx, c, Turn{RunID: "turn", Dir: t.TempDir(), Session: "saved", Model: "after", Effort: "high", Text: "Hi", FullAccess: full, MCPURL: "http://127.0.0.1:8080/mcp?thread=one", MCPToken: "test"})
		done := make(chan error, 1)
		go func() {
			done <- s.run()
			close(s.events)
		}()
		var last Message
		var sawSession, sawEdit, sawPlan bool
		for event := range s.Events() {
			switch {
			case event.Session == "saved":
				sawSession = true
			case event.Request != nil:
				if err := s.Answer(event.Request.ID, [][]string{{"dev"}}); err != nil {
					t.Fatal(err)
				}
			case event.Message != nil:
				last = *event.Message
				if last.Text == "old message" {
					t.Fatal("resumed history was emitted as a live turn")
				}
				sawEdit = sawEdit || (last.Tool != nil && last.Tool.Kind == "edit" && last.Status == "completed")
				sawPlan = sawPlan || (last.Role == "plan" && strings.Contains(last.Text, "- [x] Inspect"))
			}
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !sawSession || !sawEdit || !sawPlan || last.Text != "Hello, Dory" || last.Streaming {
			t.Fatalf("unexpected events: session %v, edit %v, plan %v, last %+v", sawSession, sawEdit, sawPlan, last)
		}
		var outbound []acpFrame
		scan := bufio.NewScanner(input)
		for scan.Scan() {
			var frame acpFrame
			if err := json.Unmarshal(scan.Bytes(), &frame); err != nil {
				t.Fatal(err)
			}
			outbound = append(outbound, frame)
		}
		wantExecute := "no"
		if full {
			wantExecute = "yes"
		}
		if len(outbound) != 9 || outbound[1].Method != "session/load" || !bytes.Contains(outbound[1].Params, []byte("mcp-stdio")) ||
			!bytes.Contains(outbound[6].Result, []byte(`"optionId":"yes"`)) ||
			!bytes.Contains(outbound[7].Result, []byte(`"optionId":"`+wantExecute+`"`)) ||
			!bytes.Contains(outbound[8].Result, []byte(`"optionId":"b"`)) {
			t.Fatalf("unexpected RPC frames: %+v", outbound)
		}
	}
}

func TestAntigravityWorktreePermissions(t *testing.T) {
	dir := t.TempDir()
	out := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, kind string
		paths      []string
		allowed    bool
	}{
		{"nested new file", "edit", []string{filepath.Join(dir, "src", "deep", "new.go")}, true},
		{"relative", "edit", []string{filepath.Join("src", "one.go")}, true},
		{"outside", "edit", []string{filepath.Join(out, "file")}, false},
		{"mixed", "edit", []string{"ok", filepath.Join(out, "file")}, false},
		{"missing target", "edit", nil, false},
		{"parent traversal", "edit", []string{filepath.Join("..", "outside")}, false},
		{"execution", "execute", []string{"ok"}, false},
		{"deletion", "delete", []string{"ok"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool := acpTool{Kind: tt.kind}
			for _, path := range tt.paths {
				tool.Locations = append(tool.Locations, acpLocation{Path: path})
			}
			if got := antigravityWorktreeEdit(dir, tool); got != tt.allowed {
				t.Fatalf("allowed=%v", got)
			}
		})
	}
	t.Run("symlink escape", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		if err := os.Symlink(out, link); err != nil {
			t.Skip(err)
		}
		if antigravityWorktreeEdit(dir, acpTool{Kind: "edit", Locations: []acpLocation{{Path: filepath.Join(link, "new")}}}) {
			t.Fatal("approved an edit outside the worktree through a symlink")
		}
	})
}

func TestAntigravityPaths(t *testing.T) {
	home, app := t.TempDir(), t.TempDir()
	for _, tt := range []struct{ os, install, root string }{
		{"linux", filepath.Join(home, ".local", "bin"), filepath.Join(app, "antigravity")},
		{"darwin", filepath.Join(home, ".local", "bin"), filepath.Join(home, "Library", "Application Support", "antigravity")},
		{"windows", filepath.Join(app, "agy", "bin"), filepath.Join(app, "antigravity")},
	} {
		if got := antigravityInstallDir(tt.os, home, app); got != tt.install {
			t.Errorf("%s: %q", tt.os, got)
		}
		if !strings.Contains(strings.Join(antigravityDataRoots(tt.os, home, app, app, app), "\n"), tt.root) {
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
	want := []usageEvent{{Time: at, Provider: antigravityID, Session: "session", Model: "gemini-test", Key: "antigravity:11:key", Reasoning: 9, Tokens: UsageTokens{Input: 100, Output: 14, CacheRead: 13}}}
	if actual := readAntigravityUsage(path, at); !reflect.DeepEqual(actual, want) {
		t.Fatalf("got %+v, want %+v", actual, want)
	}
	for _, raw := range [][]byte{{0x80}, {0x12, 0xff, 0x07, 0x1a}, {0xff, 0xff, 0xff}} {
		if parseAgyProto(raw) != nil {
			t.Fatalf("accepted malformed protobuf %x", raw)
		}
	}
}
