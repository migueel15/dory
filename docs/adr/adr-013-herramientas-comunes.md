# ADR-013: Herramientas comunes entre arneses

- Status: accepted
- Date: 2026-11-18

## Context

Cada arnés nombra y describe sus herramientas a su manera (`Edit` y `TodoWrite` en Claude Code, `apply_patch` y `update_plan` en Codex, `edit` en OpenCode). El chat solo recibía un título y un texto, así que todas se veían como la misma línea genérica. Además, `AskUserQuestion` de Claude Code no llega al usuario en modo `-p`, de modo que las preguntas fallaban siempre.

## Decision

Cada adaptador traduce sus llamadas a un `ToolCall` de Dory con uno de estos tipos: `edit`, `command`, `todo`, `plan`, `read`, `search`, `web`, `agent`, `mcp`, `question` u `other`. El mensaje guarda ese `ToolCall` (columna `thread_messages.tool`) y el chat dibuja una tarjeta por tipo, sea cual sea el arnés.

Las preguntas al usuario pasan siempre por la herramienta MCP `ask_user` de Dory, que acepta preguntas con cabecera, opciones y selección múltiple. Dory desactiva `AskUserQuestion` en Claude Code y le indica que use `ask_user`. La respuesta vuelve como el siguiente mensaje del usuario.

En Claude Code, esta vía queda sustituida por ADR-015: `AskUserQuestion` vuelve a estar activa y pausa el turno.

## Consequences

Un arnés nuevo solo tiene que mapear sus herramientas a estos tipos para verse bien en el chat. Lo que no encaja se muestra como `other` o `mcp`, con sus argumentos. Los mensajes guardados antes de este cambio se reconstruyen al leerlos a partir de su título y su argumento principal, así que también usan las tarjetas comunes, aunque con menos detalle (sin diff ni descripción).
