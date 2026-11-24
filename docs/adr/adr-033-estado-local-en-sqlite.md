# ADR-033: Estado local en SQLite

- Status: accepted
- Date: 2026-11-24

## Context

Las preferencias y cachés del nodo se guardaban en archivos JSON, cada uno con su propia lógica de persistencia. Todos tenían el mismo ámbito local.

## Decision

Guardar estos valores en la tabla local `node_kv` de `dory.db`, fuera del registro de operaciones de la malla. Los consumidores conservan sus tipos y usan transacciones al leer y modificar una clave. Las credenciales, claves e identidad en archivos mantienen sus rutas para los componentes que las leen directamente.

## Consequences

En el arranque se importan los archivos de estado anteriores una sola vez por clave y se archivan con el sufijo `.migrated`. Cada valor de SQLite tiene prioridad sobre su archivo anterior. Los campos ausentes en los ajustes conservan los valores por defecto de `config.toml`.
