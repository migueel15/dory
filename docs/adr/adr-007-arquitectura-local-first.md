# ADR-007: Arquitectura local-first con nodos sincronizados entre pares

- Status: proposed
- Date: 2026-11-12

## Context

Se quiere comprobar si Dory es viable como app local-first: un nodo en cada dispositivo (escritorio y móvil) que se sincroniza directamente con los demás, sin servidor central y sin depender de servidores del autor. T3 Code queda solo como motor de agentes. El MVP cubre Arch Linux y Android (issue #144, rama `feat/144-local-first-mvp`, PR #145 en borrador). La rama nunca se integra en `main`, porque eso despliega en infra.

Si se adopta, este ADR sustituiría a ADR-002 y ADR-005. Durante el MVP ambos siguen vigentes en producción.

## Decision

- **`dory-node` (Go)**: el mismo binario en todos los dispositivos. Reutiliza sin modificarlos `service.Service`, `api.Register`, `mcp.Handler`, `auth` y `t3.Monitor`. Solo cambia la persistencia, que pasa a `localstore` sobre SQLite (`modernc.org/sqlite`, sin cgo) como almacén de documentos JSON `records(kind, id, data, field_clock, owner_device, created_at, updated_at, archived_at)`.
- **Escritorio**: Wails v3 con el nodo en el mismo proceso, sirviendo la SPA actual; `--headless` arranca sin ventana. **Móvil (Android)**: nodo ligero (réplica y sincronización, sin T3, worktrees ni terminales); las rutas `/api/t3/*` se reenvían al nodo dueño. Plan B: `gomobile bind`.
- **Red**: mDNS (`_dory._tcp`), IP directa o el Tailscale del sistema como una dirección más. Siempre TLS mutuo con las claves de los dispositivos.
- **Dueño único**: un grupo de dispositivos = una cuenta; sin cuentas centrales ni compartir entre personas.
- **Datos**: se replican projects, repositories, environments, features, items, item_plans, memories, links, pr_file_reviews, title_settings, item_threads, issue_threads y t3_instances (sin tokens, con dispositivo dueño); `events` solo añade filas. Se quedan locales users, sessions, api_tokens, notificaciones, el índice FTS y la identidad.
- **Sincronización**: registro `ops(device_id, seq, hlc, entity, entity_id, kind, fields_json, schema_v)` escrito en la misma transacción que cada `Put`/`Archive`, solo con los campos cambiados. Gana la última escritura por campo; los arrays se fusionan como conjunto donde gana añadir; los slugs repetidos se renombran a `-2`; el texto perdedor se guarda en `events`. Cada dispositivo envía su vector de versiones y recibe las operaciones que le faltan ordenadas por HLC; con conexión activa, una suscripción en vivo. Un dispositivo nuevo arranca con una instantánea. Los campos desconocidos se conservan y reenvían.
- **Módulo anidado** `github.com/migueel15/dory/node` con `localstore`, `syncx`, `peer` y `cmd/dory-node` (datos en `$XDG_DATA_HOME/dory-node`, puerto entre pares 7710).
- **Migración**: `dory-node import --from <url> --token <token>` crea operaciones iniciales conservando ids y fechas. Producción no cambia; la migración definitiva se decide tras el MVP.
- **Fuera del MVP**: Windows, macOS, iOS, relé de push, tsnet, interfaz `Runtime`/ACP, CPace, almacén de claves del sistema, compactación del registro, notificaciones, dictado, GitHub en móvil y corte de infra.

## Consequences

- Los datos viven en los dispositivos y no hay servidor central que mantener.
- Los conflictos se resuelven por campo, con posible pérdida visible de ediciones concurrentes (guardadas en `events`).
- Se mantienen dos arquitecturas en paralelo durante el MVP.
- El MVP se valida con ocho pruebas de aceptación (emparejado por QR, sincronización en menos de 2 s, convergencia sin conexión, Tailscale, T3 remoto desde el móvil, MCP local, revocación y detección de desconexión).
