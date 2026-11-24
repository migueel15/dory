// Package i18n holds the Spanish texts the server itself must write: push
// notifications and the error a thread keeps in its state. Everything the chat
// shows is a code the web translates (web/src/i18n/es.ts); keys here repeat
// those codes when the concept is the same.
package i18n

var es = map[string]string{
	"node.restarted":           "El nodo se reinició mientras el agente trabajaba; el turno se interrumpió.",
	"node.restarted_waiting":   "El nodo se reinició mientras el agente esperaba una respuesta; el turno se interrumpió.",
	"turn.interrupted":         "interrumpido",
	"turn.no_response":         "el agente terminó sin respuesta",
	"turn.failed":              "El turno falló",
	"command.clear":            "Empieza una sesión nueva del agente; los mensajes siguen visibles",
	"push.thread.idle":         "El agente terminó",
	"push.thread.error":        "El agente falló",
	"push.thread.waiting":      "Un agente te necesita",
	"push.plan.ready":          "Plan listo para revisar",
	"push.plan.accepted":       "Tarea lista para implementar",
	"push.task.ready_to_merge": "Tarea lista para mergear",
}

// Text is the Spanish text of a code, or the code itself when it has none.
func Text(code string) string {
	if s, ok := es[code]; ok {
		return s
	}
	return code
}
