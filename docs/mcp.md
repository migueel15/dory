# MCP

Cada nodo sirve un MCP en `http://127.0.0.1:7700/mcp`. Los hilos que lanza Dory lo reciben solos, con un token del nodo. Para otros clientes, crea un token con `dory token create --scopes write`.

Los agentes leen todo lo que necesitan del trabajo en curso y escriben lo suyo. La columna del kanban se deriva de los hilos y de la PR; lo único que fijan es el `stage` de una tarea con `save_task`, con las mismas reglas que la interfaz.

| Herramienta | Qué hace |
|---|---|
| `session_start` | Memorias globales, proyectos y, si encuentra el proyecto (por id, slug, remoto o ruta), sus repositorios, tareas abiertas y memorias |
| `list_projects` | Proyectos |
| `list_tasks` | Tareas con su columna, rama, PR, hilos y bloqueos; filtra por proyecto, issue o abiertas |
| `get_task` | Una tarea con sus planes y dependencias |
| `save_task` | Crea una tarea o cambia su título, descripción, issue, PR o `stage` (`inbox`, `ready`, `done`, `cancelled`); `depends_on` añade dependencias. `done` solo se acepta sin PR o con la PR fusionada; con una PR sin fusionar devuelve error. `list_tasks` con `open` excluye las tareas hechas |
| `set_task_project` | Vincula una tarea a un proyecto o la desvincula (sin `project_id`); acepta id o slug y reajusta el repositorio |
| `ensure_thread_task` | En un hilo, reutiliza su tarea o crea y vincula una en una transacción; con `task_id` vincula el hilo a esa tarea existente; admite reintentos |
| `link_thread` | Vincula un hilo (por defecto el actual) a una tarea existente del mismo proyecto; si ya tiene otra, exige `replace` |
| `unlink_thread` | Desvincula un hilo (por defecto el actual) de su tarea |
| `list_task_threads` | Hilos de una tarea con su título, nodo, estado y rama |
| `add_task_dependency` | Marca que una tarea espera a otra; rechaza autodependencias y ciclos |
| `remove_task_dependency` | Quita una tarea de las bloqueantes de otra |
| `plan_read` | Lee el plan de una tarea: un documento Markdown con su número de revisión |
| `plan_write` | Escribe el plan entero para empezarlo; reescribir uno existente exige la `revision` leída y falla si el plan cambió desde entonces |
| `plan_edit` | Sustituye un fragmento exacto del plan, que debe aparecer una sola vez |
| `remember` | Guarda o actualiza una memoria global, de proyecto o de nodo, con tipo, título y labels |
| `recall` | Busca memorias por texto, proyecto, labels o tipo |
| `search` | Busca en tareas y memorias |
| `ask_user` | Pregunta al usuario; el hilo queda esperando su respuesta |

## Dispositivos Android

Con el ajuste **Los agentes pueden usar dispositivos** activado en el nodo ([Dispositivos Android](dispositivos.md)), el MCP añade estas herramientas. Flujo: `device_ui` para leer la pantalla, actuar y comprobar de nuevo.

| Herramienta | Qué hace |
|---|---|
| `device_list` | Dispositivos conectados y emuladores (AVD) disponibles |
| `device_open` / `device_close` | Muestra un dispositivo (o arranca un AVD) en el panel del hilo, en el lateral o flotante; lo cierra o apaga un emulador |
| `device_screenshot` | Captura PNG reducida a 900 px como imagen |
| `device_ui` | Elementos de la pantalla con texto, id, descripción, límites y centro |
| `device_tap` | Toca un elemento de la última lectura (índice, texto o id) o unas coordenadas |
| `device_swipe` | Desliza entre dos puntos |
| `device_type` | Escribe en el campo enfocado |
| `device_key` | Pulsa atrás, inicio, recientes, enter… |
| `device_launch` | Abre una app por paquete |
| `device_open_url` | Abre un enlace o *deep link* |
| `device_apps` | Lista las apps instaladas |
