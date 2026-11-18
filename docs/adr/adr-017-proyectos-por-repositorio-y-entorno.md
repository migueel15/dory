# ADR-017: Proyectos únicos por repositorio y entornos replicados

- Status: accepted
- Date: 2026-11-18
- Amends: ADR-007 (las rutas locales ya no son del todo privadas de cada nodo)

## Context

Al añadir el mismo repositorio desde dos entornos sin red, cada nodo creaba su propio proyecto. El repositorio convergía en una fila, pero quedaba un proyecto huérfano. Además, ningún nodo sabía en qué otros entornos estaba clonado un proyecto, así que no se podía elegir dónde lanzar un hilo.

## Decision

- Un proyecto creado desde un repositorio recibe un id derivado de su remoto (`oplog.ProjectID`), igual que el repositorio. Dos entornos que añaden el mismo remoto crean la misma fila. Si el repositorio ya tiene proyecto, se reutiliza.
- Cada nodo publica dónde guarda cada repositorio en la tabla replicada `repository_locations` (repositorio, nodo, ruta). `repository_paths` sigue siendo la copia local que usa el nodo para lanzar hilos.
- Al arrancar, cada nodo publica las rutas que ya tenía.
- Al lanzar un hilo de un proyecto, solo se ofrecen los entornos que tienen su repositorio.

## Consequences

- La ruta de cada nodo es visible para los demás nodos de la malla.
- Los proyectos duplicados creados antes de este cambio no se fusionan solos: el huérfano, sin repositorios, se puede archivar.
