package api

import "testing"

func TestAutoMessage(t *testing.T) {
	got := autoMessage("pr-question", map[string]string{"repo": "a/b", "pr": "7", "path": `x"y.go`, "empty": ""}, "Contexto\n", " ¿Por qué? ")
	want := "<dory-context kind=\"pr-question\" path=\"x&#34;y.go\" pr=\"7\" repo=\"a/b\">\nContexto\n</dory-context>\n\n¿Por qué?"
	if got != want {
		t.Fatalf("got %q", got)
	}
}
