# Conectar el MCP `dory`

dory expone un servidor MCP por streamable HTTP en `/mcp`. Se autentica con un token bearer.

## 1. Crear el token

En la máquina donde corre dory:

```sh
set -a; . ~/.config/dory/dory.env; set +a
dory token create
```

El token se muestra una sola vez. Crea uno por cliente para poder revocarlos por separado.

## 2. URL

- En la misma máquina: `http://127.0.0.1:8080/mcp`.
- Desde otra máquina, con Tailscale: `https://<host>.<tailnet>.ts.net/mcp`.

En los ejemplos, exporta el token en tu shell: `export DORY_TOKEN=<token>`.

## Claude Code

```sh
claude mcp add --transport http dory http://127.0.0.1:8080/mcp \
  --header "Authorization: Bearer $DORY_TOKEN"
```

Añade `--scope user` para tenerlo en todos los proyectos. Comprueba con `claude mcp list`.

## Codex

En `~/.codex/config.toml`:

```toml
[mcp_servers.dory]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "DORY_TOKEN"
```

Codex lee el token de la variable de entorno `DORY_TOKEN`, que debe estar definida en el entorno donde arranca.

## T3 Code

T3 Code lanza Claude Code y Codex, así que usa la misma configuración de cada uno:

- Para sesiones con Claude: registra el servidor con `claude mcp add --scope user`, como arriba.
- Para sesiones con Codex: configura `~/.codex/config.toml` y exporta `DORY_TOKEN` en el entorno desde el que arranca T3 Code (por ejemplo, en su unidad systemd o en el perfil de la shell).

T3 Code y los agentes deben ejecutarse en una máquina con acceso a la URL de dory.

## Herramientas disponibles

`session_start`, `list_projects`, `get_project`, `list_items`, `get_item`, `create_item`, `update_item`, `update_items`, `move_item`, `archive_item`, `list_features`, `upsert_feature`, `list_environments`, `get_environment`, `list_repositories`, `remember`, `update_memory`, `archive_memory`, `recall`, `link`, `upsert_project`, `archive_project`, `upsert_repository`, `set_repository_location`, `link_thread`.

Los environments son de solo lectura: los crea el descubrimiento de T3 Code (`DORY_T3_DISCOVERY_CIDR`). La API REST solo expone `GET /api/environments`.

Se recomienda que el agente llame a `session_start` al empezar cada sesión.

## Vistas en los listados

`list_items`, `list_features`, `list_projects` y `recall` aceptan `view`: `summary` (por defecto) o `full`.

- Item resumido: `id`, `title`, `status`, `project_id`, `feature_id`, `priority`, `tags`, `updated_at`.
- Feature resumida: `id`, `project_id`, `title`, `status`, `updated_at`.
- Proyecto resumido: `id`, `slug`, `name`, `status`, `stack`.
- Memoria resumida (`recall`): `id`, `kind`, `title`, `tags` y los 200 primeros caracteres de `content`.

Con `view: "full"` se devuelven los registros completos, como antes. `get_item` siempre devuelve el item entero.

## Respuestas de escritura y contexto de proyecto

`create_item`, `update_item` y `move_item` devuelven `{id, status, updated_at}`. Con `return: "full"` devuelven el item completo.

`get_project` y `session_start` usan la vista resumida para los items abiertos y las memorias. Los cuerpos se leen con `get_item` y `recall` con `view: "full"`.

## Filtros de `list_items`

`fields` filtra por igualdad exacta; `tags` y `refs` usan contención. `status` e `id` aceptan además un array con el sentido "cualquiera de", por ejemplo `{"status": ["triaged", "in_progress"]}`. `open: true` excluye los items `done` y `cancelled`, y se combina con el resto de filtros.

## Actualización en bloque

`update_items` aplica hasta 1000 cambios de items en una transacción: si uno falla (id inexistente, estado desconocido, `blocked` sin `blocked_reason`), no se aplica ninguno. Acepta `updates: [{id, changes}]` y/o `ids` con unos `changes` comunes, con los mismos campos que `update_item`. Devuelve `[{id, status, updated_at}]`.
