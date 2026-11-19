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
| `dory service install`, `uninstall` | Servicio de usuario: systemd (Linux), LaunchAgent (macOS) o entrada de inicio de sesión sin consola (Windows) |
| `dory update` | Instala la última versión del canal del binario (estable o nightly) desde GitHub, también la app de escritorio si la hay, y reinicia el núcleo. Para toda la malla, Entornos → «Actualizar la malla» |
| `dory user create --username X` | Crea una cuenta (contraseña en `DORY_PASSWORD`) |
| `dory token create --username X --name N --scopes read,write` | Crea un token para la API o el MCP |

Un binario compilado en local (`scripts/install.sh`) es del canal `dev` y no se actualiza. Para pasar a la nightly la primera vez:

```bash
curl -fL -o ~/.local/bin/dory "https://github.com/migueel15/dory/releases/download/nightly/dory-$(go env GOOS)-$(go env GOARCH)"
chmod +x ~/.local/bin/dory && dory restart
```

## App de escritorio

La app de Electron está en `desktop/` y lleva el binario dentro. Basta con instalarla: al abrirla por primera vez copia su núcleo a `~/.local/bin/dory` (en Windows, `%LOCALAPPDATA%\Dory\bin\dory.exe`) y lo deja como servicio de usuario (`dory service install`), así que el núcleo sigue en marcha al cerrar la ventana y arranca con la sesión. Si ya había un `dory` instalado por otra vía, la app lo respeta. En las siguientes versiones actualiza el núcleo que instaló ella. Al cerrar la ventana, la app sigue en segundo plano con un icono en la bandeja del sistema (en Linux necesita un panel con StatusNotifierItem, como KDE, Waybar o Quickshell; en GNOME, la extensión AppIndicator); se abre desde ahí y se cierra con **Salir**. Arranca oculta al iniciar sesión, así que el móvil y la malla siempre encuentran el nodo. Con el AppImage, además, instala su icono y la entrada `~/.local/share/applications/dory.desktop`, para que Dory aparezca con su icono en el lanzador de aplicaciones, y enlaza `~/.local/bin/Dory.AppImage` para que `dory open` y el lanzador abran la app.

```sh
go build -o desktop/bin/dory ./cmd/dory
cd desktop && npm ci && npm run dist
```

En Windows, el instalador pide permiso de administrador una vez para abrir en el firewall el puerto de la malla (7710, TCP), por el que entran el móvil y los demás nodos. Las actualizaciones no lo vuelven a pedir.

Genera AppImage en Linux (el único formato que se actualiza solo), .dmg en macOS e instalador NSIS en Windows. Las releases de GitHub las publican ya hechas.

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
