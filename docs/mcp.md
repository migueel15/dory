# MCP

Cada nodo sirve un MCP en `http://127.0.0.1:7700/mcp`. Los hilos que lanza Dory lo reciben solos, con un token del nodo. Para otros clientes, crea un token con `dory token create --scopes write`.

Los agentes leen todo lo que necesitan del trabajo en curso, pero solo escriben lo suyo. Nunca cambian el estado de una tarea: la columna se deriva de los hilos y de la PR.

| Herramienta | Qué hace |
|---|---|
| `session_start` | Memorias globales, proyectos y, si encuentra el proyecto (por id, slug, remoto o ruta), sus repositorios, tareas abiertas y memorias |
| `list_projects` | Proyectos |
| `list_tasks` | Tareas con su columna, rama, PR, hilos y bloqueos; filtra por proyecto, issue o abiertas |
| `get_task` | Una tarea con sus planes y dependencias |
| `save_task` | Crea una tarea o cambia su título, descripción, issue o PR; `depends_on` añade dependencias |
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
