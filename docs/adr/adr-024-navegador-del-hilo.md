# ADR-024: Navegador del hilo con dos motores

- Status: accepted
- Date: 2026-11-21

## Contexto

Los agentes necesitan ver y usar páginas web en cualquier nodo, también sin interfaz y sin que el hilo esté abierto. El usuario quiere trabajar en la misma página que el agente desde la app de escritorio, con una vista nativa y fluida.

## Decisión

- Cada hilo tiene pestañas guardadas en la tabla local `thread_browser_tabs` de su nodo; no se replican.
- Una pestaña corre en un Chromium sin interfaz del nodo, o en una `WebContentsView` de la app de escritorio mientras el usuario la tiene visible. El nodo controla las dos por CDP: la vista nativa a través del depurador de Electron, relayado por el WebSocket del panel.
- Al cambiar de motor se conserva la URL; el estado que no está en la URL se pierde. Mientras el agente tiene un turno, la pestaña no cambia de motor y el escritorio la ve por screencast.
- Cada motor tiene su perfil persistente (`browser-profile/` en el nodo, `persist:dory-browser` en el escritorio), compartido por todos los hilos.
- Para hilos de otro nodo, el escritorio escucha el mismo puerto de loopback y lleva cada conexión en crudo por WebSocket a ese nodo, que solo conecta a `127.0.0.1` y nunca a los puertos de Dory. Cada nodo puede bloquearlo en ajustes.

## Consecuencias

Un inicio de sesión hecho en un motor no existe en el otro. Los nodos que usen el navegador necesitan Chromium instalado y su memoria. El túnel solo funciona con la app de escritorio abierta y si el puerto está libre en el equipo del usuario.
