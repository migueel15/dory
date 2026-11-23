# ADR-031: HTML renderizado en el chat

- Status: accepted
- Date: 2026-11-23

## Context

Una gráfica, un diagrama o una maqueta se explican mejor viéndolos que en Markdown. T3 Code ofrece `html_render`: el agente publica una página autocontenida que la interfaz pinta en un iframe aislado, con el tema de la aplicación y la altura ajustada al contenido.

## Decision

- La herramienta MCP `render_html(title, html, height?)` solo valida (HTML no vacío y de hasta 200 000 caracteres, título de hasta 200, altura de 80 a 2000) y devuelve `{status: shown}`. No guarda archivos.
- La página viaja en los argumentos de la llamada: el runtime no recorta a 4000 caracteres los de `render_html`, así se guarda y se sincroniza con el mensaje como cualquier otra llamada.
- La web la pinta con `srcdoc` en un iframe `sandbox="allow-scripts allow-popups"` (origen opaco, sin acceso a la sesión de Dory), inyecta las variables de tema claro u oscuro y ajusta la altura por `postMessage`.
- La tarjeta queda fuera del bloque plegado "Worked for…": forma parte de la respuesta.
- No hay `html_preview` con captura, a diferencia de T3 Code.

## Consequences

- Los mensajes con HTML pesan hasta 200 KB en la base de datos y en la malla.
- La página puede cargar recursos remotos y ejecutar scripts, pero aislada del origen de Dory.
- El agente no ve cómo queda la página antes de mostrarla.
