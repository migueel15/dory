# ADR-026: Release compartida por ejecución

- Status: accepted
- Date: 2026-11-22

## Context

ADR-021 publicaba una release por plataforma y ejecución, de modo que cada compilación nightly aparecía en GitHub como tres releases sueltas. Las PR empaquetaban además las tres plataformas aunque no tocaran el escritorio, y la publicación completa para clientes antiguos ya no tenía uso.

## Decision

Cada ejecución publicable crea una única release: `nightly-<ejecución>` en nightly y la etiqueta `v*` en estable. Cada plataforma sube su núcleo y sus archivos de escritorio a esa release al terminar y, después, avanza su manifiesto en `updates-<canal>-<sistema>`, como en ADR-021. Una plataforma rápida queda disponible sin esperar a las demás, y un fallo en una no retrasa las otras.

Los clientes descargan solo los archivos de su sistema: el núcleo `dory-<sistema>-<arquitectura>` y el `latest*.yml` propio de electron-updater.

CI decide qué ejecutar según los archivos modificados. Las PR ejecutan las pruebas cuando cambian el código Go o la web y empaquetan solo si cambian el escritorio, los scripts, las dependencias Go, archivos Go específicos de macOS o Windows, o el propio workflow. En `main` se publica una nightly cuando cambia código que se distribuye; las etiquetas y las ejecuciones manuales empaquetan siempre. La web se compila una vez y se comparte entre trabajos.

Se elimina la publicación completa para clientes antiguos: la release de transición `nightly` ya está congelada y no existen releases estables anteriores.

Android sigue siendo independiente, como en ADR-021.

## Consequences

GitHub muestra una release por compilación y sus archivos aparecen a medida que termina cada plataforma. Una release puede estar incompleta mientras otra plataforma compila o si ha fallado; los clientes no se ven afectados porque solo siguen su manifiesto. Las releases `build-*` anteriores siguen disponibles para los manifiestos que aún las identifican. El resto de ADR-021 continúa vigente.
