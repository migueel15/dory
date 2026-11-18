# ADR-014: App de Android que se conecta directamente a cada nodo

- Status: accepted
- Date: 2026-11-18

## Context

No hay un nodo central: puede haber solo un portátil. El móvil tiene que encontrar los nodos de la red local o de la tailnet y abrir el que elijas, como el móvil de T3 Code. Antes solo podía usar la PWA de un servidor publicado con `tailscale serve`.

## Decision

- **El móvil es un cliente:** no guarda réplica ni ejecuta agentes. Es una app de Android hecha con Capacitor, en `mobile/`.
- **Transporte:** el móvil usa el puerto de la malla (7710). El certificado de cliente pasa a ser opcional y solo las rutas `/peer/v1` lo exigen. El resto del puerto sirve la interfaz y la API autenticada.
- **Certificado para móviles:** el WebView de Android no acepta Ed25519 en TLS, así que cada nodo tiene además una clave ECDSA P-256 en `<datos>/device.key`. Los nodos piden el ALPN `dory-peer` y reciben el certificado Ed25519; el resto recibe el ECDSA. La app fija el SHA-256 de la clave pública ECDSA.
- **Emparejar:** se usa el mismo código de Entornos. `POST /device/v1/pair` devuelve un token de dispositivo y un HMAC del código sobre la clave ECDSA, para que la app compruebe que nadie se ha puesto en medio. Tras 5 códigos erróneos, el código se anula.
- **Sesión:** cada vez que abres un nodo, `POST /device/v1/session` cambia el token por una cookie de sesión de 12 horas y la app carga `https://nodo:7710/` en el WebView.
- **Descubrir:** la app escucha la baliza UDP que ya existe en la red local. Para Tailscale, cada nodo emparejado le pasa los demás nodos de su malla con sus direcciones. También se puede escribir una dirección a mano.

## Consequences

- Hay que emparejar el móvil con cada nodo por separado: cada nodo tiene sus propias cuentas y tokens.
- Revocar un móvil en Entornos anula su token. La sesión que ya tenga abierta dura como mucho 12 horas.
- El token se guarda en el almacenamiento privado de la app, no en el almacén de claves de Android.
- No hay avisos push nativos ni versión de iOS.
