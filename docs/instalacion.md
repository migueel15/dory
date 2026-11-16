# Instalación

## Binario

Necesita Go y Node.js para compilar desde el repositorio.

```sh
scripts/install.sh
```

El script compila la interfaz web y el binario, lo instala en `~/.local/bin/dory` y deja el núcleo como servicio de usuario (`dory service install`). El núcleo arranca con tu sesión y sigue en marcha aunque cierres la interfaz.

Al abrir `http://127.0.0.1:7700` por primera vez desde la misma máquina, creas la cuenta del nodo (contraseña de 12 caracteres o más).

Comandos del núcleo:

| Comando | Qué hace |
|---|---|
| `dory status` | Dice si el núcleo está en marcha, con su nombre y versión |
| `dory start`, `stop`, `restart` | Controla el núcleo: con el servicio si está instalado; si no, `start` lo lanza en segundo plano |
| `dory open` | Arranca el núcleo si hace falta y abre la interfaz en una ventana de app |
| `dory service install`, `uninstall` | Servicio de usuario de systemd (Linux) o LaunchAgent (macOS) |
| `dory update [--channel stable\|nightly]` | Descarga la última versión del canal con `gh` y reinicia el núcleo |
| `dory user create --username X` | Crea una cuenta (contraseña en `DORY_PASSWORD`) |
| `dory token create --username X --name N --scopes read,write` | Crea un token para la API o el MCP |

## App de escritorio

La app de Electron está en `desktop/`. Arranca el núcleo si no está en marcha, nunca lo para al cerrarse y lleva el binario dentro.

```sh
go build -o desktop/bin/dory ./cmd/dory
cd desktop && npm ci && npm run dist
```

Genera AppImage, paquete de pacman y .deb en Linux, .dmg en macOS e instalador NSIS en Windows. Las releases de GitHub las publican ya hechas.

## App móvil

La interfaz es una PWA instalable. Desde el móvil, conéctate al nodo servidor y usa «Añadir a la pantalla de inicio». El navegador solo la instala y permite avisos por HTTPS. Con Tailscale, publica el nodo del servidor con un certificado válido:

```sh
tailscale serve --bg --https=443 http://127.0.0.1:7700
```

Pon `public_origin = "https://<servidor>.<tailnet>.ts.net"` y `cookie_secure = true` en la configuración del servidor. Después activa los avisos con la campana de la barra lateral.

## Versiones

- **Estable:** cada etiqueta `v*` publica una release.
- **Nightly:** cada push a `main` actualiza la release `nightly`.
- **Pull requests:** cada push a la rama de una PR genera binarios y apps descargables como artefactos del workflow.
