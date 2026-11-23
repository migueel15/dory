# ADR-029: El núcleo vive dentro de la app de escritorio

- Status: accepted
- Date: 2026-11-23

## Context

La app de escritorio instalaba su núcleo aparte, como servicio de usuario (`installCore`, ADR-016), y el núcleo se sustituía a sí mismo al actualizarse. En Windows esa sustitución fallaba, el error se ignoraba porque había escritorio y el núcleo se reiniciaba con la versión vieja: la malla pedía la misma actualización una y otra vez.

## Decision

- En un equipo con app de escritorio, Dory es la app: arranca el núcleo que trae incluido (`resources/bin`) como proceso hijo con `serve` y `DORY_DESKTOP_CORE=1`, y lo para al salir desde la bandeja. Si el hijo termina sin que la app esté saliendo, lo vuelve a lanzar. Si ya hay un núcleo sano en el puerto, como uno de desarrollo, lo usa.
- La app se abre con la sesión, oculta en la bandeja: `setLoginItemSettings` en Windows y macOS, `~/.config/autostart` en Linux.
- Un núcleo lanzado por la app no se sustituye: descarga los ficheros de escritorio en `<datos>/updates/desktop` y electron-updater reinicia la app con el núcleo nuevo.
- En modo servicio (servidores sin interfaz) todo sigue como en ADR-016, pero un fallo al sustituir el binario se devuelve como error y no reinicia.
- La app deja la CLI `dory` en el PATH apuntando a su binario: copia en `~/.local/bin` en Linux y macOS, y la carpeta de la app en el PATH de usuario en Windows.
- Migración: si existe el marcador `core-managed`, la app para el núcleo instalado, quita su servicio (en Windows, la entrada `Run`) y borra el binario y el marcador. Un núcleo instalado a mano no se toca.

## Consequences

- Con la app cerrada, ese equipo no sincroniza la malla.
- Los equipos atascados en una versión anterior necesitan instalar una vez el nuevo instalador a mano.
- Sustituye la parte de ADR-016 sobre `installCore` y el servicio en equipos con app.
