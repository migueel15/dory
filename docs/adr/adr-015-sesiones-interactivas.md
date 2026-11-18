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

Codex se lanza con `codex app-server --enable default_mode_request_user_input`, hablando su JSON-RPC por stdio: `initialize`, `thread/start` o `thread/resume` con el id de sesión guardado, y `turn/start`. Sus items se traducen a los mismos `ToolCall` que los de `codex exec --json`. La herramienta `request_user_input` solo existe en el modo por defecto con esa opción experimental. `item/tool/requestUserInput` se convierte en la petición, y la respuesta lleva las respuestas por id de pregunta. Las aprobaciones no deberían llegar, porque se usa `approvalPolicy: never`; si llegan, se rechazan.

OpenCode corre cada turno en su propio `opencode serve --port 0`, con una contraseña aleatoria en `OPENCODE_SERVER_PASSWORD`. Dory crea o reanuda la sesión, sigue `/api/event` y envía el texto con `/api/session/{id}/prompt`. Su herramienta `question` abre un formulario (`form.created`) que pausa el turno, y se responde con `/api/session/{id}/form/{formID}/reply` usando las claves de cada campo. El servidor se para cuando termina el turno.

El chat dibuja la pregunta nativa como una tarjeta con las preguntas y sus respuestas, sin burbuja del usuario. El formulario abierto va anclado encima del composer. Los hilos antiguos siguen mostrando sus llamadas a `ask_user`, cuya respuesta era el siguiente mensaje del usuario. La herramienta sigue disponible en el MCP de Dory para quien la use.

## Consequences

Un arnés con sesión interactiva solo tiene que implementar `Interactive` para pausar el turno. Los tres arneses (Claude Code, Codex y OpenCode) pausan el turno con su propia herramienta de preguntas. Si Codex quita o cambia la opción experimental `default_mode_request_user_input`, sus preguntas volverán a terminar el turno. Si el nodo se reinicia con una pregunta pendiente, el turno se pierde y el hilo queda en error. Las peticiones de permiso podrán usar la misma vía.
