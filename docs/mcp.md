# MCP

Cada nodo sirve un MCP en `http://127.0.0.1:7700/mcp`. Los hilos que lanza Dory lo reciben solos, con un token del nodo. Para otros clientes, crea un token con `dory token create --scopes write`.

Los agentes leen todo lo que necesitan del trabajo en curso, pero solo escriben lo suyo. Nunca cambian el estado de una tarea: la columna se deriva de los hilos y de la PR.

| Herramienta | Qué hace |
|---|---|
| `session_start` | Memorias globales, proyectos y, si encuentra el proyecto (por id, slug, remoto o ruta), sus repositorios, tareas abiertas y memorias |
| `list_projects` | Proyectos |
| `list_tasks` | Tareas con su columna, rama, PR, hilos y bloqueos; filtra por proyecto, issue o abiertas |
| `get_task` | Una tarea con sus planes y dependencias |
| `save_task` | Crea una tarea o cambia su título, descripción, issue o PR; `depends_on` marca dependencias |
| `plan_read` | Lee el plan de una tarea: un documento Markdown con su número de revisión |
| `plan_write` | Escribe el plan entero para empezarlo; reescribir uno existente exige la `revision` leída y falla si el plan cambió desde entonces |
| `plan_edit` | Sustituye un fragmento exacto del plan, que debe aparecer una sola vez |
| `remember` | Guarda o actualiza una memoria global, de proyecto o de nodo, con tipo, título y labels |
| `recall` | Busca memorias por texto, proyecto, labels o tipo |
| `search` | Busca en tareas y memorias |
| `ask_user` | Pregunta al usuario; el hilo queda esperando su respuesta |
