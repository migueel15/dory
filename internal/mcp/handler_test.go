package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestNormalizeRemote(t *testing.T) {
	want := "github.com/migueel15/dory"
	for _, url := range []string{
		"https://github.com/migueel15/dory.git",
		"https://GitHub.com/migueel15/dory/",
		"git@github.com:migueel15/dory.git",
		"ssh://git@github.com:22/migueel15/dory.git",
		"https://user@github.com/migueel15/dory",
	} {
		if got := normalizeRemote(url); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestHasTags(t *testing.T) {
	if !hasTags([]string{"a", "b"}, []string{"b"}) || hasTags([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("hasTags must require every wanted tag")
	}
}

func TestSummaryView(t *testing.T) {
	if s, err := checkView(""); !s || err != nil {
		t.Fatal("empty view must default to summary")
	}
	if s, err := checkView("full"); s || err != nil {
		t.Fatal("full view must not summarize")
	}
	if _, err := checkView("other"); err == nil {
		t.Fatal("unknown view must fail")
	}
	m := summarizeMemory(service.Memory{Content: strings.Repeat("ñ", 250)})
	if len([]rune(m.Content)) != 200 {
		t.Fatalf("memory summary must keep 200 characters, got %d", len([]rune(m.Content)))
	}
}

func TestWriteReturn(t *testing.T) {
	if (writeReturn{Return: "other"}).check() == nil {
		t.Fatal("unknown return must fail")
	}
	short, _ := writeReturn{}.reply(service.Item{Record: service.Record{ID: "x"}, BodyMd: strings.Repeat("a", 1000)}, nil)
	if raw, _ := json.Marshal(short); len(raw) >= 300 {
		t.Fatalf("short reply is %d bytes", len(raw))
	}
}

func TestItemFilter(t *testing.T) {
	in := itemListInput{viewListInput: viewListInput{listInput: listInput{Fields: map[string]any{"project_id": "p", "status": []any{"ready", "done"}, "tags": []any{"x"}}}}, Open: true}
	f, err := in.itemFilter()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.AnyOf["status"], []string{"ready"}) || f.Fields["project_id"] != "p" || f.Fields["tags"] == nil {
		t.Fatalf("unexpected filter %#v", f)
	}
	f, _ = itemListInput{Open: true}.itemFilter()
	if len(f.AnyOf["status"]) != 6 {
		t.Fatalf("open must list the six open statuses, got %v", f.AnyOf["status"])
	}
	if _, err = (itemListInput{viewListInput: viewListInput{listInput: listInput{Fields: map[string]any{"title": []any{"a"}}}}}).itemFilter(); err == nil {
		t.Fatal("only status and id accept several values")
	}
}

func TestUpdateItemsLimits(t *testing.T) {
	if _, err := updateItems(context.Background(), nil, updatesInput{}); err == nil {
		t.Fatal("an empty batch must fail")
	}
	if _, err := updateItems(context.Background(), nil, updatesInput{IDs: make([]string, 1001)}); err == nil {
		t.Fatal("more than 1000 updates must fail")
	}
}
