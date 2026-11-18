# ADR-012: Directorios de estado local

- Status: accepted
- Date: 2026-11-18

## Context

El nodo de desarrollo guardaba la base de datos y los adjuntos dentro del checkout. Si el checkout se abría desde T3 Code, el estado quedaba dentro de su worktree.

## Decision

Dory guarda el estado del nodo instalado en `~/.dory/data` y el de desarrollo en `~/.dory/dev`. El perfil de Electron reside en `~/.dory/desktop`.

## Consequences

Cambiar de checkout no cambia el estado del nodo de desarrollo. El nodo respeta un directorio configurado explícitamente y detecta una instalación anterior cuando todavía no hay datos en la ubicación nueva.
