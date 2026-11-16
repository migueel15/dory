# Uso

## Inicio

La pantalla inicial es un chat. Eliges proyecto y agente (proveedor, modelo, razonamiento y permisos) y escribes. Con **Modo plan** activado, la conversación crea una tarea y un hilo que la define con el agente; el plan lo acepta solo el usuario.

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
- Escribir o aceptar su plan.
- **Implementar**: crea la rama (`feat/<issue>-<título>`) y su worktree y lanza el agente.
- **Definir con un agente**: abre un hilo que propone el plan.

Una tarea con dependencias sin terminar no se puede lanzar.

## Hilos

La bandeja reúne los hilos de todos los nodos, con filtros (activos, te necesitan, trabajando, archivados).

- **Chat:** muestra en directo el razonamiento, las herramientas y los planes del agente. Lo que escribas mientras trabaja se pone en cola; el botón de parar interrumpe el turno.
- **Hilos de otras máquinas:** se abren a través de su nodo; si ese nodo está apagado, el hilo no está disponible.
- **Acciones:** renombrar, fijar y archivar. Archivar borra el worktree.
- **Cambios:** ver el diff del worktree contra la rama base.

## GitHub

Muestra las issues y PRs de los repositorios de tus proyectos.
- Una issue se convierte en tareas a mano o con un agente que la parte con el MCP.
- Una PR tiene vista de ficheros y diff, y se puede comentar y revisar.
- Un chat lateral responde preguntas sobre la PR en un hilo de revisión.

## Skills

Un único catálogo editable para todos los nodos. Cada skill indica en qué nodos y para qué agentes se instala. Cada nodo la escribe en `~/.claude/skills`, `~/.codex/skills` o `~/.config/opencode/skills` y publica su estado: sincronizada, pendiente, conflicto o error.

Dory nunca pisa una carpeta o un fichero que no escribió (por ejemplo, enlaces simbólicos a otro repositorio). Tampoco pisa una copia editada a mano: lo marca como conflicto.

## Entornos y uso

- **Entornos:** muestra este nodo y los emparejados, con sus agentes, su versión y si tienen sesión. Desde aquí se inicia sesión o se actualiza un agente en cualquier nodo conectado.
- **Uso:** muestra lo que queda en cada ventana (5 horas y semanal) según lo último que informó cada proveedor.
