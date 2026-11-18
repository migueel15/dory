# ADR-001: Estados estructurados para items y features

- Status: superseded by ADR-007
- Date: 2026-10-30

## Context

El MVP expresaba parte del estado con convenciones de texto: `[en curso]` en el título de la feature, etiquetas `bloqueado` y `cancelado`, el avance dentro de `body_md` y la pertenencia a una feature con un enlace `part_of` de `kind` libre. Además, un item podía estar `archived` por estado y por `archived_at`, con efectos distintos. Los agentes tenían que escribir y borrar ese texto a mano, y nada se podía filtrar de forma fiable.

## Decision

El estado se guarda en campos con valores cerrados, nunca en texto:

- Features: `status` es `planned`, `in_progress`, `implemented` o `deprecated`. Lo cambia una persona o un agente de forma explícita; el servidor no lo calcula.
- Items: `status` es `inbox`, `triaged`, `ready`, `in_progress`, `validating`, `blocked`, `done` o `cancelled`. `validating` indica que la implementación terminó y falta comprobarla, por una persona o de forma automática. `blocked` exige `blocked_reason`, que se vacía al salir de ese estado.
- `archived` deja de ser un estado: archivar solo oculta el registro (`archived_at`), como en el resto de entidades.
- Un item pertenece como mucho a una feature mediante `feature_id`. Los enlaces genéricos quedan para otras relaciones.
- El estado actual del trabajo va en `items.progress`, texto corto que se reemplaza; el historial vive en git y en la PR.
- `get_project` devuelve los items abiertos de cada feature para decidir cuándo cerrarla.

## Consequences

- Una migración traduce los datos existentes: `[en curso]` pasa a `in_progress`, la etiqueta `bloqueado` a `blocked`, `archived` con `cancelado` a `cancelled`, y los enlaces `part_of` a `feature_id`.
- MCP, REST, OpenAPI y la web cambian sus enumeraciones; el kanban muestra `blocked` y `cancelled`.
- `$dory-workflow` deja de usar convenciones de texto.
