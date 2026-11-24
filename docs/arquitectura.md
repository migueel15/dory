# Arquitectura

Dory es un binario Go (`cmd/dory`) con una web React embebida (`web/`). Cada nodo es igual a los demás: guarda su estado en SQLite y replica las entidades por la malla. Las decisiones están en [docs/adr](adr/README.md).

## Paquetes

| Paquete | Responsabilidad |
|---|---|
| `internal/schema` | Registro único del esquema replicado. De él salen el hash de `/peer/v2/info` y los tipos TypeScript (`go generate`) |
| `internal/data` | Repositorios tipados de las entidades replicadas (`Create`/`Patch`, concurrencia optimista) |
| `internal/oplog` | Registro de operaciones, fusión por reloj híbrido, instantáneas y compactación (ADR-041) |
| `internal/localdb`, `internal/nodestate` | Base SQLite local, migraciones y estado propio del nodo (ADR-033) |
| `internal/mesh`, `internal/outbox` | Emparejamiento, protocolo v2, sincronización y bandeja de salida (ADR-034) |
| `internal/domain` | Reglas puras sin E/S (`board`) |
| `internal/app` | Servicios de aplicación: tareas, planes, memorias, proyectos, ideas, hilos, sesión (ADR-038, ADR-044) |
| `internal/harness` | Contrato de arneses (`Driver`, `Session`, eventos tipados), registro y pool de procesos (ADR-037). Subpaquetes `claude`, `codex`, `opencode`, `acp` (con `antigravity`) y `kit` |
| `internal/runtime` | `Manager`, un actor por hilo con cola persistida y registro de eventos (ADR-039, ADR-040) |
| `internal/workspace` | Git, worktrees y diffs |
| `internal/api` | HTTP: decodifica, llama a `app` o `runtime` y codifica |
| `internal/mcp` | Herramientas MCP sobre los servicios de `app` |
| `internal/apperr`, `internal/i18n` | Errores con código y textos del servidor (ADR-043) |
| Otros | `auth`, `notify`, `github`, `update`, `browser`, `devices`, `dictation`, `htmlrender`, `skills`, `config`, `hlc`, `compress`, `cmdexec` |

En la web, `web/src/features/` agrupa el tablero, el chat, el compositor, GitHub, los ajustes y el hilo; `api/` contiene la capa de datos con los tipos generados, `i18n/` los textos y `lib/` utilidades.

## Flujo de un turno

1. La web o el MCP envía un mensaje; `api` o `mcp` llaman a `app`/`runtime`.
2. El `Manager` entrega la orden al actor del hilo. Si hay un turno en marcha, el mensaje queda en la cola persistida.
3. El actor abre una `Session` del arnés y recibe eventos tipados.
4. El `recorder` guarda cada elemento y su evento en la misma transacción (`thread_events` y `thread_messages`) y calcula consumo, diff y rama.
5. Las suscripciones SSE del hilo reciben los eventos; la web los aplica y puede reanudar con `Last-Event-ID`.
6. Al terminar, el actor decide cola, reconducción y estado. El estado de un hilo es local y se anuncia a los pares sin pasar por el registro replicado (ADR-036).

## Datos heredados

`internal/config/config.go` mantiene `~/.local/share/dory` (o `$XDG_DATA_HOME/dory`) como directorio de datos si ya contiene `dory.db`: hay nodos que arrancaron con esa ruta y moverla los dejaría sin su base. Por eso no se elimina. Los mensajes guardados antes de los códigos (`code`) y las llamadas anteriores a `kind` siguen leyéndose (`legacyTool`, `LegacyStep`).

## Deuda conocida

- La web usa tipos escritos a mano sin equivalente Go generado (`Mesh`, `Peer`, `NodeInfo`, `NodeStatus`, `Device*`, `PullReview`, `PullAgent`) y unos 70 `useQuery` en línea con claves `qk`.
- `web/src/features/chat/chat.tsx` y `steps.tsx` se importan entre sí (`Entry` y `elapsed`); `Entry` se renderiza dentro de los pasos y moverlo exige separar el componente.
- `api.Server` conserva campos de infraestructura y `mcp.Deps` conserva `Store` para las herramientas de hilos, enlaces, dependencias y búsqueda.
- Siguen en español en Go los textos que llegan al agente o tienen efecto funcional (ADR-043).
- Un fragmento de texto que llega justo tras un refetch puede dejar el texto incompleto hasta `item.completed` (ADR-040).
