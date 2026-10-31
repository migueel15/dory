# ADR-002: Conexión directa con las instancias de T3 Code

- Status: accepted
- Date: 2026-10-31

## Context

Dory debe mostrar y manejar las instancias de T3 Code de infra, Arch Linux y Arch Laptop. Infra y Arch Linux comparten la red de casa. Infra no tiene Tailscale, así que hoy no alcanza al portátil, aunque el portátil sí alcanza a Dory. T3 Code expone en cada instancia una ruta pública de descubrimiento (`/.well-known/t3/environment`), tokens con permisos (`t3 auth session issue`) y una API interna por HTTP y WebSocket sin contrato estable.

## Decision

- El servidor de Dory se conecta directamente a cada instancia. Infra y Arch Linux se conectan por la red local. No se instala ningún agente de Dory en las máquinas.
- El portátil queda fuera por ahora; la forma de alcanzarlo es una decisión futura.
- Cada instancia tiene su propio token. Los tokens empiezan con `orchestration:read`; `orchestration:operate` se añade cuando llegue la primera función que escriba en T3 Code.
- Los tokens son secretos de despliegue y no se guardan en claro en la base de datos.
- T3 Code es la fuente de verdad de hilos y ejecuciones. Dory guarda instancias y vínculos con items, sin copiar conversaciones.
- Dory comprueba `orchestrationProtocolVersion` y, si no la admite, marca la instancia como incompatible en lugar de fallar.
- Una instancia inaccesible se muestra como desconectada y no afecta a las demás.

- Las escrituras usan `orchestration.dispatchCommand` por WebSocket (`/ws` con `wsTicket`), con una conexión por llamada. Dory solo actúa sobre hilos que ya reporta una instancia y deja registro en `events`. Las aprobaciones se limitan a aceptar o rechazar una sola petición.

## Consequences

- Una actualización de T3 Code puede romper la integración hasta adaptar el conector.
- Dory solo ve las máquinas alcanzables desde infra; Arch Linux aparece como desconectada mientras está apagada.
- Añadir una instancia exige emitir un token y configurarlo en el despliegue.
