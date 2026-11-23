package net.dorlab.dory;

import android.content.Context;
import ai.moonshine.voice.AssetDownloader;
import ai.moonshine.voice.JNI;
import ai.moonshine.voice.ModelSpec;
import java.io.File;

final class AssistantModels {
    static final String ANDROID = "android";
    static final String TINY = "tiny";
    static final String SMALL = "small";
    static final String PARAKEET = "parakeet";
    /** Parakeet TDT 0.6B v3 int8 exported for sherpa-onnx. */
    static final String PARAKEET_URL = "https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8/resolve/main/";
    static final String[] PARAKEET_FILES = {"tokens.txt", "decoder.int8.onnx", "joiner.int8.onnx", "encoder.int8.onnx"};

    static boolean valid(String id) {
        return ANDROID.equals(id) || TINY.equals(id) || SMALL.equals(id) || PARAKEET.equals(id);
    }

    static boolean moonshine(String id) {
        return TINY.equals(id) || SMALL.equals(id);
    }

    static int arch(String id) {
        if (TINY.equals(id)) return JNI.MOONSHINE_MODEL_ARCH_TINY_STREAMING;
        if (SMALL.equals(id)) return JNI.MOONSHINE_MODEL_ARCH_SMALL_STREAMING;
        throw new IllegalArgumentException("Modelo desconocido");
    }

    static File directory(Context context, String id) {
        return new File(context.getFilesDir(), "assistant-models/" + id);
    }

    static ModelSpec spec(String id) {
        return ModelSpec.stt("es", arch(id), false);
    }

    static boolean installed(Context context, String id) {
        if (ANDROID.equals(id)) return true;
        if (android.os.Build.VERSION.SDK_INT < 26) return false;
        if (PARAKEET.equals(id)) {
            for (String name : PARAKEET_FILES) if (!new File(directory(context, id), name).isFile()) return false;
            return true;
        }
        return new AssetDownloader().isModelPresent(directory(context, id), spec(id));
    }

    static String selected(Context context) {
        String id = context.getSharedPreferences("assistant-model", Context.MODE_PRIVATE).getString("selected", ANDROID);
        return valid(id) && (ANDROID.equals(id) || android.os.Build.VERSION.SDK_INT >= 26) ? id : ANDROID;
    }

    static void select(Context context, String id) {
        if (!valid(id) || !installed(context, id)) throw new IllegalArgumentException("Descarga primero el modelo.");
        context.getSharedPreferences("assistant-model", Context.MODE_PRIVATE).edit().putString("selected", id).apply();
    }
}
