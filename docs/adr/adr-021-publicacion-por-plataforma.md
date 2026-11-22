# ADR-021: Publicación independiente por plataforma

- Status: parcialmente sustituido por ADR-026 (release compartida)
- Date: 2026-11-20

## Context

La actualización descrita en ADR-016 identificaba todas las plataformas con un solo commit y esperaba a todas sus compilaciones. Un fallo impedía publicar las demás.

## Decision

Cada plataforma y canal publica una release inmutable con el núcleo y los archivos de escritorio del mismo commit. Tras subirlos, CI cambia el manifiesto de esa plataforma en la rama `updates-<canal>-<sistema>`; el manifiesto identifica la release y el commit. Las publicaciones de una misma plataforma se serializan y una ejecución anterior no puede sustituir otra posterior. Los clientes nuevos consultan exclusivamente su manifiesto y descargan de la release identificada.

Para los clientes antiguos se conserva una release completa: en estable se publica al completar todas las plataformas y en nightly se publica una versión de transición y se conserva. Una instalación que ya incorpore el nuevo actualizador pasa a usar los manifiestos. La sustitución única de la release nightly anterior tiene la misma limitación del mecanismo antiguo: un cliente que actualice durante el cambio puede fallar o mezclar archivos. Esta ventana no afecta a los manifiestos independientes.

La distribución de Android es independiente y no incorpora la interfaz del servidor. Su firma debe persistir entre compilaciones para instalar actualizaciones sobre una APK existente.

## Consequences

Las plataformas pueden tener versiones distintas y conservan la última release válida si otra falla. Los binarios históricos y las releases de plataforma permanecen disponibles. Durante la transición nightly se requiere evitar actualizaciones simultáneas de clientes antiguos; los enlaces de la release nightly anterior pasan a apuntar a la versión de transición. El resto de decisiones de ADR-016 sobre actualización del núcleo, escritorio y malla continúa vigente.
