# ADR-040: Registro de eventos por hilo y stream SSE reanudable

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-008

## Context

La conversación de cada hilo se reescribía con `UPSERT`. El stream descartaba actualizaciones sin número de secuencia y existía una bandeja `Reload` como salida de emergencia. Un `resync` borraba y reinsertaba el historial desde los ficheros del arnés.

## Decision

- `thread_events (thread_id, seq, run_id, type, item_id, payload, created_at)` es un registro local de solo inserción con una secuencia por hilo, sin huecos. El evento y la proyección de mensajes (`thread_messages`) se escriben en la misma transacción.
- Los eventos durables son `turn.started/ended`, `item.started/completed`, `request.opened/resolved`, `queue.changed`, `state.changed`, `settings.reported`, `session.bound` y `marker`. Los fragmentos de texto viajan solo en vivo (`delta`).
- `GET /api/threads/{id}/stream` reanuda desde `Last-Event-ID` o `?after=` y pasa a vivo. Sin ellos envía un `snapshot` con el `seq`. Un suscriptor lento se cierra para que vuelva a reanudar, no se descarta nada en silencio.
- `/api/events` queda para pistas de entidades y actividad, con `epoch` por arranque.
- `Follow` solo añade: importa elementos posteriores al último mensaje del hilo. Ningún camino borra mensajes salvo el rebobinado explícito, que además registra un `marker`.

## Consequences

- El registro no se purga con el rebobinado: es el historial.
- Cada delta indica la longitud acumulada del texto en unidades UTF-16. Si la web detecta un hueco, vuelve a consultar el hilo y reconecta. Al conectar, recibe el texto completo de los elementos en curso y descarta los fragmentos ya aplicados.
- El registro es local al nodo y no se replica (véase ADR-036).
