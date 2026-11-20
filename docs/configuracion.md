# Configuración

Dory lee `~/.config/dory/config.toml` (o el fichero de `DORY_CONFIG`). Cada clave se puede sobrescribir con su variable de entorno. Ver `config.example.toml`.

| Clave | Variable | Por defecto | Qué es |
|---|---|---|---|
| `listen` | `DORY_LISTEN` | `127.0.0.1:7700` | Dirección de la API, la interfaz y el MCP |
| `data_dir` | `DORY_DATA_DIR` | `~/.dory/data` | Base SQLite, clave del nodo, adjuntos, worktrees y manifiestos |
| `node_name` | `DORY_NODE_NAME` | nombre del equipo | Nombre del nodo en la malla |
| `mesh_port` | `DORY_MESH_PORT` | `7710` | Puerto entre nodos; `0` apaga la malla |
| `cookie_secure` | `DORY_COOKIE_SECURE` | `false` | Cookie solo por HTTPS; actívala detrás de un proxy TLS |
| `public_origin` | `DORY_PUBLIC_ORIGIN` | vacío | Origen HTTPS público detrás de un proxy |
| `session_ttl` | `DORY_SESSION_TTL` | `168h` | Duración de la sesión web |

Ajustes de limpieza del nodo, en las tablas `[tasks]` y `[threads]`. También se cambian en **Ajustes → Tareas**; lo que guardas allí va a `settings.json` del directorio de datos, no se replica y tiene prioridad sobre el fichero:

| Clave | Por defecto | Qué es |
|---|---|---|
| `tasks.archive_threads_on_finish` | `false` | Al pasar una tarea a hecha o cancelada (a mano, por MCP, o porque su PR se fusiona o se cierra) se detienen y archivan sus hilos. El archivado por merge o cierre llega con el retraso del refresco de GitHub. Solo afecta a los hilos de este nodo |
| `threads.remove_worktree_on_archive` | `true` | Al archivar un hilo se borra su worktree si está limpio y ningún otro hilo lo usa. Con cambios sin commitear se conserva: la lista de hilos avisa y ofrece **Eliminar worktree**, que lo borra a la fuerza |

Otras variables:

- `DORY_GITHUB_TOKEN`: token de GitHub. Tiene prioridad sobre el inicio de sesión desde la interfaz y sobre la sesión de `gh`. El token no sale del nodo.
- `DORY_GITHUB_CLIENT_ID`: sustituye el `client_id` de la OAuth App de Dory, para desarrollo o forks.

### GitHub desde la interfaz

En **Ajustes → GitHub**, o en la vista de GitHub sin sesión, «Iniciar sesión con GitHub» usa el device flow: muestra un código, abre `github.com/login/device` y, al autorizarlo, guarda el token en `github.token` del directorio de datos (permisos `0600`). No se replica a otros nodos.

El token se resuelve en este orden: `DORY_GITHUB_TOKEN`, el guardado desde la interfaz y `gh auth token`. Se vuelve a leer cada 30 s, tras un `401` o mientras no haya token, así que iniciar o cerrar sesión (también con `gh auth login`) no exige reiniciar el nodo.
- `DORY_WHISPER_URL`: servidor de whisper.cpp para el dictado.

## Ficheros del nodo

El entorno de desarrollo iniciado con `scripts/dev.sh` usa `~/.dory/dev`. Electron guarda su perfil en `~/.dory/desktop`. Las instalaciones con una base de datos o un perfil anterior siguen usando esas rutas hasta que su copia esté disponible en la nueva ubicación. Una ruta explícita de datos tiene prioridad.

En `data_dir`:
- `dory.db`: datos. Las tablas replicadas solo cambian por el registro de operaciones.
- `attachments/<hilo>`: archivos adjuntos de los mensajes.
- `node.key`: identidad del nodo (permisos 0600).
- `vapid.json`: claves de los avisos push.
- `worktrees/<hilo>`: un worktree por hilo de implementación.
- `skills-manifest.json`: ficheros de skills que escribió el nodo.
