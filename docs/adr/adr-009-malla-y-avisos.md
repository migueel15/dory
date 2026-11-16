# ADR-009: Emparejamiento, sincronización y avisos entre nodos

- Status: accepted
- Date: 2026-11-16
- Supersedes: ADR-003

## Context

Los nodos de un mismo usuario tienen que encontrarse, confiar entre ellos y sincronizarse por la red local o por Tailscale. El móvil se conecta al servidor.

## Decision

- **Identidad:** cada nodo tiene una clave Ed25519 en `<datos>/node.key`.
- **Conexión:** los nodos se hablan por TLS 1.3 mutuo, fijado por la clave pública, en el puerto 7710.
- **Emparejamiento:** un nodo muestra un código de un solo uso válido 5 minutos y el otro lo escribe. Cada parte envía un HMAC del código sobre las dos claves, así un intermediario que no conoce el código no puede hacerse pasar por ninguna.
- **Descubrimiento:** sin dependencias nuevas, con avisos UDP multicast en la red local y `tailscale status` en la tailnet. Solo actualiza direcciones o propone nodos para emparejar.
- **Sincronización:** cada nodo tira de las operaciones que le faltan y empuja las suyas, así que basta con que una dirección sea alcanzable. Cada nodo mantiene un stream de avisos con el otro y sincroniza al momento.
- **Presencia:** un nodo pasa a desconectado tras 30 s sin contacto.
- **Avisos:** cada nodo envía Web Push (VAPID) a sus propias suscripciones. Como el estado de los hilos se replica, el servidor avisa al móvil de los hilos de cualquier máquina.

## Consequences

- Revocar un nodo corta su sincronización con este nodo. El nodo revocado no se entera hasta que lo intenta.
- Los avisos UDP no son mDNS estándar: otras herramientas no ven los nodos.
