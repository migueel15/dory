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

En el diff de una PR, «Preguntar al agente» manda la consulta, con las líneas seleccionadas, al hilo de revisión de la PR. La primera vez no existe: se elige máquina, proyecto T3 y modelo, y se crea un hilo que sabe que revisa esa PR sin modificar nada salvo petición explícita, y recibe la pregunta. Queda guardado como `issue_threads` de tipo `review` y auditado como `t3.review_pull`. Desde entonces la PR muestra ese hilo como chat a la derecha (60/40, redimensionable y plegable con «Agente»); preguntar desde el diff lo abre si estaba oculto.

El hilo de revisión trabaja en un worktree de la PR: antes de crearlo, Dory pide a T3 (`git.preparePullRequestThread` en modo worktree, en la raíz del proyecto T3 elegido) que haga checkout de la PR, y el hilo nace ligado a ese worktree. El agente lo actualiza antes de cada consulta y cita los ficheros como `ruta:línea`, que en Dory abren el diff en esa línea. Cada 10 minutos Dory revisa las PR con hilo de revisión: si una se ha mergeado o cerrado, elimina el worktree directamente por T3 (`vcs.removeWorktree`, sin forzar: git no lo borra si tiene cambios sin guardar) y archiva la asociación, así que la PR deja de mostrar el chat. Si la máquina no responde o el borrado falla, lo reintenta en la siguiente pasada. No interviene el agente.

Al lanzar un item con worktree, Dory pide a T3 (`vcs.createWorktree`, en la raíz del proyecto T3, sobre `origin/<base>` o `origin/HEAD`) un worktree con `path: null` y `newRefName: dory/<id8>`, y crea el hilo ligado a él; así el panel de cambios muestra el diff desde la primera edición. Dory guarda en el item la ref `worktree:<ruta>`. T3 deriva la carpeta de la rama provisional (por ejemplo, `~/.t3/worktrees/<repo>/dory-<id8>`). Si el item ya tiene una ref `worktree:<ruta>`, Dory reutiliza esa ruta sin crear otro worktree. Antes del primer commit, el agente renombra la rama provisional con `git branch -m <nombre>` según las skills del proyecto; en un worktree reutilizado conserva la rama si ya está renombrada. La carpeta mantiene su nombre aunque cambie la rama. Si el lanzamiento falla antes de crear el hilo, Dory elimina el worktree nuevo; no borra la rama provisional. Si esa rama ya existe sin una ref de worktree reutilizable, T3 devuelve un error. Cuando la PR del item se mergea o se cierra, Dory elimina el worktree con `vcs.removeWorktree` (sin forzar) y quita la ref. El token de Dory debe autorizar `vcs.createWorktree`; si T3 responde `EnvironmentAuthorizationError`, hay que ampliar su permiso.

## Notificaciones y push

Ver [notificaciones](notificaciones.md) para los ajustes por usuario, la activación
por dispositivo y la migración necesaria. Las claves VAPID se conservan en la base
de datos; no se añaden variables de entorno.

## Título de las ideas con IA

Al crear una idea desde el cuadro de idea, lo que se escribe queda como descripción (`raw_text`) y un hilo de T3 con un modelo rápido genera el título. Mientras tanto el item muestra la primera línea como título provisional y «Generando título…»; al terminar, el título lleva la etiqueta «IA». Si el hilo falla o se agota el tiempo, se queda el título provisional como título del usuario. Al arrancar, Dory trata igual los títulos que hubieran quedado pendientes.

Se configura en la web, en **Ajustes → Títulos con IA**. Los ajustes son globales, se guardan en la tabla `title_settings` (`GET`/`PUT /api/settings/titles`) y se aplican a la siguiente idea sin reiniciar Dory. Ya no hay claves `title_*` en el TOML ni variables `DORY_TITLE_*`.

| Campo | Significado |
|---|---|
| Generar el título | Activa la función. Desactivada, el título es la primera línea. |
| Conexión | Instancia (máquina) de T3 donde se abre el hilo. Obligatoria si está activa. |
| Modelo | Proveedor y modelo del catálogo de esa instancia. Obligatorio si está activa. |
| Esfuerzo | Nivel de razonamiento. Vacío usa el más bajo del modelo. |
| Proyecto de T3 | Proyecto donde se abre el hilo. Vacío usa el primero. |
| Tiempo máximo | Segundos de espera (0–3600). Por defecto 120. |

El hilo necesita el MCP de Dory para llamar a `update_item`. `title_source` (`pending`, `ai`, `user`, `fallback`) indica quién escribió el título: la IA solo puede pasar `pending` a `ai`; editar el título lo deja en `user` y nunca se sobrescribe. Si la generación falla o caduca, el título provisional queda en `fallback` y la web muestra un botón ↻ «Regenerar título con IA» (`POST /api/items/{id}/title/regenerate`), que devuelve el item a `pending` y repite la generación; solo se admite en `fallback` (409 en otro caso, 503 si la generación está apagada). Los items que fallaron antes de este estado siguen en `user`.
