# ADR-023: Modelos del asistente de Android

- Status: accepted
- Date: 2026-11-21
- Supersedes: ADR-022

## Contexto

El asistente debe capturar ideas aunque la WebView esté cerrada y permitir elegir entre el reconocimiento del sistema y modelos locales, sin incluir sus pesos en la aplicación.

## Decisión

El lanzador sincroniza los nodos emparejados y el destino de captura en preferencias privadas de Android. La captura usa el único nodo o el destino elegido, su token y la clave fijada; conserva el identificador al reintentar POST /api/ideas.

El usuario selecciona en el lanzador el reconocedor de Android o un modelo Moonshine para español descargado en el móvil. Android es el predeterminado. La selección y los archivos son locales al dispositivo; Moonshine requiere Android 8 o posterior.

## Consecuencias

El asistente usa la selección incluso con la WebView cerrada y Moonshine procesa la voz en el móvil. Descargar un modelo requiere red y almacenamiento. Guardar la idea sigue requiriendo conexión al nodo. En sistemas antiguos se mantiene el reconocedor de Android.
