# ADR-018: Antigravity CLI mediante ACP

- Status: accepted
- Date: 2026-11-19

## Context

Dory ejecuta los arneses en el nodo del hilo (ADR-008) y normaliza sus turnos interactivos (ADR-015). Antigravity CLI ofrece ACP por JSON-RPC sobre stdio y guarda las conversaciones y las credenciales en su perfil local.

## Decision

Dory abre `agy acp` como proceso hijo para cada turno activo. Crea la sesión con el directorio del hilo y guarda el identificador para cargarla en turnos posteriores. Registra el MCP de Dory por HTTP si el servidor lo anuncia y usa un puente stdio al mismo endpoint en las demás versiones. Importa conversaciones con `session/list` y `session/load`.

El arnés usa su modo `default`. Dory aprueba las solicitudes de permiso con acceso completo. En acceso restringido habilita el sandbox del arnés y aprueba únicamente ediciones cuyos destinos se resuelven dentro del worktree. Las preguntas nativas se convierten en peticiones interactivas del hilo.

## Consequences

Cada nodo usa su propia sesión de Google y el binario oficial de su plataforma. Las skills del catálogo se enlazan en `~/.gemini/config/skills`. Si Windows impide crear un enlace, Dory instala una copia gestionada. El historial de tokens se obtiene de la metadata protobuf de las bases SQLite locales, abierta en modo de solo lectura. Dory lee los saldos de suscripción ejecutando `agy -p /quota`, que responde sin modelo con una fila TSV por ventana y no crea conversaciones. Dory no lee ni usa credenciales de Antigravity ni llama a Google directamente: todo el tráfico sale del binario oficial. Si el formato cambia, la página de uso queda sin datos. Dory guarda los saldos por ventana y omite las ventanas vencidas hasta obtener una lectura actual.
