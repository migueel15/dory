package net.dorlab.dory;

import android.annotation.SuppressLint;
import android.content.Context;
import android.media.AudioFormat;
import android.media.AudioRecord;
import android.media.MediaRecorder;
import android.os.Handler;
import android.os.Looper;
import com.k2fsa.sherpa.onnx.OnlineModelConfig;
import com.k2fsa.sherpa.onnx.OnlineRecognizer;
import com.k2fsa.sherpa.onnx.OnlineRecognizerConfig;
import com.k2fsa.sherpa.onnx.OnlineStream;
import com.k2fsa.sherpa.onnx.OnlineTransducerModelConfig;
import java.io.File;

/** Prototipo local: el modelo debe instalarse por separado en el almacenamiento privado. */
final class LocalVoiceRecognizer {
    interface Listener {
        void ready();
        void text(String value, boolean finished);
        void level(float value);
        void error();
    }

    private final Handler main = new Handler(Looper.getMainLooper());
    private final File model;
    private final Listener listener;
    private volatile boolean active = true;
    private volatile AudioRecord audio;

    static boolean available(Context context) {
        File dir = new File(context.getFilesDir(), "voice-model");
        for (String name : new String[] {"encoder.onnx", "decoder.onnx", "joiner.onnx", "tokens.txt"}) {
            if (!new File(dir, name).isFile()) return false;
        }
        return true;
    }

    LocalVoiceRecognizer(Context context, Listener listener) {
        this.model = new File(context.getFilesDir(), "voice-model");
        this.listener = listener;
    }

    void start() {
        new Thread(this::run, "dory-local-voice").start();
    }

    void stop() {
        active = false;
        AudioRecord current = audio;
        if (current != null) {
            try { current.stop(); } catch (IllegalStateException ignored) {}
        }
    }

    private void post(Runnable action) {
        main.post(() -> { if (active) action.run(); });
    }

    @SuppressLint("MissingPermission")
    private void run() {
        OnlineRecognizer recognizer = null;
        OnlineStream stream = null;
        AudioRecord record = null;
        try {
            OnlineTransducerModelConfig transducer = new OnlineTransducerModelConfig();
            transducer.setEncoder(new File(model, "encoder.onnx").getAbsolutePath());
            transducer.setDecoder(new File(model, "decoder.onnx").getAbsolutePath());
            transducer.setJoiner(new File(model, "joiner.onnx").getAbsolutePath());
            OnlineModelConfig modelConfig = new OnlineModelConfig();
            modelConfig.setTransducer(transducer);
            modelConfig.setTokens(new File(model, "tokens.txt").getAbsolutePath());
            modelConfig.setNumThreads(2);
            OnlineRecognizerConfig config = new OnlineRecognizerConfig();
            config.setModelConfig(modelConfig);
            config.setEnableEndpoint(true);
            recognizer = new OnlineRecognizer(null, config);
            stream = recognizer.createStream("");
            int min = AudioRecord.getMinBufferSize(16000, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT);
            if (min <= 0) throw new IllegalStateException("Audio buffer unavailable");
            record = new AudioRecord(MediaRecorder.AudioSource.VOICE_RECOGNITION, 16000,
                AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, Math.max(min, 6400));
            if (record.getState() != AudioRecord.STATE_INITIALIZED) throw new IllegalStateException("Microphone unavailable");
            audio = record;
            if (!active) return;
            record.startRecording();
            post(listener::ready);
            short[] buffer = new short[1600];
            while (active) {
                int n = record.read(buffer, 0, buffer.length);
                if (n <= 0) {
                    if (active) throw new IllegalStateException("Microphone read failed");
                    break;
                }
                float[] samples = new float[n];
                double sum = 0;
                for (int i = 0; i < n; i++) {
                    samples[i] = buffer[i] / 32768f;
                    sum += samples[i] * samples[i];
                }
                float level = (float) Math.min(1, Math.sqrt(sum / n) * 12);
                post(() -> listener.level(level));
                stream.acceptWaveform(samples, 16000);
                while (recognizer.isReady(stream)) recognizer.decode(stream);
                String text = recognizer.getResult(stream).getText().trim();
                boolean endpoint = recognizer.isEndpoint(stream);
                post(() -> listener.text(text, endpoint));
                if (endpoint) recognizer.reset(stream);
            }
        } catch (Exception | LinkageError e) {
            post(listener::error);
        } finally {
            audio = null;
            if (record != null) {
                try { record.stop(); } catch (IllegalStateException ignored) {}
                record.release();
            }
            if (stream != null) stream.release();
            if (recognizer != null) recognizer.release();
        }
    }
}
