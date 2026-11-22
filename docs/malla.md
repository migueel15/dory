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

La malla no compara versiones al sincronizar: un nodo desactualizado sigue recibiendo y enviando cambios. Cada nodo interpreta los datos con el código de su binario:

- Guarda en su registro todas las operaciones que recibe, pero al escribir sus tablas solo usa las columnas que conoce su binario. Ignora las demás.
- Aplica las reglas de su versión. Por ejemplo, un nodo anterior a las skills del sistema (ADR-028) instala también las skills desactivadas y los forks, y deja editar las skills del sistema; los nodos actualizados restauran después su contenido.

Al actualizarse, el nodo se corrige solo: la migración recupera del registro los valores de las columnas nuevas que había recibido, las skills del sistema vuelven a su contenido y el reconciliador reinstala lo que corresponde en las carpetas de los agentes. Para acortar ese periodo, actualiza toda la malla a la vez desde **Entornos**.

Queda un caso sin aviso: si mientras el nodo estaba desactualizado se editó a mano el fichero de una skill que ahora debe quitarse, Dory no lo borra y deja de seguirlo, sin marcarlo como conflicto.

### Añadir una columna replicada

Dory no reconstruye las filas a partir del registro. Al añadir una columna a una entidad replicada (la lista de `internal/oplog/apply.go`):

1. Añade la columna en una migración.
2. En la misma migración, recupera su valor de las operaciones ya recibidas: para cada fila, el último valor por `hlc` de esa clave en `ops`. Distingue la clave ausente (`json_type(...) IS NULL`) de un `null` explícito. `00022_system_skills.sql` es el ejemplo.
3. Si la columna cambia qué hace un nodo (instalar, mostrar, permitir), escribe qué hará mientras tanto un nodo antiguo y cómo se corrige al actualizarse.

Sin el paso 2, un nodo que recibió esos cambios antes de actualizarse no los ve hasta que llegue otra operación sobre la misma fila.

## Revocar

En **Entornos**, **Revocar** deja de sincronizar con ese nodo.
