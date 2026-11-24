# ADR-007: Malla de nodos iguales con SQLite y registro de operaciones

- Status: accepted
- Date: 2026-11-16
- Supersedes: ADR-001 (en lo que toca a estados de items y features)

## Context

Dory v1 era un servidor central con Postgres. Hablaba con T3 Code en cada máquina. Sin red no había Dory, y cada función nueva dependía de la API interna de T3. El documento de requisitos de la v2 pide que cada máquina funcione sola y se sincronice con las demás cuando se ven.

## Decision

- Cada máquina ejecuta el mismo binario, un nodo completo con API, MCP, interfaz, base de datos y runtime de agentes. El servidor 24/7 es un nodo más.
- Cada nodo guarda sus datos en SQLite (modernc, sin cgo), con búsqueda FTS5.
- Cada cambio de una tabla replicada se guarda como una operación con id de nodo, secuencia, reloj lógico híbrido (HLC) y los campos cambiados. La fila se reconstruye desde sus operaciones y gana el valor más reciente por campo.
- Hay tres excepciones a esa regla:
  - Si dos nodos editan a la vez el texto de una memoria, se conservan las dos versiones.
  - Si dos proyectos comparten slug, el de id menor lo conserva y el otro recibe un sufijo.
  - El id de un repositorio se deriva de su remoto, así que el mismo repositorio es siempre la misma fila.
- Las rutas locales, las cuentas, los tokens, las credenciales y las conversaciones no se replican.
- Las tareas sustituyen a items y features. Se ligan a issues y PRs, y tienen dependencias entre ellas.
- La columna del kanban no se guarda: se deriva de los hilos y de la PR. Solo se guarda lo que decide el usuario antes de empezar: inbox, ready o cancelled.

## Consequences

- Un nodo funciona sin red, y los nodos convergen al reconectar.
- La v2 empieza vacía: no hay migración de datos de la v1.
- El registro de operaciones crece sin compactar. La compactación se resolvió en ADR-041.
