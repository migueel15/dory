package net.dorlab.dory;

import android.annotation.SuppressLint;
import android.content.Context;
import android.media.AudioFormat;
import android.media.AudioRecord;
import android.media.MediaRecorder;
import android.os.Handler;
import android.os.Looper;
import com.k2fsa.sherpa.onnx.OfflineModelConfig;
import com.k2fsa.sherpa.onnx.OfflineRecognizer;
import com.k2fsa.sherpa.onnx.OfflineRecognizerConfig;
import com.k2fsa.sherpa.onnx.OfflineStream;
import com.k2fsa.sherpa.onnx.OfflineTransducerModelConfig;
import java.io.File;

/** Records the whole idea and transcribes it with Parakeet once the user stops; no live text. */
final class ParakeetCapture implements LocalCapture {
    private static final int RATE = 16000;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Context context;
    private final MoonshineCapture.Listener listener;
    private volatile boolean stopping;
    private volatile AudioRecord audio;

    ParakeetCapture(Context context, MoonshineCapture.Listener listener) {
        this.context = context.getApplicationContext();
        this.listener = listener;
    }

    @Override public void start() { new Thread(this::run, "parakeet-capture").start(); }

    @Override public void stop() {
        stopping = true;
        AudioRecord current = audio;
        if (current != null) {
            try { current.stop(); } catch (IllegalStateException ignored) {}
        }
    }

    @SuppressLint("MissingPermission")
    private void run() {
        AudioRecord record = null;
        OfflineRecognizer engine = null;
        boolean failed = false;
        try {
            engine = recognizer(AssistantModels.directory(context, AssistantModels.PARAKEET));
            if (stopping) return;
            int min = AudioRecord.getMinBufferSize(RATE, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT);
            if (min <= 0) throw new IllegalStateException("Micrófono no disponible");
            record = new AudioRecord(MediaRecorder.AudioSource.VOICE_RECOGNITION, RATE,
                AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, Math.max(min, 6400));
            if (record.getState() != AudioRecord.STATE_INITIALIZED) throw new IllegalStateException("Micrófono no disponible");
            audio = record;
            if (stopping) return;
            record.startRecording();
            main.post(listener::ready);
            float[] samples = new float[RATE * 30];
            int length = 0;
            short[] buffer = new short[1600];
            while (!stopping) {
                int n = record.read(buffer, 0, buffer.length);
                if (n <= 0) {
                    if (!stopping) throw new IllegalStateException("No se pudo leer el micrófono");
                    break;
                }
                if (length + n > samples.length) samples = java.util.Arrays.copyOf(samples, samples.length * 2);
                double sum = 0;
                for (int i = 0; i < n; i++) {
                    float v = buffer[i] / 32768f;
                    samples[length + i] = v;
                    sum += v * v;
                }
                length += n;
                float level = (float) Math.min(1, Math.sqrt(sum / n) * 12);
                main.post(() -> listener.level(level));
            }
            if (length < RATE / 4) return;
            OfflineStream stream = engine.createStream();
            try {
                stream.acceptWaveform(java.util.Arrays.copyOf(samples, length), RATE);
                engine.decode(stream);
                String text = engine.getResult(stream).getText().trim();
                main.post(() -> listener.text(text, true));
            } finally {
                stream.release();
            }
        } catch (Exception | LinkageError e) {
            failed = true;
        } finally {
            audio = null;
            if (record != null) {
                try { record.stop(); } catch (IllegalStateException ignored) {}
                record.release();
            }
            if (engine != null) engine.release();
            boolean error = failed;
            main.post(() -> { if (error) listener.error(); else listener.finished(); });
        }
    }

    private static OfflineRecognizer recognizer(File dir) {
        OfflineTransducerModelConfig transducer = new OfflineTransducerModelConfig();
        transducer.setEncoder(new File(dir, "encoder.int8.onnx").getPath());
        transducer.setDecoder(new File(dir, "decoder.int8.onnx").getPath());
        transducer.setJoiner(new File(dir, "joiner.int8.onnx").getPath());
        OfflineModelConfig model = new OfflineModelConfig();
        model.setTransducer(transducer);
        model.setTokens(new File(dir, "tokens.txt").getPath());
        model.setModelType("nemo_transducer");
        model.setNumThreads(Math.min(4, Runtime.getRuntime().availableProcessors()));
        OfflineRecognizerConfig config = new OfflineRecognizerConfig();
        config.setModelConfig(model);
        return new OfflineRecognizer(null, config);
    }
}
