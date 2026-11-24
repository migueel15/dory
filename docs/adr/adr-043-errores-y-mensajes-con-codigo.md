# ADR-043: Errores y mensajes con código

- Status: accepted
- Date: 2026-11-24

## Context

Los errores mezclaban idiomas, la API traducía pocos casos y el resto devolvía 500 con el texto interno (salida de git, rutas). El backend también redactaba frases en español para la interfaz.

## Decision

- `internal/apperr` define un error con código estable, mensaje técnico en inglés y parámetros. Una única tabla lo traduce a HTTP y otra función a MCP. Un error que no es de dominio responde 500 `internal` sin exponer su texto. El código viaja por la malla con `PeerCall`.
- Un código genérico se afina con el parámetro `reason`. La web busca `code:reason` y después `code`.
- Los mensajes de sistema de un hilo llevan `code` y `params` (`node.restarted`, `turn.interrupted`, `context.cleared`, etc.) y la web los traduce con `web/src/i18n/es.ts`. Un mensaje antiguo sin código muestra su texto.
- El servidor solo redacta en español lo que sale de la interfaz web: títulos push, descripciones guardadas y avisos que también se muestran fuera de la web (`internal/i18n`).
- Un test web falla si un código específico no tiene texto.

## Consequences

- Siguen en español en Go, y no se tradujeron por tener efecto funcional o llegar al agente: `wakeText` y `stateLabel` de las delegaciones (la web reconoce su cabecera), el prompt de handoff y errores técnicos de `devices`, `browser`, `skills`, `update`, `github`, `dictation` y `harness`. La web traduce por código los mensajes de autenticación de GitHub y las acciones del emulador. Los diagnósticos técnicos quedan disponibles para registros y agentes.
- Cuatro casos envían un diagnóstico en el parámetro `detail` que la web muestra tal cual: dispositivos, `update_refused`, `upstream` y `branch_switch_failed`.
- El MCP devuelve al agente el texto de un error no tipado como `internal: <texto>`.
