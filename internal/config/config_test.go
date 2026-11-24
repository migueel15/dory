package config

import "testing"

func TestValidateOrigin(t *testing.T) {
	valid := []string{"https://dory.dorlab.net", "http://localhost:8080"}
	for _, o := range valid {
		if err := validateOrigin(o); err != nil {
			t.Errorf("validateOrigin(%q) returned %v", o, err)
		}
	}
	invalid := []string{"dory.dorlab.net", "ftp://dory.dorlab.net", "https://dory.dorlab.net/", "https://dory.dorlab.net/app", "https://u:p@dory.dorlab.net"}
	for _, o := range invalid {
		if err := validateOrigin(o); err == nil {
			t.Errorf("validateOrigin(%q) accepted an invalid origin", o)
		}
	}
}

func TestValidateACP(t *testing.T) {
	if err := validateACP([]ACPConfig{{ID: "gemini", Command: "gemini", Args: []string{"--acp"}, Env: []string{"A=b"}}}); err != nil {
		t.Fatal(err)
	}
	for _, list := range [][]ACPConfig{
		{{ID: "Gemini", Command: "gemini"}},
		{{ID: "claude", Command: "x"}},
		{{ID: "gemini"}},
		{{ID: "a", Command: "x"}, {ID: "a", Command: "y"}},
		{{ID: "a", Command: "x", Env: []string{"novalue"}}},
	} {
		if validateACP(list) == nil {
			t.Errorf("accepted %+v", list)
		}
	}
}
