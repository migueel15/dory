package codex

import (
	"context"
	"encoding/json"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func codexModels(ctx context.Context, bin string, env []string) []harness.Model {
	const requests = `{"id":0,"method":"initialize","params":{"clientInfo":{"name":"dory","version":"0.1.0"}}}
{"method":"initialized"}
{"id":1,"method":"model/list","params":{"limit":1000}}
`
	return kit.ReadModelCatalog(ctx, bin, env, []string{"app-server"}, requests, func(line []byte) []harness.Model {
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
		models := make([]harness.Model, 0, len(reply.Result.Data))
		for _, entry := range reply.Result.Data {
			if entry.Hidden || entry.Model == "" {
				continue
			}
			model := harness.Model{Slug: entry.Model, Name: entry.DisplayName}
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
