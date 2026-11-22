# ADR-028: Skills del sistema empaquetadas en el binario

- Status: accepted
- Date: 2026-11-22

## Context

El flujo de trabajo con Dory se explicaba en skills que vivían en un repositorio personal. Había que copiarlas o enlazarlas a mano en cada agente y en cada nodo, y quedaban desfasadas cuando cambiaba el MCP. Cualquier persona que instale Dory debe tener ese flujo sin configurar nada.

## Decision

- Dory embebe en el binario las skills que explican su uso (`internal/skills/system`): `dory-workflow` y `single-test-run`. Las skills personales, como las convenciones de GitHub, siguen fuera.
- Cada skill del sistema tiene un id fijo (`system-<nombre>`) y una `version` en su frontmatter que crece con cada cambio; `system.lock.json` y su test impiden cambiar el contenido sin subirla. El catálogo guarda la versión embebida en `system_version`. Al reconciliar, cada nodo la escribe si falta, si el catálogo tiene un `system_version` menor, o si con el mismo `system_version` el contenido o el nombre difieren. Un nodo con un binario antiguo nunca la degrada, así que la malla converge a la versión más nueva.
- Un binario anterior a esta decisión no conoce `system_version`: si edita una skill del sistema, el siguiente nodo actualizado restaura el contenido embebido. Si una skill de usuario ya tenía el nombre, se renombra a `<nombre>-custom` como fork; si estaba activa, sigue activa y la del sistema queda desactivada.
- Un nodo que recibió operaciones de columnas que su binario no conocía recupera sus valores del log al migrar.
- Son de solo lectura: no se editan ni se archivan, y su nombre queda reservado. Se pueden desactivar y volver a activar con `disabled_at`, que se replica y aplica a toda la malla.
- Para personalizarlas se hace un fork: una skill normal con `forked_from` que desactiva la original. Una original y sus forks nunca están activos a la vez: activar un fork desactiva la original, y la original solo se activa con sus forks desactivados.
- La instalación en las carpetas de cada arnés reutiliza el reconciliador de skills, con sus reglas: no pisa ficheros ni enlaces que no escribió Dory.

## Consequences

- Cambiar una skill del sistema exige subir su `version`; si no, los nodos que ya la tienen no la actualizan.
- Quitar una skill del sistema del binario no la borra del catálogo: un nodo antiguo la volvería a sembrar. Para retirarla hay que publicar una versión que la deje desactivada.
- Un usuario que tenía enlaces propios con el mismo nombre ve un conflicto hasta que los quita.
- Mientras quede un nodo con un binario anterior, ese nodo no respeta la desactivación ni los forks: instala todas las skills del catálogo. La malla se lleva a la misma versión con la actualización de ADR-016.
- Las ediciones concurrentes de una skill de usuario en dos nodos se resuelven campo a campo por reloj: gana la última y no se avisa del conflicto.
