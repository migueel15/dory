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
| Done | PR fusionada |
| Cancelled | PR cerrada sin fusionar, o tarea cancelada |

A mano solo se mueven tareas entre Inbox, Ready y Cancelled. Cada tarjeta muestra su rama, su PR y un único icono de estado: azul trabajando, naranja te necesita, amarillo CI en curso, ámbar review, verde lista, morado fusionada, rojo fallo y gris inactiva. El tooltip explica el estado.

En el detalle de una tarea puedes:
- Editarla y ligarla a un repositorio, una issue o una PR.
- Gestionar sus dependencias.
- Escribir, editar o aceptar su plan, un documento Markdown que también editan los agentes.
- **Implementar**: crea la rama (`feat/<issue>-<título>`) y su worktree y lanza el agente.
- **Definir con un agente**: abre un hilo que escribe el plan con `plan_write` y `plan_edit`.

Una tarea con dependencias sin terminar no se puede lanzar.

## Hilos

La bandeja reúne los hilos de todos los nodos, con filtros (activos, te necesitan, trabajando, archivados).

- **Chat:** muestra en directo el razonamiento, las herramientas y los planes del agente. Lo que escribas mientras trabaja se pone en cola; el botón de parar interrumpe el turno.
- **Hilos de otras máquinas:** se abren a través de su nodo; si ese nodo está apagado, el hilo no está disponible.
- **Acciones:** renombrar, fijar y archivar. Archivar borra el worktree.
- **Cambios:** ver el diff del worktree contra la rama base.
- **Plan:** si el hilo tiene tarea, el panel lateral muestra su plan. Se actualiza en directo mientras el agente lo escribe y se abre solo cuando lo cambia. Se edita directamente sobre el texto con formato: «# » crea un título, «- » una lista y «[] » una casilla. Se guarda solo al dejar de escribir. Con **Seguir** activado, la vista va a cada cambio del agente antes de mostrarlo. Si el plan cambia mientras escribes, Dory avisa y no pisa ninguna de las dos ediciones.

## GitHub

Muestra las issues y PRs de los repositorios de tus proyectos.
- Una issue se convierte en tareas a mano o con un agente que la parte con el MCP.
- Una PR tiene vista de ficheros y diff, y se puede comentar y revisar.
- Un chat lateral responde preguntas sobre la PR en un hilo de revisión.

## Skills

Un único catálogo editable para todos los nodos. Cada skill indica en qué nodos y para qué agentes se instala. Cada nodo la escribe en `~/.claude/skills`, `~/.codex/skills`, `~/.config/opencode/skills` o `~/.gemini/config/skills` y publica su estado: sincronizada, pendiente, conflicto o error.

Dory nunca pisa una carpeta o un fichero que no escribió (por ejemplo, enlaces simbólicos a otro repositorio). Tampoco pisa una copia editada a mano: lo marca como conflicto.

## Entornos y uso

- **Entornos:** muestra este nodo y los emparejados, con sus agentes, su versión y si tienen sesión. Desde aquí se inicia sesión o se actualiza un agente en cualquier nodo conectado.
  Si un agente no está instalado en el nodo, **Instalar** ejecuta allí su instalador oficial. Claude Code y Codex usan su script (`install.sh` o, en Windows, `install.ps1`). OpenCode usa su script en Linux y macOS, y `npm install -g @opencode/cli@latest` en Windows, que necesita Node. Dory busca cada agente en el PATH y, si no está, en las carpetas donde lo dejan esos instaladores, así que no hace falta reiniciar el núcleo.
  Para iniciar sesión en Claude Code desde el móvil, elige el nodo, pulsa **Iniciar sesión** en Claude y abre el enlace que aparece. Si Claude muestra un código de autorización, vuelve a Dory, pégalo en **Código de autorización** y pulsa **Confirmar código**. El código se envía al proceso de Claude Code del nodo seleccionado. La tarjeta actualiza el estado cuando el proceso termina.
- **Uso:** muestra lo que queda en cada ventana (5 horas y semanal) según lo último que informó cada proveedor.

## Antigravity

En **Entornos**, instala Antigravity en el nodo donde quieras usarlo. Pulsa **Iniciar sesión**, abre el enlace de Google y pega el código de autorización si se solicita. También puedes iniciar sesión en el nodo con `agy acp --login`.

Elige Antigravity al crear un hilo. Los modelos disponibles dependen de la cuenta y de la versión de la CLI. La conversación se reanuda en el mismo nodo tras reiniciar Dory. Las sesiones guardadas por la CLI aparecen en **Hilos**, donde puedes abrirlas e importarlas.

Con acceso restringido, Dory aprueba las solicitudes de edición dentro del worktree. Con acceso completo, aprueba las solicitudes de permiso del arnés. La página **Uso** incluye los tokens guardados en el nodo y las cuotas de la cuenta de Antigravity por grupo de modelos. Dory consulta los saldos con la sesión de `agy` cada 10 minutos y conserva la última lectura en el nodo. En Linux, accede a Secret Service con `secret-tool` si `agy` guarda allí la sesión.

## Apariencia

En **Ajustes → Apariencia**, la escala de la interfaz va del 75% al 200%. También cambia con Ctrl + y Ctrl − (Cmd en macOS) y vuelve al 100% con Ctrl 0. Se guarda en cada navegador o app de escritorio.
