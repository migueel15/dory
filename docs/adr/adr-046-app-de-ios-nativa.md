# ADR-046: App de iOS nativa sin cuenta de pago

- Status: accepted
- Date: 2026-11-26

## Context

ADR-014 dejó el móvil solo en Android. Para iPhone no hay Mac ni cuenta de Apple Developer (99 €/año). Con un Apple ID gratuito solo se pueden instalar apps propias firmadas por SideStore o AltStore, que las renuevan cada 7 días; esa firma no permite push (APNs) ni el permiso de multicast que necesitaría oír la baliza UDP. iOS tampoco deja sustituir a Siri como asistente.

## Decision

- **App nativa en `mobile/ios`:** Swift y SwiftUI, sin Capacitor. La lista de entornos, el emparejado, los ajustes y el dictado son vistas nativas; solo la interfaz del nodo es web, en un `WKWebView`.
- **Mismo contrato que Android:** `/device/v1/pair` con la prueba HMAC, `/device/v1/session` para la cookie y pin SHA-256 de la clave pública ECDSA en `URLSession` y en el `WKWebView`. El servidor no cambia.
- **Descubrimiento:** Bonjour `_dory._tcp`, que los nodos ya anuncian, en lugar de la baliza UDP; las direcciones de Tailscale llegan por `/api/mesh`, como en Android.
- **Dictado:** reconocedor de voz de Apple en una pantalla nativa, abierta con el atajo «Dictar idea» (Siri, botón de acción, Tocar atrás) o guardada sin abrir la app con «Guardar idea». Sin Moonshine.
- **Avisos:** notificaciones locales tras comprobar `/api/notices` cuando iOS concede tiempo en segundo plano (`BGAppRefreshTask`).
- **Compilación:** `project.yml` de XcodeGen; el workflow `ios` genera el proyecto en macOS y publica `dory-ios.ipa` sin firmar como prerelease `ios-<ejecución>-<intento>`.

## Consequences

- Los avisos con la app cerrada llegan con retraso (15 minutos como mínimo, a criterio de iOS) y no en tiempo real como en Android.
- Los tokens se guardan en el llavero de iOS, no en el almacenamiento de la app.
- La app hay que renovarla cada 7 días; SideStore lo hace solo desde el iPhone. Las actualizaciones se instalan a mano desde la release.
- El selector y los ajustes existen dos veces (página de Capacitor en Android, SwiftUI en iOS): un cambio de comportamiento del selector debe hacerse en ambas.
