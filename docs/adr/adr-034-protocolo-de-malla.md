# ADR-034: Protocolo versionado entre nodos de la malla

- Status: accepted
- Date: 2026-11-24
- Extends: ADR-009

## Context

Dos nodos con versiones distintas pueden recibir operaciones con entidades o columnas que uno de ellos aún desconoce. También pueden cambiar las rutas o los contratos de sincronización.

## Decision

- Las rutas entre nodos incluyen la versión del protocolo. Un nodo consulta `/peer/v2/info` antes de intercambiar operaciones y registra y muestra los protocolos incompatibles. La respuesta incluye el hash del registro de esquema replicado.
- Dentro de un protocolo compatible, cada nodo conserva y reenvía todas las operaciones recibidas. Materializa las entidades y columnas de su esquema. Las migraciones de esquemas posteriores recuperan del registro los datos que ya llegaron.
- Los intercambios entre nodos reutilizan el transporte TLS por par y dirección. Las llamadas JSON internas comparten tipos con los handlers de la API.

## Consequences

- El paso de `/peer/v1` a `/peer/v2` requiere actualizar ambos nodos para volver a sincronizar. Se conserva el emparejamiento.
- La conexión y el emparejamiento móvil siguen el protocolo `/device/v1`.
