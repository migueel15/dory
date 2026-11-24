# ADR-037: Contrato de arneses basado en capacidades

- Status: accepted
- Date: 2026-11-24
- Sustituye la división entre Runner e Interactive del ADR-015.

## Context

Dory ejecuta arneses con protocolos distintos. Sus turnos comparten un ciclo de vida y producen mensajes que se guardan y se publican en la interfaz. Los adaptadores ofrecen funciones distintas para gestionar cuentas, leer sesiones o consultar el consumo.

## Decision

Cada arnés implementa `harness.Driver` y devuelve una `Session` que emite eventos tipados y recibe las respuestas a peticiones. El registro descubre las extensiones, ordena los arneses por sus metadatos y conserva las cuentas y su activación en el estado local del nodo. El nodo inyecta su pool de procesos y trabajos de administración.

## Consequences

El Manager reconstruye los mensajes que usa la interfaz a partir de los eventos de la sesión. Los adaptadores actuales traducen sus instantáneas con `harness.Upgrade` hasta que emitan fragmentos nativos. La interfaz consulta las capacidades del estado publicado para habilitar acciones.
