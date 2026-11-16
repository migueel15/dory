# Dictado

Dory transcribe el dictado en el servidor con whisper.cpp (ADR-006). Necesita HTTPS (`public_origin`) para que el navegador dé permiso al micrófono.

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
