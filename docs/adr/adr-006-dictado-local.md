# ADR-006: Transcripción local del dictado en el servidor

- Status: accepted
- Date: 2026-11-07

## Context

El usuario quiere dictar en Dory desde el móvil y el escritorio, en español. La API de reconocimiento de voz del navegador no es fiable en todos los navegadores y envía el audio a terceros. infra-admin (i7-8700, sin GPU) ya ejecuta Dory y T3 Code.

## Decision

- La transcripción la hace `whisper-server` de whisper.cpp en infra-admin, en `127.0.0.1:8178`, con `language=es`, 4 hilos y `Nice=5` (unit `whisper-server.service`). Dory lo llama por `DORY_WHISPER_URL`; sin ella, la API de dictado responde 503.
- Modelo `small` q5_1. Medido en infra-admin, `large-v3-turbo` q5_0 tarda más que el audio en lotes de 10 s (factor ≈1,5); `small` va a ≈0,35.
- El cliente graba a 16 kHz mono y sube lotes WAV autónomos cortos (se cortan en las pausas, 6 s como máximo, con 0,5 s de solape solo en los cortes forzados y sin silencios largos) a `/api/dictation/{id}/chunks?seq=n`. Un worker por sesión los transcribe en orden y une el texto quitando las palabras repetidas por el solape. Cada lote lleva como `prompt` las últimas 30 palabras ya transcritas; los lotes sin voz se descartan en cliente y servidor. Así la espera al parar tras una pausa baja a ~1–2 s.
- Las sesiones viven en memoria, son del usuario que las crea y caducan a los 30 min. No se guarda audio.
- `finish` con `insert` espera al último lote y devuelve el texto. Con `send` responde al momento y el backend entrega el texto al terminar: `capture` crea un item en inbox como la captura web y `define` escribe en el hilo de definición del item.

## Consequences

- No hay dependencias externas ni coste por uso, y el audio no sale de la máquina.
- La transcripción compite por CPU con Dory y T3; con varios dictados a la vez la cola puede crecer (máximo 32 lotes por sesión).
- Un reinicio de Dory pierde las sesiones en curso.
