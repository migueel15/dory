# ADR-004: Definición interactiva de tickets con plan versionado

- Status: accepted
- Date: 2026-11-06

## Context

Una idea pasaba a ticket con un análisis de un solo turno (`triage`). Si el agente no tenía dudas, escribía `body_md` y dejaba el item en `ready` él mismo. Cada corrección (`replan`) abría un hilo nuevo, sin el contexto anterior. Las preguntas del agente se quedaban en T3 Code y el plan era texto libre, sin versiones. El usuario quiere hacer todo el proceso desde Dory, conversando con el agente, y decidir él cuándo un ticket está listo.

## Decision

- Cada item tiene un único hilo de definición en T3 Code (rol `refine` en `item_threads`), que se reutiliza en cada iteración. Dory lo crea con `POST /api/items/{id}/refine` y le envía los mensajes del usuario.
- El plan se guarda en `item_plans` como versiones que no cambian. Tiene secciones fijas (`goal`, `scope`, `where`, `design`, `requirements`, `validation`, `risks`) y preguntas abiertas. El agente publica cada versión con la herramienta MCP `propose_plan`; el usuario puede guardar la suya desde la web.
- Solo el usuario acepta un plan. Al aceptarlo, el plan se copia a `body_md` y el item pasa a `ready`. No se puede aceptar con preguntas abiertas. Proponer una versión nueva devuelve el item a `triaged`, que la web muestra como «En definición».
- La conversación sigue viviendo en T3 Code (ADR-002); Dory la muestra completa sin copiarla.
- Excepción al ADR-002: el hilo de definición también se alcanza por la instancia en la que Dory lo lanzó, aunque esa instancia ya no lo liste por estar asentado. Solo vale para hilos creados por Dory y vinculados al item.
- Se retiran los lanzamientos `triage` y `replan`. `/launch` solo implementa items `ready`. Los vínculos antiguos con esos roles se conservan como historial.

## Consequences

- La vista «Definir» (`/items/{id}/define`) es el único camino de definición; «Definir todo» del Inbox abre un hilo de definición por nota.
- Sustituye la decisión del 2026-10-31 de que el agente de análisis dejara el item en `ready`.
- Si la máquina del hilo no está disponible, el plan sigue visible pero el chat no; el usuario puede empezar un hilo nuevo (`restart`).
- Los agentes de implementación siguen leyendo `body_md`, así que su flujo no cambia.
