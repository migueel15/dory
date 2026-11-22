# ADR-027: Registro local de consumo por turno

- Status: accepted
- Date: 2026-11-22

## Context

Los arneses informan del consumo por llamada o guardan los contadores en sus registros locales. Sus formatos y el detalle de contexto que exponen difieren. Para saber cuánto pesa cada herramienta, Dory necesita relacionar esos contadores con lo que observa durante el turno.

## Decision

El nodo que ejecuta el hilo guarda los contadores del proveedor por llamada y por turno en su SQLite local. Guarda también un reparto estimado por categoría y herramienta a partir del tamaño de los mensajes y resultados observados. Las cifras desconocidas se mantienen como contexto o salida sin atribuir.

## Consequences

El seguimiento no incorpora mensajes ni datos de telemetría a las peticiones al modelo. El registro de consumo guarda cifras, identificadores y categorías, sin copiar el texto de mensajes ni resultados. El desglose empieza con los turnos que Dory registra y el histórico de los arneses se consulta por separado. La caché es un modo de procesamiento de la entrada que puede corresponder a varias categorías.
