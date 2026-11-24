# Malla de nodos

Cada nodo funciona solo y se sincroniza con los demás cuando se ven (ADR-007 y ADR-009).

## Emparejar dos nodos

1. En el nodo A, ve a **Entornos → Mostrar código**.
2. En el nodo B, escribe la dirección de A (IP de Tailscale o de la red local, con puerto si no es el 7710) y el código.

Sin interfaz (un servidor por SSH), usa la CLI del nodo:

- `dory pair` muestra un código, la huella y el puerto de la malla de este nodo.
- `dory pair DIRECCIÓN CÓDIGO` empareja este nodo con otro usando su código.

Puedes combinar las dos formas: muestra el código con `dory pair` en el servidor y escríbelo en Entornos de tu ordenador, o al revés.

El código vale 5 minutos y un solo uso. Desde entonces los dos nodos se reconocen por su clave. Comprueba que la huella que muestra cada uno coincide.

Los nodos que aparecen en «Nodos encontrados en la red» se ven por la red local o por Tailscale. Al pulsar uno se rellena su dirección.

## Emparejar el móvil

En la app de Android, el código de **Mostrar código** sirve igual que entre nodos. Mira [Instalación](instalacion.md#app-de-android).

## Elegir el nodo de un hilo

Al crear un hilo, el selector **Nodo** muestra este nodo y los nodos emparejados conectados. El hilo se ejecuta en el nodo elegido, con sus agentes y su copia del repositorio. Si el repositorio no está clonado en ese nodo, el hilo no se crea y aparece un error.

## Qué se replica

- **Se replica:** proyectos, repositorios, tareas, dependencias, planes, memorias, skills y su estado por nodo, la bandeja de avisos y sus ajustes, y la organización de los hilos (título, fijado, leído, tarea).
- **Se queda en cada nodo:** la ruta local de cada repositorio, las cuentas y tokens, las credenciales de los agentes, la conversación y el worktree de cada hilo, y las suscripciones a avisos.

Un cambio llega al otro nodo en menos de 2 s si están conectados. Sin conexión, los cambios esperan y convergen al reconectar:
- Gana el último cambio de cada campo.
- En las memorias, si dos nodos editaron el texto a la vez, se conservan las dos versiones.

## Nodos con versiones distintas

Los nodos anuncian su protocolo en `/peer/v2/info` antes de intercambiar operaciones. Si un par usa otro protocolo, Dory pausa la sincronización y muestra un aviso en **Entornos** y **Conexiones**. Actualiza Dory en ese nodo para reanudarla con el mismo emparejamiento. Los móviles se conectan mediante `/device/v1`.

Los nodos con el mismo protocolo pueden tener esquemas distintos:

- Cada nodo conserva y reenvía las operaciones que recibe, incluidas las de entidades desconocidas. En sus tablas materializa las entidades y columnas que conoce.
- Aplica las reglas de su versión. Por ejemplo, un nodo anterior a las skills del sistema (ADR-028) instala también las skills desactivadas y los forks, y deja editar las skills del sistema; los nodos actualizados restauran después su contenido.

Al actualizarse, el nodo se corrige solo: la migración recupera del registro los valores de las columnas nuevas que había recibido, las skills del sistema vuelven a su contenido y el reconciliador reinstala lo que corresponde en las carpetas de los agentes. Para acortar ese periodo, actualiza toda la malla a la vez desde **Entornos**.

Si mientras el nodo estaba desactualizado se editó a mano el fichero de una skill que ahora debe quitarse, Dory no lo borra: marca la skill en conflicto en ese nodo y desde **Skills** se retira la copia, que queda en una copia de seguridad.

### Añadir una columna replicada

Las migraciones recuperan del registro las operaciones ya recibidas. Al incorporar una entidad, su migración materializa sus filas a partir de esas operaciones. Para añadir una columna a una entidad replicada, sigue estos pasos.

1. Añade la columna en una migración.
2. En la misma migración, recupera su valor de las operaciones ya recibidas: para cada fila, el último valor por `hlc` de esa clave en `ops`. Distingue la clave ausente (`json_type(...) IS NULL`) de un `null` explícito. `00022_system_skills.sql` es el ejemplo.
3. Añade la columna al registro en `internal/schema` y al struct de la entidad en `internal/data/types.go`. Si el struct está expuesto a la web, ejecuta `go generate ./cmd/gen-ts` y guarda `web/src/api/types.gen.ts` junto con el cambio.
4. Si la columna cambia qué hace un nodo (instalar, mostrar, permitir), escribe qué hará mientras tanto un nodo antiguo y cómo se corrige al actualizarse.

Sin el paso 2, un nodo que recibió esos cambios antes de actualizarse no los ve hasta que llegue otra operación sobre la misma fila.

## Revocar

En **Entornos**, **Revocar** deja de sincronizar con ese nodo.
