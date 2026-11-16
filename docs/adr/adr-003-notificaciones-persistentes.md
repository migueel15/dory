# ADR-003: Notificaciones persistentes por usuario

- Status: superseded by ADR-009
- Date: 2026-11-03

## Contexto

Los avisos deben existir con los clientes cerrados y conservar su estado entre
dispositivos. La auditoría y el contador de atención no representan avisos sin leer.

## Decisión

PostgreSQL conserva avisos, preferencias por usuario, suscripciones por dispositivo
y una cola push. Un trigger crea los avisos de transiciones ready/blocked dentro de
la transacción del item, tanto desde REST como desde MCP.

El monitor del servidor observa todos los hilos T3, incluidos settled y subagentes.
Guarda únicamente su estado de notificación, identificador de ejecución y petición,
sin conversaciones. El primer snapshot establece la referencia inicial; los
siguientes detectan finalizaciones y atención. No se interpreta la ausencia como
finalización. Fin y fallo de una misma observación se agrupan.

Los canales se fijan al generar cada aviso. El historial y los estados leídos son
por usuario y globales entre proyectos. Web Push usa webpush-go y claves VAPID
generadas una vez y conservadas en PostgreSQL. Suscripciones caducadas se eliminan;
la entrega no condiciona el historial.

## Consecuencias

Se aplica la migración 00009 antes de arrancar el nuevo binario. La clave privada
VAPID forma parte de los datos sensibles incluidos en los backups de PostgreSQL.
Los sondeos pueden omitir estados intermedios entre dos observaciones de T3;
no sustituyen un registro de eventos de T3. La primera conexión de una instancia
no importa sus finalizaciones históricas como avisos nuevos.
