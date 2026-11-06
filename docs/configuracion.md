# Configuración

dory lee, por este orden: valores por defecto, el fichero TOML y las variables `DORY_*`. El TOML es `~/.config/dory/config.toml`, o la ruta de `DORY_CONFIG`. Si `DORY_CONFIG` apunta a un fichero que no existe, es un error. El fichero por defecto es opcional.

Plantillas: [`config.example.toml`](../config.example.toml) y [`deploy/dory.env.example`](../deploy/dory.env.example).

| Clave TOML | Variable | Defecto | Descripción |
|---|---|---|---|
| `listen` | `DORY_LISTEN` | `127.0.0.1:8080` | Dirección `host:puerto` de escucha |
| `database_url` | `DORY_DATABASE_URL` | (obligatoria) | Cadena de conexión a Postgres |
| `cookie_secure` | `DORY_COOKIE_SECURE` | `true` | Cookie de sesión solo por HTTPS (`true` o `false`) |
| `session_ttl` | `DORY_SESSION_TTL` | `168h` | Duración de la sesión web |

## Secretos

Los secretos van en `~/.config/dory/dory.env`, con permisos 600. Lo leen la unidad systemd (`EnvironmentFile`), el backup y `docker compose --env-file`:

- `POSTGRES_PASSWORD`: contraseña de Postgres para `docker-compose.yml`.
- `POSTGRES_PORT`: puerto local de Postgres (opcional, 5432 por defecto).
- `DORY_DATABASE_URL`: debe usar la misma contraseña.

No guardes la contraseña en `config.toml`, que es legible por otros usuarios.

## Resolver conflictos desde GitHub

En una PR abierta con conflictos, «Resolver conflictos» abre el formulario compartido de máquina, proyecto T3, modelo y razonamiento. La máquina predeterminada es la instancia disponible de infra. El proyecto T3 debe corresponder al repositorio (por nombre o directorio raíz).

El hilo recibe instrucciones para trabajar en un worktree temporal, incorporar la base mediante merge, validar y empujar la rama sin force-push. Si hay dudas o fallan las comprobaciones, debe detener el push y explicarlo en el hilo. El lanzamiento se sigue desde Trabajos como «Resolver conflictos» y queda auditado como `t3.resolve_conflicts`. El endpoint devuelve 409 cuando la PR ya no tiene conflictos.

## Notificaciones y push

Ver [notificaciones](notificaciones.md) para los ajustes por usuario, la activación
por dispositivo y la migración necesaria. Las claves VAPID se conservan en la base
de datos; no se añaden variables de entorno.

## Título de las ideas con IA

Al crear una idea desde el cuadro de idea, lo que se escribe queda como descripción (`raw_text`) y un hilo de T3 con un modelo rápido genera el título. Mientras tanto el item muestra la primera línea como título provisional y «Generando título…»; al terminar, el título lleva la etiqueta «IA». Si el hilo falla o se agota el tiempo, se queda el título provisional como título del usuario. Al arrancar, Dory trata igual los títulos que hubieran quedado pendientes.

| TOML | Variable | Significado |
|---|---|---|
| `title_model` | `DORY_TITLE_MODEL` | Modelo de T3 (slug). Vacío apaga la función. |
| `title_provider` | `DORY_TITLE_PROVIDER` | Instancia de proveedor en T3. Vacío usa la primera que ofrezca el modelo. |
| `title_effort` | `DORY_TITLE_EFFORT` | Nivel de razonamiento. Vacío usa el más bajo del modelo. |
| `title_project` | `DORY_TITLE_PROJECT` | Proyecto de T3 (id o título) donde se abre el hilo. Vacío usa el primero. |
| `title_timeout` | `DORY_TITLE_TIMEOUT` | Tiempo máximo de espera. Por defecto `2m`. |

El hilo necesita el MCP de Dory para llamar a `update_item`. `title_source` (`pending`, `ai`, `user`) indica quién escribió el título: la IA solo puede pasar `pending` a `ai`; editar el título lo deja en `user` y nunca se sobrescribe.
