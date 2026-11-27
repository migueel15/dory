# ADR-046: Adopción de tareas externas

- Status: accepted
- Date: 2026-11-27

## Context

#359 convirtió cada issue abierta de GitHub en una fila replicada de `tasks`, con un ID derivado del repositorio y del número. Esto duplicaba el trabajo: la tarea interna y la issue que el flujo crea para ella aparecían como dos tarjetas. Además, la migración cambiaba el ID de una tarea al darle una issue, y en los nodos sin caché aparecían filas vacías o invisibles. La caché externa es local y parcial, así que no puede decidir qué tareas existen.

## Decision

- Los elementos de un proveedor externo viven solo en `external_tasks_cache`, local a cada nodo y sin replicar. El tablero los muestra como sincronizados y la sincronización nunca crea filas en `tasks`.
- Un elemento pasa a ser tarea de Dory solo al **adoptarlo**, con una operación única de la capa de aplicación. La invocan: planificar, lanzar, cambiar estado o etapa, vincular a un hilo o a una dependencia, o la petición de un agente o del usuario.
- La identidad externa de una tarea es `source` (proveedor) + `external_id`, el identificador estable del proveedor (en GitHub, el `node_id` de la issue), con las mismas columnas para todos los proveedores. Repo, número y URL son datos de presentación.
- Al adoptar, el ID de la tarea se deriva de `(source, external_id)`; adoptar dos veces o desde dos nodos da la misma tarea. Una tarea interna que recibe una issue creada por Dory, o cerrada por una PR de sus hilos, conserva su ID y guarda la clave.
- La vista oculta los elementos de caché cuya clave ya pertenece a una tarea.

## Consequences

- Desaparecen los duplicados entre tareas internas y sus issues, y entre nodos.
- Una issue no adoptada solo existe en los nodos con acceso al proveedor; es lo esperado.
- Las filas `source = github` creadas por #359 (sin `external_id`) se archivan al arrancar, sin migrar nada. Sus hilos y planes siguen apuntando a las tareas archivadas; si la issue sigue abierta, vuelve a verse como sincronizada y es una tarea nueva al adoptarla.
- No está validado que GitHub conserve el `node_id` al transferir una issue a otro repositorio. Si no lo conserva, la issue transferida se verá como un elemento nuevo y la tarea anterior quedará con su último estado conocido.
- Si dos tareas internas distintas reciben la misma clave a la vez en nodos distintos, ambas la conservan hasta que se resuelva a mano; en un mismo nodo la segunda se rechaza como conflicto.
- Cada proveedor nuevo implementa la caché, la clave estable y el enlace (vista interna o URL); el tablero y la adopción no cambian.
