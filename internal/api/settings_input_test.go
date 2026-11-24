package api

import (
	"context"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
)

func TestInputSettingsValidate(t *testing.T) {
	ok := InputSettings{Shortcuts: map[string]Chord{"palette": {Code: "KeyK", Ctrl: true}}}
	if err := ok.validate(); err != nil {
		t.Fatalf("valid chord rejected: %v", err)
	}
	for _, c := range []Chord{{Code: "Enter", Ctrl: true}, {Code: "KeyK"}} {
		bad := InputSettings{Shortcuts: map[string]Chord{"palette": c}}
		if bad.validate() == nil {
			t.Fatalf("chord %+v accepted", c)
		}
	}
}

func TestInputSettingsPersist(t *testing.T) {
	ctx := context.Background()
	db, err := localdb.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := nodestate.New(db)
	st := NewSettingsStore(store, Settings{})
	if st.Get().Input != nil {
		t.Fatal("input must be unset by default")
	}
	in := &InputSettings{Shortcuts: map[string]Chord{"dictation": {Code: "KeyE", Alt: true}}, Dictation: DictationSettings{Model: "m"}}
	if err := st.Set(Settings{Input: in}); err != nil {
		t.Fatal(err)
	}
	if got := NewSettingsStore(store, Settings{}).Get().Input; got == nil || got.Dictation.Model != "m" {
		t.Fatalf("not persisted: %+v", got)
	}
	if err := store.Put(ctx, nodestate.KeySettings, map[string]any{
		"archive_threads_on_finish": false,
		"input":                     in,
	}); err != nil {
		t.Fatal(err)
	}
	reloaded := NewSettingsStore(store, Settings{
		ArchiveThreadsOnFinish: true, RemoveWorktreeOnArchive: true,
	}).Get()
	if reloaded.ArchiveThreadsOnFinish || !reloaded.RemoveWorktreeOnArchive {
		t.Fatalf("settings did not overlay defaults: %+v", reloaded)
	}
	got := reloaded.Input
	if got == nil || got.Dictation.Model != "m" || got.Shortcuts["dictation"].Code != "KeyE" {
		t.Fatalf("not persisted: %+v", got)
	}
}
