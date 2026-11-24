# ADR-045: Sesiones del usuario solo en el navegador visible

- Status: accepted
- Date: 2026-11-24

## Contexto

Algunos sitios, como Google, no dejan iniciar sesión en un navegador integrado. Para usarlos en el navegador del hilo hay que llevar allí las sesiones del navegador del usuario. Los agentes también usan ese navegador (ADR-024), y unas cookies copiadas les darían las cuentas reales del usuario sin que nadie lo viera.

## Decisión

- La app de escritorio puede copiar las cookies de un navegador de su equipo (Zen, Firefox, LibreWolf, Floorp, Chrome, Chromium o Brave, en Linux) a su perfil `persist:dory-browser`, a mano o en cada arranque. La elección vive en el perfil de la app, porque depende de ese equipo.
- El Chromium sin interfaz de los nodos nunca recibe esas cookies. Un agente en segundo plano navega con su propio perfil: consulta páginas de desarrollo o públicas, pero no las cuentas del usuario.
- Un agente solo llega a esas sesiones a través de una pestaña nativa del escritorio, es decir, con la app abierta y la pestaña visible para el usuario, que puede tomar el control.
- Cada nodo decide con `agents_use_browser` si sus agentes tienen las herramientas `browser_*`. Activado por defecto.

## Consecuencias

El usuario ve todo lo que un agente hace con sus sesiones. Un agente en segundo plano no puede hacer tareas que pidan una cuenta del usuario. Las sesiones copiadas quedan en el perfil del escritorio hasta que caducan, aunque se cierren en el navegador de origen. Leer las cookies de Chromium necesita la clave del llavero del sistema.
