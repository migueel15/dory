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

Otras variables:

- `DORY_GITHUB_TOKEN`: token de GitHub. Sin él, Dory usa la sesión de `gh` de la máquina. El token no sale del nodo.
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
