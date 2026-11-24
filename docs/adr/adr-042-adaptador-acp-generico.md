# ADR-042: Adaptador ACP genérico con perfiles

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-018

## Context

La implementación del protocolo ACP era exclusiva de Antigravity y vivía en `internal/runtime`.

## Decision

- `internal/harness/acp` implementa el cliente JSON-RPC por stdio, la traducción de actualizaciones, los permisos con guardia de rutas del worktree, la elicitación, los modelos y las sesiones externas. Un `Profile` describe lo específico de cada arnés.
- `internal/harness/acp/antigravity` es el primer perfil: instalación, cuotas, historial de uso, skills e instrucciones.
- Se declara otro arnés ACP en `config.toml` con `[[acp]]` (`id`, `name`, `command`, `args`, `env`) sin código nuevo. Un perfil declarado es un driver simple, sin cuotas, catálogo ni comandos de administración. Los ids `claude`, `codex`, `opencode` y `antigravity` están reservados.
- Los adaptadores emiten eventos tipados nativos de `harness/kit`.

## Consequences

- Los perfiles declarados anuncian reanudación y peticiones durante el turno; si el servidor no admite `loadSession`, abren una sesión nueva.
