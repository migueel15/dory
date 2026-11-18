# ADR-010: Contexto entre proveedores de un mismo hilo

- Status: accepted
- Date: 2026-11-17

## Context

Cada proveedor del runtime definido en el ADR-008 crea sus propios identificadores de sesión. El hilo de Dory puede durar más que cualquiera de sus sesiones.

## Decision

El proveedor y el modelo se eligen en el hilo existente. Al enviar un mensaje tras cambiar de proveedor, el nodo abre una sesión del proveedor elegido y añade un extracto de los mensajes visibles de usuario y asistente, con un máximo de 16 000 bytes para el historial. El extracto prioriza la petición inicial y los mensajes recientes e indica las omisiones. La nueva petición se conserva íntegra. Los turnos siguientes del mismo proveedor retoman la sesión nueva.

## Consequences

El hilo conserva su identidad, permisos y directorio de trabajo. Tras un traspaso, SQLite conserva la conversación completa de Dory aunque el archivo nativo de una sesión cubra solo un tramo. El contexto portátil transfiere texto visible. Los adjuntos previos y el estado interno de herramientas se quedan en su sesión de origen.
