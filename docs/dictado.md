# Dictado

El micrófono del compositor requiere HTTPS o localhost. El navegador entrega el audio al nodo Dory abierto, donde el modelo se ejecuta localmente. En desarrollo, si el nodo está en este ordenador, el audio se procesa aquí.

## Modelos del compositor

No hace falta instalar nada. Al descargar un modelo, si el nodo no tiene motor, Dory descarga [NeMo-Speech.cpp](https://github.com/NVIDIA/NeMo-Speech.cpp/releases) v0.3.0 (variante CPU) para su sistema, verifica su SHA-256 y lo guarda en `dictation-models/runtime/`. Hay motor para Linux y macOS (x86_64 y arm64) y Windows x86_64.

Dory busca el motor en cada uso, en este orden: `DORY_NEMO_SPEECH_BIN` si está fijada, el motor descargado y `nemo-speech` en el PATH. Para usar otra variante, como CUDA o Vulkan, instálala y fija `DORY_NEMO_SPEECH_BIN` a su ruta absoluta.

En **Ajustes → Dictado local**, descarga el modelo y marca **Usar modelo**. La preferencia se guarda en el navegador y las descargas en `dictation-models/` dentro del directorio de datos del nodo.

- **Parakeet TDT 0.6B v3:** inserta el texto al detener la grabación.
- **Nemotron Streaming 3.5 0.6B:** inserta una vista previa mientras hablas y completa la transcripción al detener la grabación.

Pulsa el micrófono en el inicio, dentro de un hilo o al redactar una respuesta a una cita. También está disponible en comentarios de revisión y campos de texto libre como ideas, descripciones y planes. Al terminar puedes editar el texto antes de enviarlo o guardarlo.

Un tono breve ascendente indica que el micrófono ya está grabando. Un tono descendente indica que ha dejado de grabar y empieza a finalizar la transcripción.

## Atajo de dictado

`Ctrl+D` inicia o detiene el dictado en el editor activo. Puedes cambiarlo en **Ajustes → Dictado local**.

- Si seleccionas texto con un hilo abierto y pulsas el atajo, se abre la respuesta a la cita y empieza el dictado. `Enter` termina el dictado y añade la cita con su respuesta al borrador.
- En el cuadro de mensaje, `Ctrl+D` deja el texto listo para editar. Si pulsas `Enter` mientras dictas, Dory espera al texto final y envía el mensaje.

## Motor Whisper para captura

El flujo de captura existente transcribe mediante whisper.cpp según el ADR-006.

## Puesta en marcha del motor en infra-admin

```sh
sudo apt-get install -y cmake
mkdir -p ~/opt && cd ~/opt
git clone --depth 1 https://github.com/ggml-org/whisper.cpp && cd whisper.cpp
cmake -B build -DCMAKE_BUILD_TYPE=Release -DGGML_NATIVE=ON
cmake --build build -j6 --target whisper-server
sh ./models/download-ggml-model.sh small-q5_1
sudo cp ~/repos/dory/deploy/whisper-server.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now whisper-server
```

Añade a `~/.config/dory/dory.env` y reinicia Dory:

```sh
DORY_WHISPER_URL=http://127.0.0.1:8178
```

Comprobación: `curl 127.0.0.1:8178/inference -F file=@audio.wav -F language=es -F response_format=json`.

## API

- `POST /api/dictation` crea una sesión.
- `POST /api/dictation/{id}/chunks?seq=n` sube un lote WAV (16 kHz mono, máx. 2 MiB), en orden desde 0.
- `GET /api/dictation/{id}` devuelve el texto parcial y los lotes pendientes.
- `POST /api/dictation/{id}/finish` con `{"mode":"insert"}` espera y devuelve el texto; con `{"mode":"send","target":{"kind":"capture","project_id":"…"}}` o `{"kind":"thread","thread_id":"…"}` el backend lo entrega al terminar: crea una tarea en el inbox o envía el texto al hilo.

## Troceado

- El cliente mide la energía (RMS) en marcos de 100 ms; por debajo de 0,01 es silencio.
- Un lote se envía tras ~1 s de silencio si lleva ≥1,5 s de voz; desde 4 s, en el primer marco silencioso; a los 6 s, en el marco más callado, con 0,5 s de solape con el siguiente.
- Antes de subirlo, cada silencio de más de 0,6 s se acorta a 0,3 s. Un lote sin voz no se sube.
- El servidor también ignora un WAV sin voz: avanza `seq` pero no lo transcribe.
- Cada lote se transcribe con `prompt` = últimas 30 palabras del texto de la sesión.
- El parcial se consulta cada segundo.
