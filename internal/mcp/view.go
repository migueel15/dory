package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/migueel15/dory/internal/service"
)

// Listings default to the summary view so agents do not pay for bodies they
// rarely read; view "full" returns whole records and get_item stays complete.
type viewListInput struct {
	listInput
	View string `json:"view,omitempty" jsonschema:"summary (default) or full"`
}

func checkView(view string) (bool, error) {
	switch view {
	case "", "summary":
		return true, nil
	case "full":
		return false, nil
	}
	return false, fmt.Errorf("%w: view must be summary or full", service.ErrInvalid)
}

type itemSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	ProjectID *string   `json:"project_id"`
	FeatureID *string   `json:"feature_id"`
	Priority  int32     `json:"priority"`
	Tags      []string  `json:"tags"`
	UpdatedAt time.Time `json:"updated_at"`
}
type featureView struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}
type projectSummary struct {
	ID     string   `json:"id"`
	Slug   string   `json:"slug"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Stack  []string `json:"stack"`
}
type memorySummary struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
	Content string   `json:"content" jsonschema:"First 200 characters"`
}

func summarizeItem(i service.Item) itemSummary {
	return itemSummary{i.ID, i.Title, i.Status, i.ProjectID, i.FeatureID, i.Priority, i.Tags, i.UpdatedAt}
}
func summarizeFeature(f service.Feature) featureView {
	return featureView{f.ID, f.ProjectID, f.Title, f.Status, f.UpdatedAt}
}
func summarizeProject(p service.Project) projectSummary {
	return projectSummary{p.ID, p.Slug, p.Name, p.Status, p.Stack}
}
func summarizeMemory(m service.Memory) memorySummary {
	content := m.Content
	if r := []rune(content); len(r) > 200 {
		content = string(r[:200])
	}
	return memorySummary{m.ID, m.Kind, m.Title, m.Tags, content}
}
func mapAll[T, S any](rows []T, fn func(T) S) []S {
	result := make([]S, 0, len(rows))
	for _, row := range rows {
		result = append(result, fn(row))
	}
	return result
}

// summarizeHits turns recall hits into memory summaries.
func summarizeHits(hits []service.SearchHit) ([]memorySummary, error) {
	result := make([]memorySummary, 0, len(hits))
	for _, hit := range hits {
		var memory service.Memory
		if err := json.Unmarshal(hit.Content, &memory); err != nil {
			return nil, err
		}
		result = append(result, summarizeMemory(memory))
	}
	return result, nil
}
