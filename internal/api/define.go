package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// definePrompt asks the agent to turn an issue into well defined Dory items
// without implementing anything: implementing stays a click of the user.
const definePrompt = `Define en Dory las tareas necesarias para resolver esta issue. No cambies el código ni implementes nada.

1. Carga el contexto con las herramientas MCP de Dory (session_start, get_project, recall) y lee la issue completa con sus comentarios (gh issue view %[1]s --comments).
2. Analiza el repositorio para saber qué hay que cambiar: ficheros, componentes, contratos y riesgos.
3. Divide el trabajo en items pequeños y revisables (cerca de 400 líneas de código funcional como mucho cada uno), en orden de implementación.
4. Crea cada item con create_item en el proyecto %[2]s, en estado ready, con:
   - title claro; body_md con objetivo, alcance (qué ficheros o piezas cambiar y qué hacer en cada uno), dependencias con otros items y cómo probarlo;
   - refs que incluyan exactamente la URL de la issue: %[1]s
   - feature_id: reutiliza la feature existente del proyecto que corresponda a cada item, o crea una con upsert_feature si es una capacidad nueva. No metas todos los items en la misma feature si tocan capacidades distintas; los bugs y tareas menores no necesitan feature.
5. Antes de crear, busca con list_items(fields: {refs: ["%[1]s"]}) items ya creados para esta issue y actualízalos en lugar de duplicarlos.

Si hay dudas que cambien el resultado, crea solo lo que esté claro y plantea las dudas en este hilo.

Termina con un resumen breve: los items creados, su feature y el orden propuesto.`

func defineIssuePrompt(issue *github.Issue, body, projectID string) string {
	head := fmt.Sprintf("Issue %s#%d: %s\nURL: %s\n\nDescripción de la issue:\n%s\n\n", issue.Repo, issue.Number, issue.Title, issue.URL, body)
	return head + fmt.Sprintf(definePrompt, issue.URL, projectID)
}

func mountDefine(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor, gh *github.Client) {
	mux.HandleFunc("POST /api/github/repos/{owner}/{repo}/issues/{number}/define", func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return
		}
		var body struct {
			InstanceID   string `json:"instance_id"`
			ProjectID    string `json:"project_id"`
			Provider     string `json:"provider"`
			Model        string `json:"model"`
			EffortOption string `json:"effort_option"`
			Effort       string `json:"effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InstanceID == "" || body.ProjectID == "" || body.Provider == "" || body.Model == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "instance_id, project_id, provider and model are required")
			return
		}
		detail, err := gh.Detail(r.Context(), r.PathValue("owner")+"/"+r.PathValue("repo"), "issues", number)
		if err != nil {
			writeGithubError(w, err)
			return
		}
		if detail.Issue == nil {
			writeError(w, http.StatusNotFound, "not_found", "issue not found")
			return
		}
		issue := detail.Issue
		c, err := monitor.InstanceClient(r.Context(), body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		threadID, err := c.Launch(r.Context(), t3.LaunchInput{
			ProjectID: body.ProjectID, Title: fmt.Sprintf("Definir tareas #%d: %s", issue.Number, issue.Title),
			Provider: body.Provider, Model: body.Model, EffortOption: body.EffortOption, Effort: body.Effort,
			Prompt: defineIssuePrompt(issue, detail.Body, issue.ProjectID),
		})
		if threadID == "" {
			writeT3Error(w, err)
			return
		}
		// The thread exists from here on, so keep it tracked even if its turn failed.
		link := service.IssueThread{ProjectID: issue.ProjectID, Repo: issue.Repo, Number: issue.Number, IssueURL: issue.URL, Title: issue.Title,
			InstanceID: body.InstanceID, ThreadID: threadID, Model: body.Model, Effort: body.Effort}
		if _, lerr := svc.IssueThreads().Put(r.Context(), actor(r), link); lerr != nil {
			logError(lerr)
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.define_issue", "issue": issue.URL, "model": body.Model, "effort": body.Effort})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
	})
}
