# Instalación

## Binario

Necesita Go y Node.js para compilar desde el repositorio.

```sh
scripts/install.sh
```

El script compila la interfaz web y el binario, lo instala en `~/.local/bin/dory` y deja el núcleo como servicio de usuario (`dory service install`). El núcleo arranca con tu sesión y sigue en marcha aunque cierres la interfaz.

Al abrir `http://127.0.0.1:7700` por primera vez desde la misma máquina, creas la cuenta del nodo. Escribe una contraseña de al menos ocho caracteres y repítela para confirmarla.

Comandos del núcleo:

| Comando | Qué hace |
|---|---|
| `dory status` | Dice si el núcleo está en marcha, con su nombre y versión |
| `dory start`, `stop`, `restart` | Controla el núcleo: con el servicio si está instalado; si no, `start` lo lanza en segundo plano |
| `dory open` | Arranca el núcleo si hace falta y abre la interfaz en una ventana de app |
| `dory service install`, `uninstall` | Servicio de usuario: systemd (Linux), LaunchAgent (macOS) o entrada de inicio de sesión sin consola (Windows) |
| `dory update` | Instala la última versión del canal del binario (estable o nightly) desde GitHub, también la app de escritorio si la hay, y reinicia el núcleo. Para toda la malla, Entornos → «Actualizar la malla» |
| `dory user create --username X` | Crea una cuenta (contraseña de al menos ocho caracteres en `DORY_PASSWORD`) |
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

El AppImage se publica siempre como `Dory.AppImage`, sin versión. Guárdalo con ese nombre (por ejemplo en `~/Apps/Dory.AppImage`): las actualizaciones reemplazan ese mismo fichero, así que el lanzador, el arranque con la sesión y `dory open` siguen apuntando a la misma ruta. Con una versión en el nombre, el actualizador lo renombraría en cada versión.

Genera AppImage en Linux (el único formato que se actualiza solo), .dmg en macOS e instalador NSIS en Windows. Las releases de GitHub las publican ya hechas.

## App de Android

La app está en `mobile/`. Encuentra los nodos de tu red local, y por Tailscale los de la malla de un nodo ya emparejado, y abre directamente el que elijas. No necesita un servidor central (ADR-014).

```sh
cd mobile && npm ci && npx cap sync android
cd android && JAVA_HOME=/usr/lib/jvm/java-21-openjdk ./gradlew assembleDebug
adb install app/build/outputs/apk/debug/app-debug.apk
```

Gradle necesita JDK 17 o 21, y el SDK de Android en `ANDROID_HOME` o en `mobile/android/local.properties`.

Los cambios en `mobile/` ejecutan la compilación Android independiente. En `main` publica una prerelease `android-<ejecución>-<intento>` con `dory-android.apk`; las PR dejan la APK como artefacto. Un cambio exclusivo en `web/` no ejecuta esta compilación: la interfaz del nodo se carga desde el servidor.

Para instalar sobre una app existente, Android exige el mismo identificador y certificado de firma. No se ha comprobado la clave de la APK instalada. Sin configurar firma, CI genera una clave debug nueva en cada ejecución: esas APK no se pueden actualizar entre sí ni se puede garantizar que actualicen la instalada. Para conservar la firma configura los secretos `ANDROID_KEYSTORE_BASE64` (almacén codificado en base64), `ANDROID_STORE_PASSWORD`, `ANDROID_KEY_ALIAS` y `ANDROID_KEY_PASSWORD` con la clave de la instalación anterior. Si no se dispone de ella, instalar la APK con otra firma requiere desinstalar primero la app y se perderán sus datos y emparejamientos. La APK no se actualiza automáticamente.

El WebView solicita el permiso de micrófono o cámara de Android cuando una página del nodo emparejado lo necesita; la denegación se respeta. La cámara queda preparada para una futura función de fotos. Android permite modificar estos permisos en los ajustes de la aplicación.

Para emparejar, abre en el ordenador **Entornos → Mostrar código** y escribe el código en la app, en el nodo que aparece o con su dirección. Cada nodo se empareja por separado. Los móviles emparejados aparecen en Entornos y se pueden revocar desde ahí.

La interfaz también funciona como PWA si publicas un nodo por HTTPS (`tailscale serve --bg --https=443 http://127.0.0.1:7700`, con `public_origin` y `cookie_secure = true`).

## Versiones

- **Estable:** cada etiqueta `v*` publica cada plataforma al terminar su compilación. La release completa para clientes anteriores aparece cuando terminan todas.
- **Nightly:** cada push a `main` publica cada plataforma al terminar. La release `nightly` completa queda como versión de transición para clientes anteriores; el actualizador nuevo consulta la última release de su plataforma. El enlace de instalación `nightly` instala esa transición y después se puede ejecutar `dory update`.
- **Pull requests:** se comprueban el núcleo y la interfaz. Los cambios de Android generan además una APK como artefacto.

Una plataforma puede ofrecer una versión más reciente que otra mientras se compila; si una falla, conserva la última publicación válida. La primera sustitución de la release `nightly` compartida debe realizarse sin actualizaciones simultáneas de clientes antiguos: su actualizador no puede garantizar coherencia mientras cambia esa release. Las descargas de las nuevas publicaciones por plataforma usan releases inmutables.

Un ordenador apagado durante la transición conserva su actualizador anterior. Cuando vuelva a conectarse, la release `nightly` completa le ofrece la versión de transición aunque ya existan publicaciones posteriores por plataforma. Después de instalarla y reiniciar, vuelve a comprobar las actualizaciones y aplica la más reciente que se ofrezca para su plataforma. No hace falta que todos los ordenadores estén encendidos al mismo tiempo. La actualización del núcleo se inicia desde Entornos o con `dory update`; encender el equipo no la aplica por sí solo.
