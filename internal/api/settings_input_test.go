package api

import "testing"

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
	dir := t.TempDir()
	st := NewSettingsStore(dir, Settings{})
	if st.Get().Input != nil {
		t.Fatal("input must be unset by default")
	}
	in := &InputSettings{Shortcuts: map[string]Chord{"dictation": {Code: "KeyE", Alt: true}}, Dictation: DictationSettings{Model: "m"}}
	if err := st.Set(Settings{Input: in}); err != nil {
		t.Fatal(err)
	}
	got := NewSettingsStore(dir, Settings{}).Get().Input
	if got == nil || got.Dictation.Model != "m" || got.Shortcuts["dictation"].Code != "KeyE" {
		t.Fatalf("not persisted: %+v", got)
	}
}
