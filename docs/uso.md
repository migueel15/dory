# Uso

## Inicio

La pantalla inicial es un chat. Eliges proyecto y agente (proveedor, modelo, razonamiento y permisos) y escribes. Con **Modo plan** activado, la conversación crea una tarea y un hilo que la define con el agente; el plan lo acepta solo el usuario.

## Cambiar de proveedor en un hilo

Cuando termine el turno, abre el selector de modelos del hilo y elige un modelo en otro proveedor. Al enviar el siguiente mensaje, el proveedor recibe un extracto de la conversación y continúa en el mismo hilo y directorio de trabajo. En conversaciones largas, el extracto prioriza la petición inicial y los mensajes recientes. Vuelve a adjuntar cualquier archivo anterior que el nuevo proveedor necesite. Al elegir otro modelo, se muestra un separador pendiente. Si vuelves al modelo del último turno, el aviso desaparece. Al enviar con el modelo nuevo, el separador se guarda en el historial e indica si se transfirió contexto a otra sesión.

## Citar texto

Selecciona texto con un hilo abierto y elige **Citar** para añadirlo al mensaje, o **Responder** para escribir una respuesta a ese fragmento. Las citas se apilan encima del cuadro de mensaje. Puedes quitar una cita con su ×. Al enviar, se incluye cada fragmento con su respuesta y el texto que hayas escrito en el cuadro principal.

## Adjuntar imágenes

Pega una imagen o añádela con el botón **Adjuntar**. La miniatura queda junto a tu mensaje en el historial. Pulsa la imagen para ampliarla y pulsa Escape para cerrar la vista.

## Tareas

El kanban tiene estas columnas, que se calculan solas a partir de los hilos y de la PR de la rama:

| Columna | Cuándo |
|---|---|
| Inbox | Ideas capturadas, sin decidir |
| Ready | Lista para lanzar, con o sin plan |
| Working | Un hilo trabaja o la PR está en draft |
| Needs you | El hilo te espera o falló, el CI falla o piden cambios |
| In review | PR lista, con review pendiente |
| Ready to merge | PR aprobada o sin review pedida; fusionar es decisión tuya |
| Done | PR fusionada, o tarea marcada como hecha a mano sin PR |
| Cancelled | PR cerrada sin fusionar, o tarea cancelada |

A mano se mueven tareas entre Inbox, Ready y Cancelled. Con **Marcar como hecha** (menú rápido o menú de la tarea) una tarea pasa a Done cuando su trabajo fue directo a `main` y no tiene PR; si tiene una PR sin fusionar (abierta, en borrador o cerrada) se rechaza, y con una PR fusionada ya está en Done sola. La PR manda: si a una tarea hecha a mano se le asocia después una PR sin fusionar, vuelve al flujo normal. **Reabrir** devuelve las tareas hechas a mano o canceladas a Ready; no desarchiva sus hilos. Cada tarjeta muestra su rama, su PR y un único icono de estado: azul trabajando, naranja te necesita, amarillo CI en curso, ámbar review, verde lista, morado fusionada, rojo fallo y gris inactiva. El tooltip explica el estado.

Al abrir una carta ves su título, estado y descripción. El botón **Abrir PR** y la referencia **PR #…** llevan al detalle de la pull request en Dory. Los enlaces al repositorio y a la issue abren GitHub cuando están disponibles. Los hilos y las tareas de las que depende se abren desde sus nombres. En pantallas anchas, las referencias y dependencias quedan a la derecha.

En el detalle de una tarea puedes:

- Usar **Editar tarea** para cambiar el título, la descripción, el repositorio, la issue o la PR.
- Gestionar sus dependencias y usar las acciones al pie de la carta para cambiar el estado o archivarla.
- Pulsar **Definir plan** para elegir el entorno y el modelo en el panel bajo el botón. **Abrir hilo** inicia la definición con el agente.
- Usar **Revisar plan** para abrir y aceptar un plan pendiente. Un plan existente se abre plegado y su encabezado muestra el estado y la revisión.
- Pulsar **Implementar** con un plan aceptado. El mismo panel permite elegir el entorno y el modelo. **Abrir hilo** crea la rama y su worktree y lanza el agente.

Para implementar, las dependencias deben estar resueltas.

## Hilos

La bandeja reúne los hilos de todos los nodos, con filtros (activos, te necesitan, trabajando, archivados).

- **Chat:** muestra en directo el razonamiento, las herramientas y los planes del agente. Lo que escribas mientras trabaja se pone en cola; el botón de parar interrumpe el turno.
- **Hilos de otras máquinas:** se abren a través de su nodo; si ese nodo está apagado, el hilo no está disponible.
- **Acciones:** renombrar, fijar y archivar. Archivar borra el worktree.
- **Cambios:** ver el diff del worktree contra la rama base.
- **Servicios:** en Linux, la información del hilo muestra los puertos TCP de procesos marcados con el identificador del hilo y permite detenerlos cuando no comparten grupo con el agente ni con una shell. Los procesos que no conservan la marca no aparecen. Los enlaces HTTP se abren en el navegador externo cuando el hilo pertenece al nodo consultado. Los puertos de otros entornos se muestran sin enlace local.
- **Planificar esta conversación:** el botón del compositor solicita un plan al agente del mismo hilo sin enviar el borrador. Si está trabajando, la solicitud se encola. El agente reutiliza la tarea vinculada o crea y vincula una; solo el usuario acepta el plan.
- **Plan:** si el hilo tiene tarea, el panel lateral muestra su plan. Se actualiza en directo mientras el agente lo escribe y se abre solo cuando lo cambia. Se edita directamente sobre el texto con formato: «# » crea un título, «- » una lista y «[] » una casilla. Se guarda solo al dejar de escribir. Con **Seguir** activado, la vista va a cada cambio del agente antes de mostrarlo. Si el agente cambia el plan mientras escribes, sus cambios se combinan con los tuyos sin moverte el cursor. Si los dos tocáis la misma línea, se quedan las dos versiones, la suya citada debajo de la tuya, para que elijas.

## GitHub

Muestra las issues y PRs de los repositorios de tus proyectos.
- Una issue se convierte en tareas a mano o con un agente que la parte con el MCP.
- Una PR tiene vista de ficheros y diff, y se puede comentar y revisar.
- Un chat lateral responde preguntas sobre la PR en un hilo de revisión.

## Skills

Un único catálogo editable para todos los nodos. Cada skill indica en qué nodos y para qué agentes se instala. Cada nodo la escribe en `~/.claude/skills`, `~/.codex/skills`, `~/.config/opencode/skills` o `~/.gemini/config/skills` y publica su estado: sincronizada, pendiente, conflicto o error.

Dory trae sus propias **skills del sistema** (`dory-workflow` y `single-test-run`), que explican a los agentes cómo usar Dory. Llevan la etiqueta Sistema, se instalan en todos los nodos sin configurar nada y se actualizan con cada versión de Dory. No se pueden editar: se pueden desactivar para toda la malla o hacer un fork, que es una copia editable y desactiva la original. La original solo se puede volver a activar con el fork desactivado.

Dory nunca pisa una carpeta o un fichero que no escribió (por ejemplo, enlaces simbólicos a otro repositorio). Tampoco pisa una copia editada a mano: lo marca como conflicto.

## Entornos y uso

- **Entornos:** muestra este nodo y los emparejados, con sus agentes, su versión y si tienen sesión. Desde aquí se inicia sesión o se actualiza un agente en cualquier nodo conectado.
  Si un agente no está instalado en el nodo, **Instalar** ejecuta allí su instalador oficial. Claude Code y Codex usan su script (`install.sh` o, en Windows, `install.ps1`). OpenCode usa su script en Linux y macOS, y `npm install -g @opencode/cli@latest` en Windows, que necesita Node. Dory busca cada agente en el PATH y, si no está, en las carpetas donde lo dejan esos instaladores, así que no hace falta reiniciar el núcleo.
  Para iniciar sesión en Claude Code desde el móvil, elige el nodo, pulsa **Iniciar sesión** en Claude y abre el enlace que aparece. Si Claude muestra un código de autorización, vuelve a Dory, pégalo en **Código de autorización** y pulsa **Confirmar código**. El código se envía al proceso de Claude Code del nodo seleccionado. La tarjeta actualiza el estado cuando el proceso termina.
- **Uso:** muestra los límites de los proveedores, el historial de tokens y su equivalente a tarifa API. En **Desglose** puedes filtrar por periodo, entorno, proveedor e hilo. La tabla separa entrada, salida y caché y muestra qué porcentaje corresponde a la conversación, las instrucciones, el razonamiento y las herramientas visibles. Abre **Llamadas MCP** para ver qué herramientas intervinieron.
- Al seleccionar un hilo en **Desglose**, abre un turno para ver el modelo, la entrada, la caché, la salida y el razonamiento registrados en cada llamada. **Cambio de entrada** compara el contexto total con la llamada anterior del mismo turno cuando ambas son del agente principal y usan el mismo modelo. Una subida indica que se procesó más contexto y puede incluir varias fuentes.
- El desglose toma los contadores que informa el proveedor y estima su reparto a partir del contenido visible para Dory. El historial previo, los esquemas de herramientas y las instrucciones internas pueden quedar en **Contexto sin atribuir**. El coste equivalente usa las tarifas API de referencia de los modelos conocidos y la tarifa de escritura de caché de corta duración. La cobertura indica qué proporción de tokens tiene una tarifa aplicable.
- El desglose empieza con los turnos nuevos que registren contadores. Cada entorno guarda los suyos en su base local. Las pestañas históricas **Tokens** y **Coste** siguen leyendo los registros de los arneses.

## Antigravity

En **Entornos**, instala Antigravity en el nodo donde quieras usarlo. Pulsa **Iniciar sesión**, abre el enlace de Google y pega el código de autorización si se solicita. También puedes iniciar sesión en el nodo con `agy acp --login`.

Elige Antigravity al crear un hilo. Los modelos disponibles dependen de la cuenta y de la versión de la CLI. La conversación se reanuda en el mismo nodo tras reiniciar Dory. Las sesiones guardadas por la CLI aparecen en **Hilos**, donde puedes abrirlas e importarlas.

Con acceso restringido, Dory aprueba las solicitudes de edición dentro del worktree. Con acceso completo, aprueba las solicitudes de permiso del arnés. La página **Uso** incluye los tokens guardados en el nodo y las cuotas de la cuenta de Antigravity por grupo de modelos. Dory consulta los saldos con la sesión de `agy` cada 10 minutos y conserva la última lectura en el nodo. En Linux, accede a Secret Service con `secret-tool` si `agy` guarda allí la sesión.

## Avisos

La bandeja de avisos (icono de bandeja, junto a la campana) guarda cada aviso con su estado leído o sin leer y un contador. Es la misma en todos los dispositivos: leer un aviso en uno lo marca leído en todos. Los avisos leídos se borran a los 30 días; los no leídos se conservan.

Eventos que generan aviso: el agente terminó, el agente falló, un agente te necesita, plan listo para revisar, plan aceptado (tarea lista para implementar) y tarea lista para mergear (pasa a la columna *Ready to merge*, sin tener en cuenta el CI).

Como en una app de mensajería, cada aviso llega a todos tus dispositivos a la vez. Con el modo **Automático**:

1. En la interfaz que tiene el foco, el aviso sale dentro de la app.
2. En el resto (otra ventana, el navegador en segundo plano, el móvil con la app cerrada) llega como notificación del sistema.
3. El servidor envía también push a los navegadores suscritos.
4. Con todo cerrado y sin push, el aviso espera en la bandeja.

Si una interfaz se cierra bruscamente, la malla la da por cerrada a los 45 s.

En **Ajustes → Avisos** eliges para cada tipo de evento: Automático, Siempre push (también con foco), Solo en la app (nunca push) o Desactivado (no genera aviso). El ajuste es común a toda la malla.

La app de Android no usa Web Push ni servicios de Google. Igual que Gotify, mantiene un servicio en primer plano conectado al flujo de eventos de un nodo emparejado (prueba los nodos guardados por turnos) y muestra una notificación nativa por cada aviso nuevo sin leer, salvo mientras la propia app está en pantalla, y nada para los tipos en *Solo en la app* o *Desactivado*. El servicio deja fija la notificación «Dory conectado a la malla», que puedes silenciar desde los ajustes del canal. Arranca al abrir la app, al guardar nodos y al encender el móvil. Si el móvil no alcanza ningún nodo (fuera de la red local y sin Tailscale), reintenta cada vez más espaciado hasta que vuelve a tener conexión.

## Apariencia

En **Ajustes → Apariencia**, la escala de la interfaz va del 75% al 200%. También cambia con Ctrl + y Ctrl − (Cmd en macOS) y vuelve al 100% con Ctrl 0. Se guarda en cada navegador o app de escritorio.

## Atajos

En **Ajustes → Atajos** puedes consultar y cambiar las combinaciones, cancelar la captura con Escape o restaurar los valores originales. El ajuste de dictado anterior se conserva. Los atajos solo actúan con Dory enfocado y algunos dependen de la vista. El navegador o el sistema pueden interceptar combinaciones, incluido Ctrl+N; puedes elegir otra. En macOS se usa Cmd en lugar de Ctrl para los valores iniciales, salvo el dictado existente.

| Acción | Combinación inicial |
|---|---|
| Nuevo hilo | Ctrl+N |
| Buscar hilos | Ctrl+Mayús+F |
| Paleta de comandos | Ctrl+K |
| Modelo | Ctrl+Mayús+M |
| Modo plan o planificar conversación | Ctrl+Mayús+P |
| Rama | Ctrl+Mayús+B |
| Proyecto | Ctrl+Mayús+O |
| Navegación principal | Ctrl+Mayús+L |
| Lista de hilos | Ctrl+Mayús+H |
| Panel lateral | Ctrl+Mayús+J |
| Dictado | Ctrl+D |

La paleta también busca tareas, proyectos, entornos y PR e issues de los repositorios seguidos. Seleccionar una tarea abre su detalle sin cambiar su estado. La búsqueda de GitHub puede indicar resultados incompletos si alguna consulta falla.

Para evitar perder un borrador, Dory impide que los comandos de navegación abandonen su vista mientras haya texto, citas o adjuntos pendientes. La búsqueda dentro de Hilos abre la lista sin salir del hilo actual.
