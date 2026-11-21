package net.dorlab.dory;

import android.annotation.SuppressLint;
import android.content.Context;
import android.media.AudioFormat;
import android.media.AudioRecord;
import android.media.MediaRecorder;
import android.os.Handler;
import android.os.Looper;
import ai.moonshine.voice.Transcriber;
import ai.moonshine.voice.TranscriptEvent;

/** Runs capture and inference on one worker so the final audio is flushed before sending. */
final class MoonshineCapture {
    interface Listener {
        void ready();
        void text(String value, boolean complete);
        void level(float value);
        void finished();
        void error();
    }
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Context context;
    private final String model;
    private final Listener listener;
    private volatile boolean stopping;
    private volatile AudioRecord audio;

    MoonshineCapture(Context context, String model, Listener listener) {
        this.context = context.getApplicationContext();
        this.model = model;
        this.listener = listener;
    }

    void start() { new Thread(this::run, "moonshine-capture").start(); }

    void stop() {
        stopping = true;
        AudioRecord current = audio;
        if (current != null) {
            try { current.stop(); } catch (IllegalStateException ignored) {}
        }
    }

    @SuppressLint("MissingPermission")
    private void run() {
        Transcriber engine = new Transcriber();
        AudioRecord record = null;
        boolean started = false;
        boolean failed = false;
        try {
            engine.addListener(event -> {
                if (event instanceof TranscriptEvent.LineTextChanged) {
                    String value = ((TranscriptEvent.LineTextChanged) event).line.text;
                    main.post(() -> listener.text(value == null ? "" : value, false));
                } else if (event instanceof TranscriptEvent.LineCompleted) {
                    String value = ((TranscriptEvent.LineCompleted) event).line.text;
                    main.post(() -> listener.text(value == null ? "" : value, true));
                }
            });
            engine.loadFromFiles(AssistantModels.directory(context, model).getAbsolutePath(), AssistantModels.arch(model));
            if (stopping) return;
            int min = AudioRecord.getMinBufferSize(16000, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT);
            if (min <= 0) throw new IllegalStateException("Micrófono no disponible");
            record = new AudioRecord(MediaRecorder.AudioSource.VOICE_RECOGNITION, 16000,
                AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, Math.max(min, 6400));
            if (record.getState() != AudioRecord.STATE_INITIALIZED) throw new IllegalStateException("Micrófono no disponible");
            audio = record;
            if (stopping) return;
            engine.start();
            started = true;
            record.startRecording();
            main.post(listener::ready);
            short[] buffer = new short[1600];
            while (!stopping) {
                int n = record.read(buffer, 0, buffer.length);
                if (n <= 0) {
                    if (!stopping) throw new IllegalStateException("No se pudo leer el micrófono");
                    break;
                }
                float[] samples = new float[n];
                double sum = 0;
                for (int i = 0; i < n; i++) {
                    samples[i] = buffer[i] / 32768f;
                    sum += samples[i] * samples[i];
                }
                float level = (float) Math.min(1, Math.sqrt(sum / n) * 12);
                main.post(() -> listener.level(level));
                engine.addAudio(samples, 16000);
            }
        } catch (Exception | LinkageError e) {
            failed = true;
        } finally {
            audio = null;
            if (record != null) {
                try { record.stop(); } catch (IllegalStateException ignored) {}
                record.release();
            }
            if (started) {
                try { engine.stop(); } catch (Exception e) { failed = true; }
            }
            engine.close();
            boolean error = failed;
            main.post(() -> { if (error) listener.error(); else listener.finished(); });
        }
    }
}
