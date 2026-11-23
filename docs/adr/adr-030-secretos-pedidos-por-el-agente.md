# ADR-030: Secretos pedidos por el agente

- Status: accepted
- Date: 2026-11-23

## Context

El agente a veces necesita una contraseña de sudo, un token o un código 2FA. Si lo pide en el chat, el valor queda en los mensajes, en la base de datos y en el contexto del modelo. t3code resuelve esto con una herramienta que guarda el valor en un archivo y devuelve solo su ruta.

## Decision

- La herramienta MCP `request_secret` muestra un formulario de 1 a 8 campos en el composer. Es una petición `secret` de `thread_requests` que solo guarda los metadatos (título, motivo, url, campos), nunca los valores.
- El usuario responde por `POST /api/threads/{id}/secrets/{req}`, separado de las respuestas a preguntas. Los campos deben coincidir exactamente.
- Cada valor se escribe en `<datos>/secrets/files/<32 hex>` (directorio `0700`, archivo `0600` creado con `O_EXCL` y renombrado). El agente recibe `file` o `files` por nombre de campo. Un `value` pegado por el usuario se escribe sin formulario.
- Los archivos se borran a las 24 h, al arrancar el nodo y cada hora.
- Rechazar o agotar el tiempo (10 min por defecto, máximo 1 h) no escribe nada.

## Consequences

- El agente puede leer e imprimir los archivos: se confía en sus instrucciones, como en t3code.
- Los valores quedan en claro en disco hasta 24 h, protegidos solo por los permisos.
- Si el nodo se reinicia con una petición pendiente, se descarta como las demás.
