package t3

import (
	"encoding/json"
	"testing"
)

func TestEffortOf(t *testing.T) {
	var ds []wireOptionDescriptor
	raw := `[{"id":"serviceTier","type":"select","choices":[{"id":"default","isDefault":true}]},{"id":"effort","type":"select","choices":[{"id":"low"},{"id":"medium","isDefault":true},{"id":"high"}]}]`
	if err := json.Unmarshal([]byte(raw), &ds); err != nil {
		t.Fatal(err)
	}
	got := effortOf(ds)
	if got == nil || got.ID != "effort" || got.Default != "medium" || len(got.Values) != 3 {
		t.Fatalf("unexpected effort %+v", got)
	}
	if effortOf(nil) != nil {
		t.Fatal("expected no effort without descriptors")
	}
}
