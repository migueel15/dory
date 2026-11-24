package claude

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func claudeModels(ctx context.Context, bin string, env []string) []harness.Model {
	return kit.ReadModelCatalog(ctx, bin, env, []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"},
		"{\"type\":\"control_request\",\"request_id\":\"models\",\"request\":{\"subtype\":\"initialize\"}}\n",
		func(line []byte) []harness.Model {
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
			models := make([]harness.Model, 0, len(entries))
			for _, entry := range entries {
				if entry.Value != "" && entry.Value != "default" && strings.TrimSpace(entry.DisplayName) != "" {
					models = append(models, harness.Model{Slug: entry.Value, Name: entry.DisplayName, Efforts: entry.SupportedEffortLevels, ResolvedModel: entry.ResolvedModel})
				}
			}
			return models
		})
}
