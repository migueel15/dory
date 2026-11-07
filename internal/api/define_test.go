package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/github"
)

func TestDefineIssuePromptPlans(t *testing.T) {
	issue := &github.Issue{Repo: "owner/repo", Number: 42, Title: "Una mejora", URL: "https://github.com/owner/repo/issues/42"}
	prompt := defineIssuePrompt(issue, "Descripción completa", "project-id")
	for _, instruction := range []string{
		"Issue owner/repo#42: Una mejora", "Descripción completa",
		"gh issue view " + issue.URL + " --comments",
		"una sola vez para toda la issue", "sin repetirlo en cada uno",
		"400 líneas de código funcional", "cambio de código propio y verificable",
		"menos items de tamaño razonable", "5-6 items", "avisa en el resumen",
		`list_items(fields: {refs: ["` + issue.URL + `"]})`,
		"en lugar de duplicarlos", "no lo borres ni archives",
		"create_item en el proyecto project-id, en estado triaged",
		"body_md breve", "refs que incluyan exactamente la URL de la issue: " + issue.URL,
		"feature_id", "plan v1 con propose_plan", "7 secciones concisas",
		"goal (", "scope (", "where (", "design (", "requirements (", "validation (", "risks (",
		"solo el usuario acepta el plan", "No implementes ni avances el estado",
		"open_questions incluye solo dudas específicas del item que cambien el resultado",
		"una sola vez en el resumen del hilo, no repetidas en cada plan",
		"decisión razonable anotada en risks", "respuestas del usuario en este mismo hilo",
		"nuevas versiones con propose_plan", "fusiona o actualiza los items existentes",
		"items y su feature, orden propuesto, dudas comunes e items sobrantes",
	} {
		if !strings.Contains(prompt, instruction) {
			t.Errorf("definition prompt missing %q", instruction)
		}
	}
	if strings.Contains(prompt, "ready") {
		t.Error("definition prompt must leave acceptance to the user")
	}
}
