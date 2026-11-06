# ADR-005: Suscripción persistente a T3 Code

- Status: accepted
- Date: 2026-11-06

## Context

Dory leía el shell de cada instancia de T3 Code cada 10 s y la SPA sondeaba cada 10 s. Un cambio de estado tardaba de 10 a 20 s en verse, también justo después de que Dory lanzara un hilo. Dory y T3 Code trabajan juntos en la misma máquina, y el usuario quiere ver los cambios casi al instante.

## Decision

- El monitor mantiene por cada instancia una conexión WebSocket con `orchestration.subscribeShell` (`omitIdleSubagents`, `omitOlderSettled`). Responde `Pong` a cada `Ping` y `Ack` a cada `Chunk`, porque T3 no manda el siguiente chunk sin ese `Ack`.
- Los chunks se usan solo como señal. Tras cada ráfaga (200 ms), Dory vuelve a leer la instancia con la llamada HTTP de siempre. Así un cambio en el formato del stream no estropea lo que Dory muestra.
- Si la suscripción cae, se reintenta con espera de 1 s a 30 s y el sondeo de 10 s cubre ese tiempo. Con la suscripción viva, cada instancia se vuelve a leer al menos cada 60 s.
- Tras cada escritura de Dory en T3 (lanzar, definir, enviar, responder), la instancia se relee al momento y de nuevo a 1 s y a 3 s.
- El SSE `/api/events/stream` añade `event: change` con `{"entity_type":"t3"}`, sin `id`, cuando cambia lo que muestra el monitor. Son señales en memoria y no se pueden reanudar.
- `GET /api/t3/threads/{id}/stream` abre `orchestration.subscribeThread` mientras la SPA muestra esa conversación y emite `event: thread` tras cada ráfaga (250 ms). La SPA vuelve a pedir la conversación. Se aplica la misma regla de acceso que a las lecturas, con la excepción de ADR-004 para hilos enlazados a un item.
- Esto enmienda en ADR-002 la «conexión por llamada», que sigue valiendo para las escrituras.

## Consequences

- El estado y el chat se actualizan en 1–2 s y la SPA sondea solo cada 60 s como respaldo.
- Hay una conexión abierta por instancia y otra por cada conversación abierta en una pestaña.
- El protocolo de stream de T3 es interno; si cambia, Dory vuelve al sondeo y lo registra en el log.
