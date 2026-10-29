# Dory MVP: plan de trabajo

Dory es la app personal de Miguel que sustituirá al tracker `work/` y a la memoria de SecondBrain para el desarrollo con agentes. Un único binario de Go sirve una API REST, un servidor MCP llamado `dory` y una SPA de React empotrada. Los datos van en Postgres.

Este plan cubre **solo la fase 1 (MVP)**. Fuera de alcance: el triaje automático con las CLIs, `dory sync`, la captura desde el móvil, la búsqueda semántica y la adaptación de las skills.

## Reglas de trabajo
- Sin tracking: nada de `work/` ni `$task-workflow`, ni issues ni PRs. No se crea ninguna issue ni PR sin que Miguel apruebe el texto. Nunca se hace merge a `main`.
- `main` solo recibe el commit inicial del esqueleto. Todo el MVP se integra en la rama `feat/mvp`; cada slice va en su propia rama, que sale de `feat/mvp` y vuelve a ella. El orquestador hace los merges locales a `feat/mvp` y la sube (push).
- Commits con Conventional Commits: tipo en inglés y resumen en español (por ejemplo, `feat(api): añade endpoints de items`). **Sin atribución a IA**: nada de `Co-Authored-By`, "Generated with" ni firmas.
- Testing mínimo: `go build ./...`, `go vet ./...`, `npm run build` y una prueba de humo al final. No se escriben suites de tests en el MVP.
- Slices de unas 400 líneas de lógica propia (sin contar el código generado).
- Modelos: el orquestador es Opus 5.5 low. Los implementadores son Sonnet 5.5 medium (`claude-sonnet-5-5`, effort medium) y GPT-6.1 Sol medium (`gpt-6.1-sol`, reasoningEffort medium). Haiku 5.5 se usa para tareas mecánicas. Como mucho 5 subagentes a la vez, en segundo plano.

## Decisiones fijadas
- **Backend:** Go, `net/http` estándar, SDK oficial de MCP (`github.com/modelcontextprotocol/go-sdk`, transporte streamable HTTP en `/mcp`), `pgx` + `sqlc` y migraciones con `goose`.
- **Estructura:**
  - `cmd/dory`: subcomandos `serve`, `migrate`, `user create`, `token create`
  - `internal/{config,store,service,api,mcp,auth}`
  - `migrations/`
  - `web/`
- **Capa de servicio:** REST y MCP llaman siempre a `internal/service` y nunca tocan la BD directamente. Borrar siempre significa archivar (`archived_at`). Cada cambio escribe en la tabla `events`.
- **Frontend:** React + Vite + TS, TanStack Query + Router, Tailwind + shadcn/ui, dnd-kit para el kanban y `react-markdown`. Se compila a `web/dist` y se empotra con `go:embed`.
- **Contrato:** `api/openapi.yaml` es la fuente de verdad para REST, y los tipos de TS se generan a partir de él.
- **Auth:**
  - Web: usuario y contraseña (argon2id), sesión en cookie `HttpOnly`/`Secure`/`SameSite=Lax` y límite de intentos.
  - Clientes (agentes, móvil): tokens bearer con hash y permisos (`read`, `write`, `admin`).
- **Despliegue:** en la VM de T3 Code. `dory` es un servicio de systemd con el usuario `miguel`, y Postgres corre en Docker Compose con el puerto ligado a localhost. La configuración va en `~/.config/dory/config.toml` y los secretos en un fichero de entorno con permisos 600. Acceso por Tailscale.

## Modelo de datos (MVP)
- `projects`: id, slug, name, description, status, stack, timestamps, archived_at
- `repositories`: id, project_id, remote_url (único), default_branch, timestamps
- `environments`: id, name, os, access (texto o markdown: alias SSH, Tailscale…), agents_available, notes
- `repository_locations`: repository_id, environment_id, path
- `features`: id, project_id, title, body_md, code_refs, status (`implemented|deprecated`), timestamps
- `items`: id (UUID que puede generar el cliente, para que el envío sea idempotente), project_id NULL, raw_text, title, body_md, tags text[], status (`inbox|triaged|ready|in_progress|done|archived`), priority, source, timestamps
- `memories`: id, scope (`global|project|environment`), scope_id, content, tags, timestamps
- `links`: from_id, from_type, to_id, to_type, kind
- `events`: id, entity_type, entity_id, actor, change jsonb, at
- `users`, `sessions`, `api_tokens`
- Búsqueda: texto completo de Postgres (`tsvector`) sobre items, features y memories

## Herramientas del MCP `dory`
`session_start(project?)`, `list_projects`, `get_project` (con repos, features, memorias y tareas abiertas), `list_items`, `get_item`, `create_item`, `update_item`, `move_item`, `archive_item`, `list_features`, `upsert_feature`, `list_environments`, `get_environment`, `list_repositories`, `remember`, `recall(query, scope?)`, `link`.

## Oleadas

### Oleada 0: repositorio (Haiku 5.5, low), secuencial
- Crear el repo privado `migueel15/dory` con `gh` y clonarlo en `/home/miguel/repos/dory`.
- Esqueleto: `go mod init github.com/migueel15/dory`, carpetas vacías con `.gitkeep`, `.gitignore` (Go, Node, `web/dist`, `.env`) y un `README.md` breve en español.
- Commit inicial en `main` y push. Después, crear la rama `feat/mvp` y subirla.
- Registrar el repo como proyecto en T3 Code (`t3_project_create`), para poder lanzar subagentes en worktrees.

### Oleada 1: contratos (1 agente, Sol medium), secuencial
Rama `feat/foundation`. Es la base que deja trabajar en paralelo al resto:
- Migraciones SQL del modelo completo, `sqlc.yaml` y las queries básicas (CRUD y listados con filtros).
- `internal/config` (TOML + entorno) y `docker-compose.yml` con Postgres.
- `internal/service`: tipos de dominio e **interfaces** de servicio con firmas definitivas, más una implementación sobre el store, aunque sea simple.
- `api/openapi.yaml` completo para el MVP.
- `cmd/dory` con `serve` (router vacío que monta `/api`, `/mcp` y `/`) y `migrate`.

Antes de la oleada 2, el orquestador integra esta rama en `feat/mvp`.

### Oleada 2: en paralelo (hasta 5 agentes, cada uno en su worktree desde `feat/mvp`)
| Slice | Rama | Modelo | Contenido |
|---|---|---|---|
| A | `feat/rest-api` | Sonnet medium | Handlers REST según el OpenAPI, sobre `service`. Errores JSON uniformes. SSE `/api/events/stream` (simple) |
| B | `feat/mcp` | Sol medium | Servidor MCP con todas las herramientas de arriba sobre `service`, autenticado con token bearer |
| C | `feat/auth` | Sol medium | Usuarios, sesiones, tokens, middleware de la web y de los tokens, límite de intentos, subcomandos `user create` y `token create`, endpoints de login y logout |
| D | `feat/web` | Sonnet medium | SPA con login, inbox (crear y editar items), kanban por proyecto con arrastrar y soltar, fichas de proyecto, entornos, memorias y features. Tipos generados del OpenAPI. `go:embed` de `web/dist` |
| E | `feat/deploy` | Sonnet medium | Unidad systemd, `config.example.toml`, ejemplo de fichero de entorno, script `scripts/install.sh` (idempotente; no lo ejecuta), backup diario con `pg_dump` y un timer, y `docs/` con la instalación y la configuración del MCP en Claude y Codex |

Si alguna slice supera las ~400 líneas, se divide (por ejemplo, la web en D1, shell + login + inbox, y D2, kanban + fichas).

### Oleada 3: integración (orquestador + 1 agente de arreglos)
- Merge de A a E en `feat/mvp`, resolviendo conflictos.
- `go build`, `go vet`, `npm run build` y prueba de humo: levantar Postgres y `dory serve`, crear un usuario y un token, `curl` a login e items, listar las herramientas MCP y hacer un `create_item` por MCP.
- Push de `feat/mvp`.
- **No** instalar el servicio systemd ni tocar el sistema sin confirmación explícita de Miguel.

## Entregable
La rama `feat/mvp` subida, compilando, con la prueba de humo superada. Se acompaña de un resumen en español para Miguel: qué funciona, cómo instalarlo, cómo conectar el MCP en T3 Code, y la lista de pendientes para iterar.
