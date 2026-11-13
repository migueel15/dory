# Hilos

La sección **Hilos** (`/threads`) es la bandeja única de los hilos de T3 Code de todas las instancias disponibles. Dory lee del monitor (`GET /api/t3/threads`) todos los hilos de cada instancia: activos, asentados, archivados y subagentes. T3 Code sigue siendo la fuente de verdad (ADR-002).

## Lista

- **Filtros** (en la URL como `?f=`): Activos (por defecto), Te necesitan, Trabajando, Con tarea, Lanzados (hilos que Dory abrió para un item o una issue; sustituye a la antigua página Trabajos, y `/jobs` redirige aquí), Pospuestos, Asentados y Archivados. Un filtro sin hilos se oculta.
- **Búsqueda:** busca en todos los hilos menos en los archivados, sea cual sea el filtro. También acepta un enlace de T3 o un id.
- **Fijados** arriba, en el orden que asigna T3, y **Recientes** debajo.
- **Subagentes** plegados bajo su hilo padre. La fila del padre indica cuántos están activos.
- Un punto azul marca los hilos con actividad desde la última visita. Abrir un hilo lo marca como leído.
- Al pasar el ratón por un hilo se precarga su conversación.

## Acciones

Con clic derecho o el botón «⋯» de cada fila de la lista, y haciendo clic en el título del hilo abierto:

| Acción | Comando de T3 |
|---|---|
| Renombrar (en la lista, también con doble clic) | `thread.metadata.update` |
| Fijar arriba / Desfijar | `thread.pin` / `thread.unpin` |
| Marcar como leído / no leído | `thread.visit` / `thread.mark-unread` (solo con algún turno terminado) |
| Posponer 1 h, 3 h, mañana o el lunes a las 9:00 / Despertar ahora | `thread.snooze` / `thread.unsnooze` |
| Asentar / Reactivar | `thread.settle` / `thread.unsettle` |
| Bifurcar desde el último turno (abre el hilo nuevo) | `thread.fork` |
| Parar el hilo | `thread.stop` |
| Archivar / Desarchivar | `thread.archive` / `thread.unarchive` |
| Eliminar (pide confirmación) | `thread.delete` |

Todas pasan por `POST /api/t3/threads/{id}/actions` con `{action, title?, until?, run_id?}`. La lista se actualiza al momento y se corrige si T3 rechaza la acción.

La tarjeta de detalles del hilo (botón ⓘ) no se cierra al usar el chat ni al hacer clic fuera: solo con su botón. Cada hilo recuerda si la tiene abierta (en este navegador), y al ir a un subagente o al padre desde ella sigue abierta. Con el panel lateral de cambios abierto (por ejemplo, desde la fila «Cambios»), la tarjeta se oculta; su botón la muestra entonces como ventana emergente, que se cierra al hacer clic fuera o con Esc.

La sección **Linaje** de la tarjeta ofrece «Volver a …» en un subagente, lista los subagentes que trabajan o esperan respuesta con el logo de su modelo y su estado, y debajo despliega «Agentes previos»: los ya terminados, con su estado y hace cuánto están inactivos. El botón ⓘ indica cuántos subagentes están trabajando. La cabecera de un subagente muestra la ruta «padre › subagente» con el mismo tamaño: el padre en gris (enlaza a él) y el hilo abierto resaltado. Junto al título, el botón con el logo de T3 y una flecha abre el hilo en T3 Code.

Un subagente lo maneja su padre y no acepta mensajes: en su hilo, en lugar del compositor aparece una barra, como en T3 Code. Muestra el logo y el nombre del modelo, el esfuerzo, el estado con su duración («Trabajando · 45s», «Completado en 1m 59s»), «Se ejecuta por su cuenta» y «Abrir padre». Los subagentes nativos de Claude no tienen ejecuciones propias en T3, así que su duración va de la creación al último cambio.

## Rendimiento

- Todas las llamadas RPC a una instancia comparten un WebSocket persistente.
- La vista de cada hilo se guarda en una caché del servidor. Se invalida con cada escritura de Dory, con cada ráfaga del stream del hilo y cuando el monitor ve que el hilo cambió. Un hilo que está trabajando siempre se lee de T3.
- El diff de un worktree local se consulta cada 3 s mientras se ve; el diff que calcula T3 se actualiza con los eventos del hilo.
- La SPA se sirve con gzip y con caché `immutable` para `/assets/*`. Las páginas pesadas se cargan bajo demanda y se precargan en segundo plano.
