# ADR-031: HTML renderizado en el chat

- Status: accepted
- Date: 2026-11-23

## Context

Una gráfica, un diagrama o una maqueta se explican mejor viéndolos que en Markdown. T3 Code ofrece `html_preview` y `html_render`: el agente comprueba la página con una captura y la publica. Se guarda como adjunto en el entorno dueño del hilo, el mensaje solo lleva su referencia y la interfaz la pinta en un iframe aislado, con el tema de la aplicación y la altura ajustada al contenido. T3 no replica entre entornos: cada uno es dueño de sus hilos, y otro entorno los ve reenviando el flujo del dueño.

## Decision

- Se sigue el mismo modelo. `html_preview(html|path, width?, light?)` abre la página envuelta en el tema en una pestaña oculta del Chromium del nodo y devuelve PNG, `content_height`, `captured_height` y consola.
- `render_html(html|path, title, height)` guarda la página como adjunto del hilo (`<datos>/attachments/<hilo>/`) y responde `{status, html_render: {attachment_id, title, height}}`. El mensaje solo guarda esa respuesta; los argumentos siguen recortados a 4000 caracteres como los de cualquier herramienta.
- `GET /api/threads/{id}/renders/{id}?theme=` sirve la página envuelta en el tema, con CSP `sandbox allow-scripts allow-popups`. Como las demás rutas del hilo, la malla la reenvía al nodo dueño: los mensajes y los adjuntos no se replican (ADR-007).
- La web la carga en un iframe `sandbox="allow-scripts allow-popups"` (origen opaco), ajusta la altura con `postMessage` y la deja fuera del bloque plegado "Worked for…".
- El tema (variables CSS, margen de 8 px, script de altura) vive en Go (`internal/htmlrender`), así la captura y el chat se ven igual.

## Consequences

- La base de datos solo crece con la referencia; la página ocupa disco del nodo dueño, hasta 512 KB.
- Si el nodo dueño no está conectado, la página no se ve, igual que el resto del hilo.
- Un nodo sin Chromium no puede usar `html_preview`, pero sí `render_html`.
