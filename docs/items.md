# Acciones rápidas de items

El clic derecho sobre una tarjeta kanban o una fila de Inbox abre el menú de acciones del item. Permite abrir el detalle, editar, abrir la vista «Definir» de los items inbox, triaged (en definición) o ready, y enviar los ready a implementar mediante el diálogo de lanzamiento existente.

«Mover a estado…» muestra los estados disponibles; bloquear requiere un motivo. Archivar pide confirmación. Si existen, se incluyen el último hilo conocido y la última PR de las referencias.

Las flechas arriba/abajo y Home/End recorren las acciones. Enter activa la seleccionada; la flecha derecha abre el submenú de estados y la izquierda vuelve. Escape, Tab, clic fuera o scroll cierran el menú.

El doble clic y el arrastre del kanban conservan sus manejadores. En móvil se mantiene el botón ✎ y no se añade un gesto de pulsación larga. Fuera de las tarjetas y filas se conserva el menú del navegador.

# Archivado de items e hilos

Al archivar un item desde la web o MCP (`archive_item`), Dory intenta archivar también todos sus hilos asociados en `item_threads` cuyos enlaces no estén archivados. Cada hilo se alcanza mediante su propia instancia T3. El intento ocurre después de confirmar el archivado en la base de datos; un fallo de T3 se registra en el log y no impide archivar el item ni intentar los demás hilos.

Archivar en T3 oculta el hilo y puede detener trabajo que esté en curso. Se usa `thread.archive`, sin asentarlo antes con `thread.settle`. Recuperar el item no recupera sus hilos, y moverlo a `done` no los archiva. Este comportamiento corresponde al archivado individual del item, no al archivado en cascada de un proyecto.

# Vista «Definir»

`/items/{id}/define` convierte una nota en un ticket listo. En escritorio muestra dos columnas; en móvil, las pestañas Chat y Plan.

- **Chat:** la primera vez se elige máquina, proyecto de T3, modelo y razonamiento, con indicaciones opcionales. Después cada mensaje va al mismo hilo de definición (Ctrl+Enter envía). Las preguntas del agente aparecen como tarjetas con opciones y respuesta libre. Si el hilo no es accesible, se puede empezar uno nuevo.
- **Plan:** la última versión, sección a sección. Las secciones que cambiaron respecto a la versión anterior se resaltan. «Comentar» cita la sección en el chat. «Editar» guarda una versión escrita por el usuario.
- **Aceptar plan** solo se activa sin preguntas abiertas. Copia el plan a la descripción y deja el item en ready. Desde ready, «Enviar a implementar» abre el diálogo de lanzamiento.
- **Definir todo**, en el Inbox, abre un hilo de definición por cada nota con la misma máquina y modelo.
