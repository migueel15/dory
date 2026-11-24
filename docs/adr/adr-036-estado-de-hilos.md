# ADR-036: Estado de hilos fuera del registro replicado

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-009, ADR-033, ADR-034

## Context

El inicio y el fin de cada turno modificaban la fila replicada del hilo. Cada modificación agrandaba el registro y obligaba a reconstruir la fila a partir de su historial.

## Decision

El nodo dueño guarda el estado, el error y la última actividad del hilo en `thread_state`. Publica los de sus hilos no archivados en `/peer/v2/info`. Los pares conservan la última copia y la marcan como desactualizada cuando el dueño se desconecta.

El registro replica la organización del hilo, la fecha de lectura y los avisos de la bandeja. El hilo tiene actividad sin leer cuando la última actividad es posterior a la última lectura o aún carece de fecha de lectura.

## Consequences

La migración conserva el estado actual, elimina del registro las operaciones que solo transportaban estado y preserva la secuencia máxima del nodo. La malla se actualiza en conjunto para que los nodos anteriores dejen de emitir operaciones de estado.
