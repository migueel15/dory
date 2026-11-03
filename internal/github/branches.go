package github

import (
	"context"
	"encoding/json"
	"fmt"
)

// Branches lists the branch names of a registered repository (first 100).
func (c *Client) Branches(ctx context.Context, repo string) ([]string, error) {
	if _, err := c.checkRepo(ctx, repo); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, "/repos/"+repo+"/branches?per_page=100", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var wire []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("github: decode branches: %w", err)
	}
	out := make([]string, len(wire))
	for i, b := range wire {
		out[i] = b.Name
	}
	return out, nil
}
