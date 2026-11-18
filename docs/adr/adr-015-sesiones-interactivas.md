# ADR-015: Sesiones interactivas de los arneses

- Status: accepted
- Date: 2026-11-18

## Context

Las preguntas del agente pasaban por la herramienta MCP `ask_user` (ADR-013), que contestaba al momento. El agente no esperaba y podía seguir escribiendo debajo de la pregunta. La respuesta llegaba como un mensaje nuevo del usuario, y la pregunta abierta quedaba perdida en medio del chat. Las apps oficiales y T3 Code pausan el turno: Claude Code pide permiso para usar la herramienta por su protocolo de control y Codex `app-server` envía `item/tool/requestUserInput`. En los dos casos la respuesta vuelve como el resultado de la herramienta.

## Decision

`internal/runtime` separa cómo corre cada arnés:

- `Runner`: un proceso por turno con el que no se puede hablar mientras trabaja.
- `Interactive`: el turno es una `Session` con el proceso abierto. Emite eventos y acepta `Answer(requestID, answers)`. Para interrumpir se cancela su contexto.

Un evento `Request` (id, tipo `question` y, más adelante, `permission`, y las preguntas) pausa el turno. El `Manager` lo guarda en `thread_requests`, pone el hilo en `waiting` y lo expone en la vista del hilo como `requests`. `POST /api/threads/{id}/requests/{reqId}` con `answers` (una lista por pregunta) llama a `Session.Answer`. Cuando termina el turno se borran sus peticiones, y al arrancar el nodo se descartan las que quedaron, porque su proceso ya no existe.

Claude Code se lanza con `--input-format stream-json --permission-prompt-tool stdio`, hablando el protocolo directamente desde Go, sin el SDK. Ante un `control_request` `can_use_tool` de `AskUserQuestion` se crea la petición, y se contesta con `allow` y `updatedInput.answers` (pregunta → respuesta; varias opciones se unen con ", "). Con acceso total, el resto de herramientas se permite; sin él, se rechazan como antes, hasta que haya aprobaciones en Dory. Ya no se desactiva `AskUserQuestion`.

El chat dibuja la pregunta nativa como una tarjeta con las preguntas y sus respuestas, sin burbuja del usuario. El formulario abierto va anclado encima del composer. Los hilos antiguos y los arneses sin sesión siguen usando `ask_user`.

## Consequences

Un arnés con sesión interactiva solo tiene que implementar `Interactive` para pausar el turno. Codex (`app-server`) y OpenCode irán en pasos posteriores; mientras tanto siguen con `ask_user`. Si el nodo se reinicia con una pregunta pendiente, el turno se pierde y el hilo queda en error. Las peticiones de permiso podrán usar la misma vía.
