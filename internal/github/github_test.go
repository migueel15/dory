package github

import "testing"

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/migueel15/dory.git": "migueel15/dory",
		"git@github.com:migueel15/dory.git":     "migueel15/dory",
		"https://github.com/a/b.c":              "a/b.c",
		"https://gitlab.com/a/b":                "",
	} {
		got, ok := ParseRemote(in)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v", in, got, ok)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		kind string
		a    Action
		ok   bool
	}{
		{"pulls", Action{Type: "approve"}, true},
		{"issues", Action{Type: "approve"}, false},
		{"issues", Action{Type: "comment", Body: " "}, false},
		{"issues", Action{Type: "close_issue"}, true},
		{"issues", Action{Type: "close_issue", Method: "NOT_PLANNED"}, true},
		{"issues", Action{Type: "close_issue", Method: "DUPLICATE"}, false},
		{"issues", Action{Type: "edit_issue", Title: " "}, false},
		{"issues", Action{Type: "edit_issue", Title: "Nuevo"}, true},
		{"pulls", Action{Type: "edit_issue", Title: "Nuevo"}, false},
		{"issues", Action{Type: "assign", UserIDs: []string{"U1"}}, true},
		{"issues", Action{Type: "assign"}, false},
		{"pulls", Action{Type: "merge"}, false},
		{"pulls", Action{Type: "merge", Method: "SQUASH"}, true},
		{"pulls", Action{Type: "delete"}, false},
	}
	for _, c := range cases {
		if err := validate(c.kind, c.a); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestUnique(t *testing.T) {
	got := unique(map[string]string{"bancosol-tfg/intranet": "local", "Bancosol-TFG/intranet": "tfg", "migueel15/dory": "dory"})
	want := map[string]string{"Bancosol-TFG/intranet": "tfg", "migueel15/dory": "dory"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v", got)
		}
	}
}

func TestMergedBy(t *testing.T) {
	const a, b = "https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"
	for name, c := range map[string]struct {
		in   []prState
		want string
	}{
		"none":         {nil, ""},
		"open":         {[]prState{{a, "OPEN"}}, ""},
		"merged":       {[]prState{{a, "MERGED"}}, a},
		"closed":       {[]prState{{a, "CLOSED"}}, ""},
		"merged+open":  {[]prState{{a, "MERGED"}, {b, "OPEN"}}, ""},
		"closed+merge": {[]prState{{a, "CLOSED"}, {b, "MERGED"}}, b},
	} {
		if got, ok := mergedBy(c.in); got != c.want || ok != (c.want != "") {
			t.Errorf("%s: got %q %v", name, got, ok)
		}
	}
}

func TestParseRef(t *testing.T) {
	if r, ok := parseRef("https://github.com/o/r/pull/7"); !ok || !r.pull || r.number != 7 || r.repo != "o/r" {
		t.Errorf("pull: %+v %v", r, ok)
	}
	if r, ok := parseRef("https://github.com/o/r/issues/3"); !ok || r.pull {
		t.Errorf("issue: %+v %v", r, ok)
	}
	if _, ok := parseRef("https://github.com/o/r/tree/main"); ok {
		t.Error("branch accepted")
	}
}
