# ADR-041: Compactación del registro de operaciones con instantáneas

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-007

## Context

El registro de operaciones crecía sin límite. Un nodo nuevo tenía que recibir todo el historial para reconstruir las entidades.

## Decision

- Cada nodo anota el vector que obtuvo en su último intercambio con cada par emparejado (`peer_vectors`).
- `Compact` convierte en instantáneas por fila (`row_snapshots`: columnas más recientes e historial de fusión) los prefijos estables en transacciones acotadas. Un prefijo es estable si todos los pares activos que sincronizaron en los últimos 30 días lo han recibido. Un par activo sin ningún acuse cuenta con vector vacío hasta su primer acuse. Un nodo sin pares registrados puede compactar.
- La compactación se detiene antes de la primera operación de una entidad desconocida por el esquema local, para no hacer que un receptor pierda esa operación.
- Un par que pide un prefijo ya compactado recibe `ErrSnapshotRequired` y arranca desde las instantáneas.

## Consequences

- El registro queda acotado por el vector de los pares activos. Un par ausente más de 30 días vuelve a sincronizar desde instantáneas.
- Cierra lo que ADR-007 dejaba pendiente sobre la compactación.
