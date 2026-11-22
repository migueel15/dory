package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// readModelCatalog keeps stdin open until the harness replies to the metadata
// request. Discovery does not submit a prompt or start an agent turn.
func readModelCatalog(ctx context.Context, bin string, env []string, args []string, requests string, read func([]byte) []Model) []Model {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := cmdexec.CommandContext(ctx, bin, args...)
	if env != nil {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	defer cmd.Wait()
	defer cancel()
	if _, err := stdin.Write([]byte(requests)); err != nil {
		return nil
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		if models := read(scanner.Bytes()); len(models) > 0 {
			return models
		}
	}
	return nil
}

func claudeModels(ctx context.Context, bin string, env []string) []Model {
	return readModelCatalog(ctx, bin, env, []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"},
		"{\"type\":\"control_request\",\"request_id\":\"models\",\"request\":{\"subtype\":\"initialize\"}}\n",
		func(line []byte) []Model {
			var reply struct {
				Response struct {
					RequestID string `json:"request_id"`
					Response  struct {
						Models []struct {
							Value                 string
							DisplayName           string
							ResolvedModel         string
							SupportedEffortLevels []string
						}
					}
				}
			}
			if json.Unmarshal(line, &reply) != nil || reply.Response.RequestID != "models" {
				return nil
			}
			entries := reply.Response.Response.Models
			models := make([]Model, 0, len(entries))
			for _, entry := range entries {
				if entry.Value != "" && entry.Value != "default" && strings.TrimSpace(entry.DisplayName) != "" {
					models = append(models, Model{Slug: entry.Value, Name: entry.DisplayName, Efforts: entry.SupportedEffortLevels, ResolvedModel: entry.ResolvedModel})
				}
			}
			return models
		})
}

func codexModels(ctx context.Context, bin string, env []string) []Model {
	const requests = `{"id":0,"method":"initialize","params":{"clientInfo":{"name":"dory","version":"0.1.0"}}}
{"method":"initialized"}
{"id":1,"method":"model/list","params":{"limit":1000}}
`
	return readModelCatalog(ctx, bin, env, []string{"app-server"}, requests, func(line []byte) []Model {
		var reply struct {
			ID     int
			Result struct {
				Data []struct {
					Model                     string
					DisplayName               string
					Hidden                    bool
					SupportedReasoningEfforts []struct {
						ReasoningEffort string
					}
				}
			}
		}
		if json.Unmarshal(line, &reply) != nil || reply.ID != 1 {
			return nil
		}
		models := make([]Model, 0, len(reply.Result.Data))
		for _, entry := range reply.Result.Data {
			if entry.Hidden || entry.Model == "" {
				continue
			}
			model := Model{Slug: entry.Model, Name: entry.DisplayName}
			if model.Name == "" {
				model.Name = model.Slug
			}
			for _, effort := range entry.SupportedReasoningEfforts {
				model.Efforts = append(model.Efforts, effort.ReasoningEffort)
			}
			models = append(models, model)
		}
		return models
	})
}

// rememberCatalog returns the models to show for a harness: the catalog it just
// reported, saved for later, or the last saved one when this read failed.
func (m *Manager) rememberCatalog(id string, st Status) []Model {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.catalogs == nil {
		m.catalogs = map[string][]Model{}
		if raw, err := os.ReadFile(m.catalogFile()); err == nil {
			_ = json.Unmarshal(raw, &m.catalogs)
		}
	}
	if st.Catalog {
		m.catalogs[id] = st.Models
		if m.dataDir != "" {
			if raw, err := json.Marshal(m.catalogs); err == nil {
				_ = os.WriteFile(m.catalogFile(), raw, 0o600)
			}
		}
		return st.Models
	}
	if saved := m.catalogs[id]; len(saved) > 0 {
		return saved
	}
	return st.Models
}

func (m *Manager) catalogFile() string { return filepath.Join(m.dataDir, "model-catalog.json") }
