# ADR-044: Capa de aplicación y dominio

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-038

## Context

Las reglas de negocio estaban repartidas entre los handlers HTTP y las herramientas MCP, con comportamientos distintos para una misma operación.

## Decision

- `internal/domain` contiene reglas puras sin E/S (`domain/board` para las columnas del tablero y el cierre de tareas).
- `internal/app` ofrece servicios (`Tasks`, `Plans`, `Memories`, `Projects`, `Ideas`, `Session`, además de los hilos del ADR-038) que usan la API, el MCP y las peticiones entre nodos. Los handlers solo decodifican, llaman al servicio y codifican.
- `internal/mcp` no importa `internal/api`: `Handler(*Deps)` recibe los servicios. `Deps` conserva `Store` para herramientas de hilos, enlaces, dependencias y búsqueda, que no tienen servicio propio.
- El tablero se calcula con datos locales: las pull requests se guardan en una caché local (`task_pulls`) que actualiza el vigilante de GitHub con TTL y límite de consultas.
- `main.go` construye los servicios una vez; no hay un agregado `app.App`.

## Consequences

- `api.Server` conserva campos de infraestructura (runtime, eventos, nodo) y no es un recorte completo.
- `CheckDone` usa solo la caché: una PR sin estado conocido no bloquea.
