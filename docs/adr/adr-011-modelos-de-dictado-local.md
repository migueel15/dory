# ADR-011: Modelos de dictado local para el compositor

- Status: accepted
- Date: 2026-11-17

## Context

El flujo por lotes de Whisper del ADR-006 ya sirve para la captura. En el compositor hay que poder alternar entre modelos locales y mostrar los parciales de los que admiten streaming.

## Decision

Dory instala artefactos GGUF de revisiones fijas y verifica su tamaño y SHA-256. Si el nodo no tiene NeMo-Speech.cpp, lo descarga junto al primer modelo: la release fija de la variante CPU para su sistema, con tamaño y SHA-256 verificados. Busca el motor en cada uso y lo lanza en loopback. El navegador accede al motor a través de la API autenticada de Dory. Parakeet recibe una grabación WAV al terminar. Nemotron recibe PCM por WebSocket y devuelve parciales de su propia sesión de streaming.

## Consequences

El modelo seleccionado se recuerda por navegador y los pesos quedan en el nodo que transcribe. El borrador se puede editar al terminar, antes de enviarlo al agente. La API por lotes del ADR-006 sigue disponible para captura.
