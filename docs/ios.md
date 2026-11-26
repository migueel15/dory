# App de iPhone

La app de iPhone está en `mobile/ios`. Hace lo mismo que la de Android: encuentra los nodos de la red local y, por Tailscale, los de la malla de un nodo emparejado, se empareja con el código de Entornos y abre la interfaz móvil del nodo dentro de la app. Está hecha en Swift (SwiftUI) y no necesita Mac ni cuenta de pago de Apple (ADR-046).

## Descargar

Cada cambio en `mobile/ios` ejecuta el workflow `ios` en GitHub Actions:

- En `main` publica una prerelease `ios-<ejecución>-<intento>` con `dory-ios.ipa`.
- En una PR deja el IPA como artefacto `dory-ios-ipa` de la ejecución.

El IPA va sin firmar: lo firma en el iPhone SideStore o AltStore con tu Apple ID.

## Instalar con SideStore

Solo hace falta un ordenador (Linux, Windows o Mac) la primera vez.

1. Instala SideStore en el iPhone siguiendo su guía oficial (<https://sidestore.io>). Durante la instalación se genera el archivo de emparejamiento del iPhone y se inicia sesión con tu Apple ID.
2. En el iPhone activa **Ajustes → Privacidad y seguridad → Modo de desarrollador** (iOS 16 o posterior) y reinicia cuando lo pida.
3. Descarga `dory-ios.ipa` de la release en el iPhone (Safari → Descargas).
4. Abre SideStore → **My Apps → +** y elige el IPA. SideStore lo firma e instala Dory con su icono.
5. Con la VPN local de SideStore activa (StosVPN o WireGuard, según su guía), SideStore renueva la firma antes de los 7 días sin conectar el iPhone al ordenador. Si caduca, abre SideStore y pulsa **Refresh All**.

Con AltStore el proceso es igual, pero renueva la firma por Wi-Fi cuando AltServer está abierto en el ordenador.

Una cuenta gratuita admite tres apps firmadas a la vez, contando SideStore. Para actualizar Dory, instala el IPA nuevo de la misma forma: se conservan los nodos emparejados.

## Primer uso

- La primera vez iOS pide permiso para **buscar en la red local**: acéptalo o la app no verá los nodos de casa.
- Para emparejar, abre en el ordenador **Entornos → Mostrar código**. En la app toca el entorno disponible o **Emparejar con una dirección** e introduce el código.
- Toca un entorno emparejado para abrir su interfaz. Dentro, desliza desde el borde izquierdo para volver atrás; en la primera página vuelves a la lista.
- Desliza hacia abajo en la lista para comprobar de nuevo los entornos. Los tokens se guardan en el llavero del iPhone.

## Dictar ideas

iOS no deja cambiar el asistente del sistema. En su lugar, Dory añade dos atajos que aparecen solos en Siri y en la app Atajos:

- **Dictar idea** abre la pantalla de dictado: el botón central pausa o sigue, el avión envía y la X descarta. Una pausa al hablar no envía el texto; al salir, el borrador se conserva.
- **Guardar idea** pide la idea a Siri y la guarda sin abrir la app.

Di «Oye Siri, dictar idea en Dory» o «Guardar idea en Dory». También puedes asignar «Dictar idea» al botón de acción (**Ajustes → Botón de acción → Atajo**), al centro de control o a **Accesibilidad → Tocar → Tocar atrás**. Con varios entornos, elige el **Nodo de captura** en los ajustes de la app. El reconocedor es el de Apple; con **Reconocer sin conexión** el audio no sale del iPhone.

## Avisos

Sin cuenta de pago no hay push. Con la app cerrada, iOS deja a Dory comprobar los avisos de un nodo emparejado cada cierto tiempo (15 minutos como mínimo, según el uso y la batería) y muestra una notificación por cada aviso nuevo sin leer, con las mismas reglas que Android: nada para los tipos en *Solo en la app* o *Desactivado*, ni en *Automático* si otra interfaz de la malla tiene el foco. Tocar la notificación abre el nodo en la página del aviso. Con la app abierta, los avisos salen en la interfaz del nodo. Se desactivan en **Ajustes → Avisos con la app cerrada**; **Ajustes → Actualizar en segundo plano** de iOS debe estar activo para Dory.

## Compilar

`mobile/ios/project.yml` describe el proyecto para XcodeGen; el `.xcodeproj` no se guarda en el repositorio. En un Mac:

```sh
brew install xcodegen
cd mobile/ios && xcodegen generate && open Dory.xcodeproj
```
