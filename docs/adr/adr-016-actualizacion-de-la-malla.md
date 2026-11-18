# ADR-016: Actualización de la malla desde las releases de GitHub

- Status: accepted
- Date: 2026-11-18

## Context

Cada nodo se instalaba compilando desde el código, y un nodo con una versión antigua puede dejar de sincronizar con los nuevos (ADR-014). Hacía falta llevar todos los nodos conectados a la misma versión desde la interfaz, también la app de escritorio, y de paso actualizar los proveedores. El repositorio es privado.

## Decision

- `release.yml` publica `vX` como estable y `nightly` una vez al día desde `main`, solo si cambió (o a mano con `workflow_dispatch`). En los pull requests solo corren los tests y los binarios del núcleo, para no gastar minutos de Mac (×10) y Windows (×2). Compila el núcleo con `-ldflags "-X .../internal/version.Channel=stable|nightly"`. Un build local queda en el canal `dev` y no se actualiza solo. La app de escritorio toma la versión del tag, o `<versión>-nightly.<run>` en nightly. Se publican también los `latest*.yml`, los `.blockmap` y el `zip` de Mac.
- `internal/update`: cada núcleo resuelve la release de su canal y su commit con `gh`, que usa las credenciales del usuario. Descarga `dory-<so>-<arch>` en `<datos>/updates`, sustituye su binario y se reinicia: con el servicio, sale con error y systemd o launchd lo arrancan de nuevo; sin servicio, lanza `dory restart`. Si existe `~/.dory/desktop`, descarga también los ficheros de escritorio de su sistema en `<datos>/updates/desktop`.
- La app de escritorio usa electron-updater con el proveedor `generic` apuntando a `/api/mesh/desktop/` del núcleo local, con el token de control. Así no necesita credenciales de GitHub. Al descargar la actualización, la instala y se reinicia.
- API: `GET|POST /api/nodes/{node}/update` (reenviado por la malla) y `GET|POST /api/mesh/update`, que lo aplica a este nodo y a todos los conectados. No actualiza un nodo con turnos en marcha salvo con `force`. Con `providers` ejecuta antes el comando de actualización de cada arnés instalado.

## Consequences

- Cada nodo necesita `gh` con sesión iniciada para actualizarse.
- En Mac, la actualización automática de la app exige firmarla; mientras no se firme, hay que reinstalarla a mano.
- El núcleo que va dentro de un AppImage es de solo lectura; en ese caso lo actualiza la propia app de escritorio.
