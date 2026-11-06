package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

const roleRefine = "refine"

// refinePrompt sets up the single definition thread of an item. The agent
// talks with the user through Dory and never decides when the item is ready.
const refinePrompt = `Eres el agente de definición de este item. El usuario conversa contigo desde Dory hasta dejar claro qué hay que implementar. Consulta el código y las herramientas MCP de Dory (session_start, get_project, get_item). No cambies el código ni implementes nada.

1. Analiza qué se pide, dónde vive en el código, qué arquitectura y dependencias toca y cómo se validará.
2. Publica el plan con propose_plan para este item, con las secciones goal (objetivo), scope (alcance y lo que queda fuera), where (archivos y módulos), design (diseño y arquitectura), requirements (requisitos y dependencias), validation (cómo probar) y risks (riesgos). Pon en open_questions las dudas que cambien el resultado.
3. Si tienes dudas, pregúntalas con tu herramienta de preguntas estructuradas, con opciones y respuesta libre.
4. Tras cada respuesta o comentario del usuario, publica una versión nueva del plan con propose_plan.
5. Con update_item puedes ajustar el título, tags, project_id y feature_id (crea la feature con upsert_feature si es una capacidad nueva; los bugs y tareas menores no la necesitan). No toques body_md ni el estado: solo el usuario acepta el plan desde Dory.

Termina cada turno con un resumen breve de lo que ha cambiado en el plan.`

func refineFirstMessage(item service.Item, text string) string {
	msg := fmt.Sprintf("Item de Dory %s\nTítulo: %s\nTexto original: %s\nDescripción:\n%s\n\n%s", item.ID, item.Title, item.RawText, item.BodyMd, refinePrompt)
	if text != "" {
		msg += "\n\nIndicaciones del usuario:\n" + text
	}
	return msg
}

// refineThread is the newest definition thread of an item, or nil.
func refineThread(r *http.Request, svc service.Service, itemID string) (*service.ItemThread, error) {
	links, err := svc.ItemThreads().List(r.Context(), service.Filter{Fields: map[string]any{"item_id": itemID, "role": roleRefine}, Limit: 1000})
	if err != nil || len(links) == 0 {
		return nil, err
	}
	return &links[len(links)-1], nil
}

// refineClient reaches the definition thread. T3 stops reporting older
// settled threads, so a thread Dory created for this item is also reached
// through the instance it was launched on.
func refineClient(r *http.Request, monitor *t3.Monitor, link service.ItemThread) (t3.Client, error) {
	if c, err := monitor.ThreadClient(r.Context(), link.ThreadID); err == nil {
		return c, nil
	}
	return monitor.InstanceClient(r.Context(), link.InstanceID)
}

func definable(status string) bool {
	return status == "inbox" || status == "triaged" || status == "ready"
}

func mountRefine(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor) {
	mux.HandleFunc("POST /api/items/{id}/refine", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
			// Restart opens a new definition thread instead of reusing the last one.
			Restart      bool   `json:"restart"`
			InstanceID   string `json:"instance_id"`
			ProjectID    string `json:"project_id"`
			Provider     string `json:"provider"`
			Model        string `json:"model"`
			EffortOption string `json:"effort_option"`
			Effort       string `json:"effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		body.Text = strings.TrimSpace(body.Text)
		item, err := svc.Items().Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if !definable(item.Status) {
			writeError(w, http.StatusConflict, "conflict", "only inbox, triaged or ready items can be defined")
			return
		}
		link, err := refineThread(r, svc, item.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if link != nil && !body.Restart {
			if body.Text == "" {
				writeError(w, http.StatusBadRequest, "invalid_argument", "text is required")
				return
			}
			c, err := refineClient(r, monitor, *link)
			if err == nil {
				err = c.SendMessage(r.Context(), link.ThreadID, body.Text)
			}
			if err != nil {
				writeT3Error(w, err)
				return
			}
			monitor.Refresh(link.InstanceID)
			audit(r, svc, link.ThreadID, map[string]any{"operation": "t3.message", "item_id": item.ID, "text": body.Text})
			writeJSON(w, http.StatusOK, map[string]string{"thread_id": link.ThreadID})
			return
		}
		if body.InstanceID == "" || body.ProjectID == "" || body.Provider == "" || body.Model == "" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "instance_id, project_id, provider and model are required to start the definition")
			return
		}
		c, err := monitor.InstanceClient(r.Context(), body.InstanceID)
		if err != nil {
			writeT3Error(w, err)
			return
		}
		threadID, err := c.Launch(r.Context(), t3.LaunchInput{
			ProjectID: body.ProjectID, Title: item.Title, Provider: body.Provider, Model: body.Model, EffortOption: body.EffortOption, Effort: body.Effort,
			Prompt: refineFirstMessage(item, body.Text),
		})
		if threadID == "" {
			writeT3Error(w, err)
			return
		}
		monitor.Refresh(body.InstanceID)
		// The thread exists from here on, so keep it linked even if its turn failed.
		if _, lerr := svc.ItemThreads().Put(r.Context(), actor(r), service.ItemThread{ItemID: item.ID, InstanceID: body.InstanceID, ThreadID: threadID, Role: roleRefine}); lerr != nil {
			logError(lerr)
		}
		if err != nil {
			writeT3Error(w, err)
			return
		}
		if item.Status == "inbox" {
			item.Status = "triaged"
			if _, err := svc.Items().Put(r.Context(), actor(r), item); err != nil {
				logError(err)
			}
		}
		audit(r, svc, threadID, map[string]any{"operation": "t3.launch", "item_id": item.ID, "role": roleRefine, "model": body.Model, "effort": body.Effort})
		writeJSON(w, http.StatusCreated, map[string]string{"thread_id": threadID})
	})
	mux.HandleFunc("GET /api/items/{id}/refinement", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := svc.Items().Get(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		plan, err := service.LatestPlan(r.Context(), svc, id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		link, err := refineThread(r, svc, id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := refinement{Plan: plan, Messages: []t3.Message{}}
		if link == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
		out.Thread = &refinementThread{ThreadID: link.ThreadID, InstanceID: link.InstanceID}
		if live, ok := monitor.Thread(link.ThreadID); ok {
			out.Thread.Status, out.Thread.Attention, out.Thread.URL, out.Thread.Model, out.Thread.Effort = live.Status, live.Attention, live.URL, live.Model, live.Effort
		}
		// An unreachable instance still shows the plan; the error explains the empty chat.
		c, err := refineClient(r, monitor, *link)
		if err == nil {
			var conv t3.ThreadContext
			if conv, err = c.ThreadConversation(r.Context(), link.ThreadID); err == nil {
				out.Messages, out.Pending = conv.Messages, conv.Pending
			}
		}
		if err != nil {
			out.ThreadError = threadErrorText(err)
		}
		writeJSON(w, http.StatusOK, out)
	})
}

type refinementThread struct {
	ThreadID   string `json:"thread_id"`
	InstanceID string `json:"instance_id"`
	// Status is empty when no instance reports the thread right now.
	Status    string `json:"status"`
	Attention bool   `json:"attention"`
	URL       string `json:"url"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
}

// refinement is everything the definition view shows.
type refinement struct {
	Thread      *refinementThread `json:"thread"`
	Messages    []t3.Message      `json:"messages"`
	Pending     *t3.Pending       `json:"pending"`
	ThreadError string            `json:"thread_error,omitempty"`
	Plan        *service.ItemPlan `json:"plan"`
}

func threadErrorText(err error) string {
	if errors.Is(err, t3.ErrInstanceUnavailable) {
		return "La máquina del hilo no está disponible."
	}
	return err.Error()
}
