package t3

import (
	"context"
	"errors"
)

// ErrInstanceUnavailable rejects launches on an instance that is not connected.
var ErrInstanceUnavailable = errors.New("t3: instance is not available")

// CatalogProvider is an enabled provider and its current models.
type CatalogProvider struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
	// Driver is the provider kind (codex, claudeAgent, ...), used for its icon.
	Driver string         `json:"driver"`
	Models []CatalogModel `json:"models"`
}

type CatalogModel struct {
	Slug   string        `json:"slug"`
	Name   string        `json:"name"`
	Effort *EffortOption `json:"effort,omitempty"`
}

// EffortOption is the model option that sets its reasoning effort.
type EffortOption struct {
	ID      string   `json:"id"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

// effortOptionIDs are the option ids T3 treats as reasoning effort, in order.
var effortOptionIDs = []string{"reasoningEffort", "effort", "reasoning", "reasoning_effort", "thinking_level", "thinking"}

type wireChoice struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault"`
}

type wireOptionDescriptor struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	Choices []wireChoice `json:"choices"`
	Options []wireChoice `json:"options"`
	Default any          `json:"default"`
}

func effortOf(descriptors []wireOptionDescriptor) *EffortOption {
	for _, id := range effortOptionIDs {
		for _, d := range descriptors {
			choices := d.Choices
			if len(choices) == 0 {
				choices = d.Options
			}
			if d.ID != id || d.Type != "select" || len(choices) == 0 {
				continue
			}
			out := &EffortOption{ID: id, Values: []string{}}
			for _, o := range choices {
				out.Values = append(out.Values, o.ID)
				if o.IsDefault {
					out.Default = o.ID
				}
			}
			if v, ok := d.Default.(string); ok && out.Default == "" {
				out.Default = v
			}
			return out
		}
	}
	return nil
}

type CatalogProject struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Root  string `json:"root"`
	// Repo is the web URL of the project's repository; empty when T3 knows none.
	Repo string `json:"repo"`
	// RepoName is the repository name, used to name a discovered Dory project.
	RepoName string `json:"-"`
}

// Catalog is what can be chosen when launching a thread on an instance.
type Catalog struct {
	Providers []CatalogProvider `json:"providers"`
	Projects  []CatalogProject  `json:"projects"`
}

// Catalog reads the enabled providers and the projects of the instance.
func (c Client) Catalog(ctx context.Context) (Catalog, error) {
	out := Catalog{Providers: []CatalogProvider{}, Projects: []CatalogProject{}}
	var snap struct {
		Projects []struct {
			ID                 string `json:"id"`
			Title              string `json:"title"`
			WorkspaceRoot      string `json:"workspaceRoot"`
			RepositoryIdentity *struct {
				WebURL string `json:"webUrl"`
				Name   string `json:"name"`
			} `json:"repositoryIdentity"`
		} `json:"projects"`
	}
	if err := c.get(ctx, "/api/orchestration/shell?omitIdleSubagents=true&omitOlderSettled=true", true, &snap); err != nil {
		return out, err
	}
	for _, p := range snap.Projects {
		cp := CatalogProject{ID: p.ID, Title: p.Title, Root: p.WorkspaceRoot}
		if r := p.RepositoryIdentity; r != nil {
			cp.Repo, cp.RepoName = r.WebURL, r.Name
		}
		out.Projects = append(out.Projects, cp)
	}
	var cfg struct {
		Providers []struct {
			InstanceID  string `json:"instanceId"`
			Driver      string `json:"driver"`
			DisplayName string `json:"displayName"`
			Enabled     bool   `json:"enabled"`
			Models      []struct {
				Slug         string `json:"slug"`
				Name         string `json:"name"`
				IsLegacy     bool   `json:"isLegacy"`
				Capabilities struct {
					OptionDescriptors []wireOptionDescriptor `json:"optionDescriptors"`
				} `json:"capabilities"`
			} `json:"models"`
		} `json:"providers"`
	}
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	ws, err := c.dial(ctx)
	if err != nil {
		return out, err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(-1)
	if err := call(ctx, ws, "1", "server.getConfig", map[string]any{}, &cfg); err != nil {
		return out, err
	}
	for _, p := range cfg.Providers {
		provider := CatalogProvider{InstanceID: p.InstanceID, Name: p.DisplayName, Driver: p.Driver, Models: []CatalogModel{}}
		for _, m := range p.Models {
			if !m.IsLegacy {
				provider.Models = append(provider.Models, CatalogModel{Slug: m.Slug, Name: m.Name, Effort: effortOf(m.Capabilities.OptionDescriptors)})
			}
		}
		if p.Enabled && len(provider.Models) > 0 {
			out.Providers = append(out.Providers, provider)
		}
	}
	return out, nil
}

// LaunchInput describes a thread started on behalf of a Dory item.
type LaunchInput struct {
	ProjectID string
	Title     string
	Provider  string
	Model     string
	Prompt    string
	// EffortOption and Effort set the reasoning effort; empty keeps the default.
	EffortOption string
	Effort       string
}

// Launch creates the thread and starts its turn. The returned id is set when
// the thread exists, even if the turn could not start.
func (c Client) Launch(ctx context.Context, in LaunchInput) (string, error) {
	id, err := c.CreateThread(ctx, CreateThreadInput{ProjectID: in.ProjectID, Title: in.Title, InstanceID: in.Provider, Model: in.Model, EffortOption: in.EffortOption, Effort: in.Effort})
	if err != nil {
		return "", err
	}
	return id, c.SendMessage(ctx, id, in.Prompt)
}
