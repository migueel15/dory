# ADR-038: Capa de aplicación para los hilos

- Status: accepted
- Date: 2026-11-24

## Context

La API y el MCP aplicaban reglas distintas al crear hilos. Un fallo de Git podía ocurrir después de publicar el hilo en la malla y dejarlo sin runtime local.

## Decision

`internal/app` coordina la creación y el archivado en el nodo propietario. La creación resuelve la selección del modelo y prepara el workspace antes de registrar el hilo. El oplog y el runtime local se escriben en la misma transacción. Si falla el registro, se retiran el worktree nuevo y la rama creada para él.

## Consequences

La API, el MCP y las peticiones entre nodos comparten los ajustes y la validación del nodo de destino. Si falla el envío del primer mensaje, el error incluye el identificador del hilo guardado para poder continuar en él.
