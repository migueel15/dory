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

## Qué se replica

- **Se replica:** proyectos, repositorios, tareas, dependencias, planes, memorias, skills y su estado por nodo, y la organización de los hilos (título, fijado, leído, tarea).
- **Se queda en cada nodo:** la ruta local de cada repositorio, las cuentas y tokens, las credenciales de los agentes, la conversación y el worktree de cada hilo, y las suscripciones a avisos.

Un cambio llega al otro nodo en menos de 2 s si están conectados. Sin conexión, los cambios esperan y convergen al reconectar:
- Gana el último cambio de cada campo.
- En las memorias, si dos nodos editaron el texto a la vez, se conservan las dos versiones.

## Revocar

En **Entornos**, **Revocar** deja de sincronizar con ese nodo.
