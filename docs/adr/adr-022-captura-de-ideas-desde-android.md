# ADR-022: Captura de ideas desde el asistente de Android

- Status: superseded by ADR-023
- Date: 2026-11-20

## Contexto

El selector de entornos de Dory vive en una WebView. La captura invocada por el asistente necesita guardar ideas aunque la WebView esté cerrada.

## Decisión

- El lanzador sincroniza los nodos ya emparejados y el destino de captura con las preferencias privadas de Android. La copia se actualiza al abrir el lanzador, emparejar, olvidar o cambiar la dirección de un nodo.
- La captura nativa usa el único nodo emparejado o el destino elegido explícitamente, con su token y la clave fijada en el emparejamiento.
- El reconocedor de Android produce el texto y la app reutiliza `POST /api/ideas`, con el mismo identificador para reintentos de la misma idea.

## Consecuencias

La primera apertura del lanzador después de actualizar hace disponibles los emparejamientos anteriores para el asistente. La captura usa la red para guardar en el nodo y puede conservar el texto en pantalla si el envío falla.
