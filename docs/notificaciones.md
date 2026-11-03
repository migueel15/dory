# Notificaciones

La campana muestra avisos sin leer de todos los proyectos. Desde ella se abre el
historial paginado, el destino afectado y los ajustes. Marcar leído se sincroniza
con otros dispositivos mediante sondeo cada tres a cinco segundos.

Hay cuatro eventos: item listo para implementar, item bloqueado, hilo finalizado
e hilo que requiere atención. Cada uno permite desactivar el evento completo o
elegir campana/lista, toast y push. Los ajustes pertenecen al usuario y se
sincronizan entre dispositivos. Solo afectan a avisos futuros y conservan los
anteriores. Los toast se limitan a avisos nuevos mientras la página está visible;
abrir o reconectar no reproduce el historial.

Para Android, abrir o instalar la PWA sobre HTTPS y elegir «Activar push en este
dispositivo» en los ajustes de la campana. Conceder el permiso del navegador.
El servidor conserva la suscripción y entrega push aunque la aplicación esté
cerrada. «Desactivar push en este dispositivo» elimina la suscripción del servidor
y del navegador. Sin soporte o permiso siguen disponibles los canales dentro de
Dory. Las claves VAPID se crean al primer arranque y no necesitan configuración
manual. El servidor necesita salida HTTPS hacia el servicio push del navegador.

El historial se genera en el servidor. Aplicar la migración 00009 con
`dory migrate up` antes de iniciar el nuevo binario. Los avisos de items se crean
en la transacción de su cambio. Los hilos T3 se observan cada diez segundos:
el primer snapshot establece una referencia sin importar avisos antiguos.
Desconexiones y desapariciones no equivalen a finalizaciones.

## API

Todas las rutas requieren autenticación. Las escrituras requieren permiso write.

- `GET /api/notifications?before=<id>`: hasta 100 avisos anteriores al cursor
  (sin cursor, los últimos), contador global `unread` y `server_time`.
  Cada aviso incluye id, title, url, kinds, bell, toast, read y created_at.
- `POST /api/notifications/{id}/read`: marcar leído un aviso propio.
- `GET /api/notifications/preferences`: settings y public_key VAPID.
- `PUT /api/notifications/preferences`: reemplazar settings, un objeto cuyas
  claves son item_ready, item_blocked, thread_finished y thread_attention.
  Cada entrada acepta enabled, bell, toast y push booleanos; los omitidos
  valen true. Push solo se entrega a dispositivos activados.
- `POST /api/notifications/subscription`: registrar el JSON PushSubscription.
- `DELETE /api/notifications/subscription`: eliminar el endpoint indicado
  en el cuerpo JSON para el usuario autenticado.

## Validación manual

Seguir los siete escenarios del item 5ac1cf9d-b82c-4ee2-a427-32d6c0a1b06f:
transiciones de otros proyectos vía REST/MCP, finalización normal y fallo T3,
ausencia de duplicados y falsos finales al desconectar, historial con clientes
cerrados, lectura y preferencias entre dispositivos, push Android cerrado y
navegación, y presentación móvil en ambos temas. Incluir una segunda ejecución
del mismo hilo y dos peticiones distintas mientras está esperando respuesta.
