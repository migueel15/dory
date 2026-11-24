# ADR-039: Actor por hilo y cola persistida

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-008

## Context

El runtime guardaba el estado de cada turno en varios mapas bajo un mutex y en banderas sueltas. El ciclo de vida del turno, la cola de mensajes y los ajustes pendientes dependían de ese estado compartido y producían carreras.

## Decision

- Cada hilo activo tiene un `threadRunner`: una goroutine dueña del turno, la sesión, la cola, las peticiones abiertas y los ajustes pendientes. Recibe órdenes por canal y responde con errores tipados.
- Los estados son `Idle`, `Starting`, `Running`, `WaitingInput` y `Stopping`. Una tabla explícita de estado y orden decide la acción; un par no listado devuelve un error y no cambia el estado.
- El actor publica una vista atómica de solo lectura (`Running`, `Queue`, `Pending`, `Active`), de modo que las consultas no esperan al actor. Termina tras 10 minutos sin turno ni cola.
- `Manager` conserva el registro de actores, las suscripciones y la caché de catálogo y detección. El registro de lo que ocurre en un turno (mensajes, consumo, diff, rama) vive en `recorder.go` y no toca el estado del actor.
- La cola se guarda en SQLite (`thread_queue`).

## Consequences

- Tras reiniciar el nodo, `Recover` carga las colas y las muestra, pero no las ejecuta: el usuario las envía con reconducir o al enviar otro mensaje.
- Enviar un mensaje mientras el turno se detiene lo encola y no se ejecuta hasta que otro turno vacíe la cola.
