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

## App de Android

La app está en `mobile/`. Encuentra los nodos de tu red local, y por Tailscale los de la malla de un nodo ya emparejado, y abre directamente el que elijas. No necesita un servidor central (ADR-014).

```sh
cd mobile && npm ci && npx cap sync android
cd android && JAVA_HOME=/usr/lib/jvm/java-21-openjdk ./gradlew assembleDebug
adb install app/build/outputs/apk/debug/app-debug.apk
```

Gradle necesita JDK 17 o 21, y el SDK de Android en `ANDROID_HOME` o en `mobile/android/local.properties`.

Para emparejar, abre en el ordenador **Entornos → Mostrar código** y escribe el código en la app, en el nodo que aparece o con su dirección. Cada nodo se empareja por separado. Los móviles emparejados aparecen en Entornos y se pueden revocar desde ahí.

La interfaz también funciona como PWA si publicas un nodo por HTTPS (`tailscale serve --bg --https=443 http://127.0.0.1:7700`, con `public_origin` y `cookie_secure = true`).

## Versiones

- **Estable:** cada etiqueta `v*` publica una release.
- **Nightly:** cada push a `main` actualiza la release `nightly`.
- **Pull requests:** cada push a la rama de una PR genera binarios y apps descargables como artefactos del workflow.
