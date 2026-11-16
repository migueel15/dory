# ADR-008: Runtime de agentes propio, sin T3 Code

- Status: accepted
- Date: 2026-11-16
- Supersedes: ADR-002, ADR-005

## Context

Dory dependía de T3 Code para crear y seguir hilos, y de su API interna, que no tiene contrato estable. La v2 quiere hablar directamente con cada harness.

## Decision

- Cada nodo lanza los harnesses como procesos hijo, con un adaptador por harness:
  - Claude Code: `-p` con stream-json.
  - Codex: `exec --json`, y `resume` para seguir la sesión.
  - OpenCode: `run --format json`, con `--session`.
- Cada adaptador traduce su salida a mensajes normalizados: usuario, asistente, razonamiento, herramienta, plan y sistema.
- Un hilo pertenece al nodo que lo crea. Su conversación y su worktree viven en ese nodo. Los demás nodos lo ven porque la API reenvía las llamadas al nodo dueño por la malla.
- Un hilo de implementación tiene su propia rama y su worktree en `<datos>/worktrees/<hilo>`.
- Los agentes reciben el MCP de Dory del nodo con un token propio del nodo. Si el hilo pide permisos completos, se lanzan sin aprobaciones del harness. Si no, Claude solo acepta ediciones y Codex usa el sandbox workspace-write.
- Hay un proveedor de reproducción para desarrollar sin agentes reales.

## Consequences

- Dory ya no depende de la API de T3.
- Hoy no hay aprobaciones interactivas: un agente solo puede preguntar con la herramienta `ask_user` del MCP.
- El checkpoint por turno de T3 desaparece. El diff de un hilo se calcula con git contra su rama base.
