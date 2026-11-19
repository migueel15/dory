# ADR-020: El plan de una tarea es un documento Markdown editable

- Status: accepted
- Date: 2026-11-19

## Context

Hasta ahora, cada propuesta del agente con `propose_plan` era una versión cerrada de un plan con secciones fijas (ADR-004). El usuario quiere que el plan sea un documento vivo de la tarea. Debe poder editarlo él o cualquier hilo vinculado a la tarea, desde cualquier nodo de la malla, viendo en directo cómo lo escribe el agente.

## Decision

- La tarea es la fuente de verdad. Su plan es un único documento Markdown (`plans.body_md`) que se edita en el sitio. Los hilos solo referencian la tarea; cualquier hilo de la tarea, o el usuario, puede editar el plan.
- `plans.revision` cuenta los guardados. Una escritura que indica la revisión de la que parte se rechaza si el plan ya cambió (HTTP 409 `plan_conflict`), de modo que nunca se pisa una edición ajena. El nodo serializa los guardados.
- Las herramientas MCP `plan_read`, `plan_write` y `plan_edit` sustituyen a `propose_plan`. `plan_edit` sustituye un fragmento exacto que debe aparecer una sola vez, para que el agente haga cambios pequeños que el usuario ve aparecer en su sitio.
- El plan se ve y se edita en la pestaña «Plan» del panel lateral del hilo y en el detalle de la tarea. Se refresca con las actualizaciones en vivo y la pestaña se abre sola cuando el agente cambia el plan.
- Si el usuario tiene texto sin guardar cuando llega otra revisión, el editor fusiona las dos ediciones por líneas a partir de la última versión guardada (fusión de tres vías, como git) y guarda el resultado. Si ambos cambiaron la misma línea, se conservan las dos versiones: la del usuario y, debajo, la otra citada con su etiqueta. `plan_write` no puede reescribir un plan existente sin la revisión leída; los cambios del agente van con `plan_edit`, que se aplica sobre la versión guardada en ese momento.
- Solo el usuario acepta el plan. Cambiar un plan aceptado lo deja otra vez pendiente de aceptar.
- Los planes anteriores, con secciones, se muestran convertidos a Markdown y pasan a guardarse como documento en su primera edición.

## Consequences

- Sustituye la parte del ADR-004 sobre versiones cerradas, secciones fijas y `propose_plan`. Se mantiene que solo el usuario acepta el plan.
- Las columnas nuevas se replican con el registro de operaciones; los nodos antiguos las ignoran. Dos ediciones simultáneas en nodos distintos se resuelven por última escritura del documento completo.
- Se edita con un editor visual (TipTap) que guarda Markdown; los agentes siguen leyendo y escribiendo Markdown.
