# MCP

Cada nodo sirve un MCP en `http://127.0.0.1:7700/mcp`. Los hilos gestionados por Dory reciben la conexión automáticamente con una credencial vinculada a su identificador. Para otros clientes, crea un token con `dory token create --scopes write`.

Los agentes leen todo lo que necesitan del trabajo en curso y escriben lo suyo. La columna del kanban se deriva de los hilos y de la PR; lo único que fijan es el `stage` de una tarea con `save_task`, con las mismas reglas que la interfaz.

## Sesiones gestionadas

Al enviar un turno, Dory comprueba el catálogo MCP con la credencial de ese hilo y añade instrucciones sobre las herramientas al mensaje que recibe el arnés. Si falla la comprobación, el envío devuelve un error con el proveedor y el motivo. El catálogo del nodo refleja sus ajustes, por ejemplo la disponibilidad de las herramientas de dispositivos.

| Arnés | Registro de `dory` | Sesión |
|---|---|---|
| Codex | Configuración HTTP de `app-server` y bearer en el entorno | `thread/start` o `thread/resume` |
| Claude Code | `--mcp-config` con un encabezado HTTP que referencia una variable de entorno | Nueva sesión o `--resume` |
| OpenCode v2 | Registro en memoria por la API del servidor del hilo. Dory espera el estado conectado | El servidor abre o retoma la sesión por su API |
| Antigravity | `mcpServers` en `session/new` o `session/load`. Usa HTTP cuando el arnés lo anuncia y el puente stdio de Dory en los demás casos | ACP conserva la sesión tras iniciar el turno |

Con el mismo arnés, cambiar el modelo conserva el identificador de sesión. Al cambiar de arnés, el siguiente turno abre una sesión en el destino y lleva la solicitud nueva junto con un extracto del texto visible del hilo. Dory prioriza la petición anterior más reciente y la inicial dentro de 16 KB. Si el destino conservaba un proceso de una visita anterior, Dory lo cierra para abrir la sesión nueva y se detienen los servicios que dependan de ese proceso.

Para probar el flujo desde escritorio, prepara perfiles de cliente con acceso al modelo y sin MCP manual. En un hilo nuevo, pide al agente usar `session_start` y abrir la URL de un mockup local con `browser_open`. Comprueba la pestaña activa con `browser_tabs` y en el panel del mismo hilo. Envía otro turno después de reanudar, cambia de Codex a Claude y pide que continúe la tarea y consulte el catálogo de nuevo. La comprobación local del transporte y los tests de traspaso cubren partes de este recorrido. La prueba completa requiere cuentas de proveedor y escritorio.

La comprobación de transporte con perfiles limpios y un MCP local de prueba alcanzó el catálogo en Codex 0.171.0, Claude Code 2.1.330 y Antigravity 1.5.14 mediante stdio. OpenCode 2.3.2 informó del estado conectado desde el servidor que inicia Dory. Queda por comprobar la reanudación y el cambio de proveedor con modelos autenticados, incluido el panel de escritorio.

## Catálogo

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
| `device_launch` | Abre una app por su nombre («WhatsApp», «Ajustes») o por paquete; si hay varias candidatas, las lista |
| `device_open_url` | Abre un enlace o *deep link* |
| `device_apps` | Apps con icono de lanzador (paquete, actividad y nombre legible), con filtro; con `all`, todos los paquetes |
| `device_scroll` | Desplaza el contenido arriba, abajo, izquierda o derecha, sin coordenadas |
| `device_long_press` | Mantiene pulsado un elemento o un punto |
| `device_tap_text` | Lee la pantalla y toca el elemento con ese texto en una sola llamada |
| `device_find` / `device_wait_for` | Comprueba, o espera hasta 30 s, a que un texto aparezca en pantalla, sin imágenes |
| `device_state` | Estado en texto: app en primer plano, pantalla, batería, modo oscuro, orientación, Wi-Fi y datos |
| `device_notifications` | Notificaciones activas con app, título y texto |
| `device_screen` | Enciende o apaga la pantalla, o pasa un bloqueo sin PIN |
| `device_setting` | Cambia modo oscuro, tamaño de texto, orientación, Wi-Fi, datos, permisos o ubicación (emulador) y devuelve el valor leído |

## Navegador del hilo

Cada hilo tiene un navegador propio en su nodo. Las herramientas solo funcionan dentro de un hilo y se ejecutan en el nodo que lo ejecuta, aunque nadie tenga el hilo abierto y aunque el nodo no tenga interfaz.

| Herramienta | Qué hace |
|---|---|
| `browser_open` | Abre una URL en la pestaña activa o en una nueva (`new_tab`). Si el hilo está abierto en Dory Desktop, despliega el panel lateral y selecciona **Navegador** |
| `browser_tabs` | Lista las pestañas y cuál está activa |
| `browser_activate`, `browser_close` | Trae al frente o cierra una pestaña |
| `browser_navigate` | Va a una URL, o `back`, `forward` o `reload` |
| `browser_screenshot` | Devuelve una imagen PNG de la vista o de la página entera (`full_page`) |
| `browser_read` | Devuelve URL, título, texto visible y los elementos interactivos con las coordenadas de su centro |
| `browser_console` | Devuelve los últimos mensajes de consola y excepciones |
| `browser_click`, `browser_type`, `browser_key`, `browser_scroll` | Actúan por coordenadas o por selector CSS |
| `browser_resize` | Cambia el tamaño de la vista de una pestaña que corre en el nodo |

Una pestaña corre en el Chromium sin interfaz del nodo, o en una vista nativa de la app de escritorio mientras el usuario la tiene en la pestaña **Navegador** del panel lateral. Mientras el usuario ha tomado el control, las acciones devuelven un error y la lectura y las capturas siguen funcionando.
