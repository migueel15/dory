package api

import "testing"

func TestCaptureItemFromDictation(t *testing.T) {
	p := "project"
	item := captureItem("Primera línea dictada\nresto", &p)
	if item.Title != "Primera línea dictada" || item.RawText != "Primera línea dictada\nresto" || item.Status != "inbox" || item.TitleSource != "pending" || *item.ProjectID != "project" || len(item.ID) != 36 {
		t.Fatalf("unexpected item %+v", item)
	}
}
