// Package goldentest compares the events of an adapter with a recorded
// sequence, so a parser change shows up as a readable diff.
package goldentest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/migueel15/dory/internal/harness"
)

// timestamps are the wall clock of the run, which a golden file cannot hold.
var timestamps = regexp.MustCompile(`"(CreatedAt|created_at)": "[^"]+"`)

var update = flag.Bool("update", false, "rewrite the golden files")

// Lines feeds each line of testdata/<name> to parse and returns what it emits.
func Lines(t *testing.T, name string, parse func(line []byte, emit func(harness.Event))) []harness.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var events []harness.Event
	for _, line := range bytes.Split(raw, []byte("\n")) {
		parse(line, func(e harness.Event) { events = append(events, e) })
	}
	return events
}

// Check compares events with testdata/<name>.golden.json; -update rewrites it.
func Check(t *testing.T, name string, events []harness.Event) {
	t.Helper()
	type typed struct {
		Type  string
		Event harness.Event
	}
	out := make([]typed, 0, len(events))
	for _, e := range events {
		out = append(out, typed{reflect.TypeOf(e).Name(), e})
	}
	got, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(timestamps.ReplaceAll(got, []byte(`"$1": "<time>"`)), '\n')
	path := filepath.Join("testdata", name+".golden.json")
	if *update {
		if err = os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from its golden file (run with -update to rewrite it):\n%s", name, firstDiff(string(want), string(got)))
	}
}

func firstDiff(want, got string) string {
	w, g := bytes.Split([]byte(want), []byte("\n")), bytes.Split([]byte(got), []byte("\n"))
	for i := 0; i < len(w) && i < len(g); i++ {
		if !bytes.Equal(w[i], g[i]) {
			return fmt.Sprintf("line %d\n want: %s\n  got: %s", i+1, w[i], g[i])
		}
	}
	return fmt.Sprintf("lengths differ: want %d lines, got %d", len(w), len(g))
}
