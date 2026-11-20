package net.dorlab.dory;

import android.Manifest;
import android.app.KeyguardManager;
import android.app.role.RoleManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.speech.RecognitionListener;
import android.speech.RecognizerIntent;
import android.speech.SpeechRecognizer;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;
import androidx.activity.OnBackPressedCallback;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import java.util.ArrayList;
import java.util.UUID;
import org.json.JSONObject;

public class IdeaCaptureActivity extends AppCompatActivity {
    private enum State { STARTING, LISTENING, PAUSING, PAUSED, SPEECH_ERROR, SAVING, SAVE_ERROR, SAVED }

    private static final int ACCENT = 0xff93c5fd;
    private static final int FOREGROUND = 0xffe5e7eb;
    private static final int MUTED = 0xffaab7c8;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final String ideaId = UUID.randomUUID().toString();
    private SpeechRecognizer recognizer;
    private boolean usingOnDevice;
    private boolean systemFallback;
    private State state = State.STARTING;
    private String committed = "";
    private String partial = "";
    private final Runnable finishPause = () -> finishSpeech(partial);
    private String destinationId;
    private TextView status;
    private TextView transcript;
    private FrameLayout frame;
    private ImageView mic;
    private Button primary;
    private Button cancel;

    private final ActivityResultLauncher<String> microphonePermission =
        registerForActivityResult(new ActivityResultContracts.RequestPermission(), granted -> {
            if (isFinishing()) return;
            if (granted) listen();
            else display(State.SPEECH_ERROR, "Permite el micrófono para dictar una idea.", "Continuar", this::requestMicrophone);
        });

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        KeyguardManager keyguard = getSystemService(KeyguardManager.class);
        if (!Intent.ACTION_ASSIST.equals(getIntent().getAction()) || keyguard.isDeviceLocked() || !canAssist()) {
            finish();
            return;
        }
        createView();
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                if (state != State.SAVING) finish();
            }
        });
        try {
            destinationId = CaptureNodes.destination(this, null).getString("id");
        } catch (Exception e) {
            display(State.SPEECH_ERROR, e.getMessage(), "Abrir Dory", () -> {
                startActivity(new Intent(this, MainActivity.class));
                finish();
            });
            return;
        }
        frame.post(this::requestMicrophone);
    }

    private boolean canAssist() {
        if (Build.VERSION.SDK_INT < 29) return true;
        RoleManager roles = getSystemService(RoleManager.class);
        return roles != null && roles.isRoleHeld(RoleManager.ROLE_ASSISTANT);
    }

    private void requestMicrophone() {
        if (isFinishing()) return;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
            listen();
        } else {
            microphonePermission.launch(Manifest.permission.RECORD_AUDIO);
        }
    }

    private void listen() {
        if (isFinishing()) return;
        closeRecognizer();
        usingOnDevice = !systemFallback && Build.VERSION.SDK_INT >= 31
            && SpeechRecognizer.isOnDeviceRecognitionAvailable(this);
        if (!usingOnDevice && !SpeechRecognizer.isRecognitionAvailable(this)) {
            display(State.SPEECH_ERROR, "No hay un reconocedor de voz disponible.", "Reintentar", this::listen);
            return;
        }
        display(State.STARTING, "Activando micrófono…", null, null);
        try {
            SpeechRecognizer session = usingOnDevice
                ? SpeechRecognizer.createOnDeviceSpeechRecognizer(this) : SpeechRecognizer.createSpeechRecognizer(this);
            recognizer = session;
            session.setRecognitionListener(new RecognitionListener() {
                private boolean current() {
                    return recognizer == session && !isFinishing();
                }

                @Override public void onReadyForSpeech(Bundle params) {
                    if (current() && state == State.STARTING) {
                        display(State.LISTENING, "Escuchando…", "Pausar", IdeaCaptureActivity.this::pause);
                    }
                }
                @Override public void onPartialResults(Bundle results) {
                    if (!current()) return;
                    partial = bestText(results);
                    transcript.setText(join(committed, partial));
                    if (state == State.STARTING) display(State.LISTENING, "Escuchando…", "Pausar", IdeaCaptureActivity.this::pause);
                }
                @Override public void onResults(Bundle results) {
                    if (current()) finishSpeech(bestText(results));
                }
                @Override public void onError(int error) {
                    if (!current()) return;
                    if (state == State.PAUSING) {
                        finishSpeech(partial);
                    } else if (usingOnDevice && partial.isEmpty() && (error == SpeechRecognizer.ERROR_LANGUAGE_NOT_SUPPORTED
                        || error == SpeechRecognizer.ERROR_LANGUAGE_UNAVAILABLE)) {
                        systemFallback = true;
                        listen();
                    } else {
                        closeRecognizer();
                        committed = join(committed, partial);
                        partial = "";
                        transcript.setText(committed);
                        String message = error == SpeechRecognizer.ERROR_NO_MATCH || error == SpeechRecognizer.ERROR_SPEECH_TIMEOUT
                            ? "No se detectó voz." : "No se pudo completar el dictado.";
                        display(State.SPEECH_ERROR, message, "Continuar", IdeaCaptureActivity.this::requestMicrophone);
                    }
                }
                @Override public void onBeginningOfSpeech() {}
                @Override public void onEndOfSpeech() {}
                @Override public void onRmsChanged(float rms) {}
                @Override public void onBufferReceived(byte[] buffer) {}
                @Override public void onEvent(int eventType, Bundle params) {}
            });
            Intent dictation = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH)
                .putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
                .putExtra(RecognizerIntent.EXTRA_LANGUAGE, "es-ES")
                .putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true);
            session.startListening(dictation);
        } catch (RuntimeException e) {
            closeRecognizer();
            display(State.SPEECH_ERROR, "No se pudo iniciar el reconocedor.", "Reintentar", this::requestMicrophone);
        }
    }

    private void pause() {
        display(State.PAUSING, "Pausando…", null, null);
        main.postDelayed(finishPause, 1600);
        recognizer.stopListening();
    }

    private void finishSpeech(String text) {
        boolean paused = state == State.PAUSING;
        closeRecognizer();
        if (paused && text.isEmpty()) text = partial;
        partial = "";
        committed = join(committed, text);
        transcript.setText(committed);
        if (paused) {
            display(State.PAUSED, "En pausa", "Continuar", this::requestMicrophone);
        } else if (text.isEmpty()) {
            display(State.SPEECH_ERROR, "No se detectó voz.", "Continuar", this::requestMicrophone);
        } else {
            save();
        }
    }

    private void closeRecognizer() {
        main.removeCallbacks(finishPause);
        SpeechRecognizer session = recognizer;
        recognizer = null;
        if (session != null) session.destroy();
    }

    private void save() {
        display(State.SAVING, "Guardando…", null, null);
        String text = committed;
        new Thread(() -> {
            try {
                JSONObject node = CaptureNodes.destination(getApplicationContext(), destinationId);
                CaptureNodes.sendIdea(node, ideaId, text);
                main.post(() -> {
                    if (isFinishing() || isDestroyed()) return;
                    display(State.SAVED, "Idea guardada", null, null);
                    main.postDelayed(this::finish, 1000);
                });
            } catch (Exception e) {
                main.post(() -> {
                    if (isFinishing() || isDestroyed()) return;
                    String detail = e instanceof java.io.IOException ? "Comprueba la conexión al nodo." : e.getMessage();
                    display(State.SAVE_ERROR, "No se pudo guardar. " + (detail == null ? "" : detail), "Reintentar", this::save);
                });
            }
        }, "dory-save-idea").start();
    }

    @Override
    protected void onStop() {
        super.onStop();
        if (recognizer != null) {
            closeRecognizer();
            committed = join(committed, partial);
            partial = "";
            display(State.PAUSED, "En pausa", "Continuar", this::requestMicrophone);
            transcript.setText(committed);
        }
    }

    @Override
    protected void onDestroy() {
        closeRecognizer();
        main.removeCallbacksAndMessages(null);
        super.onDestroy();
    }

    private void display(State next, String message, String action, Runnable onAction) {
        state = next;
        status.setText(message);
        boolean listening = next == State.LISTENING || next == State.STARTING;
        frame.setBackground(shape(Color.TRANSPARENT, 24, listening ? ACCENT : MUTED));
        mic.setColorFilter(listening ? ACCENT : MUTED);
        primary.setVisibility(action == null ? View.GONE : View.VISIBLE);
        primary.setText(action);
        primary.setOnClickListener(onAction == null ? null : view -> onAction.run());
        cancel.setVisibility(next == State.SAVING || next == State.SAVED ? View.GONE : View.VISIBLE);
    }

    private void createView() {
        WindowCompat.setDecorFitsSystemWindows(getWindow(), false);
        frame = new FrameLayout(this);
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(18), dp(16), dp(18), dp(10));
        panel.setBackground(shape(0xff15181d, 24, 0xff354354));
        FrameLayout.LayoutParams panelSize = new FrameLayout.LayoutParams(
            FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT, Gravity.BOTTOM | Gravity.CENTER_HORIZONTAL);
        frame.addView(panel, panelSize);
        ViewCompat.setOnApplyWindowInsetsListener(frame, (view, windowInsets) -> {
            Insets insets = windowInsets.getInsets(WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.displayCutout());
            panelSize.setMargins(insets.left + dp(12), insets.top + dp(12), insets.right + dp(12), insets.bottom + dp(12));
            panel.setLayoutParams(panelSize);
            return windowInsets;
        });
        LinearLayout heading = new LinearLayout(this);
        heading.setGravity(Gravity.CENTER_VERTICAL);
        mic = new ImageView(this);
        mic.setImageResource(android.R.drawable.ic_btn_speak_now);
        mic.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        heading.addView(mic, new LinearLayout.LayoutParams(dp(28), dp(28)));
        TextView title = label("Guarda tu idea", 20, FOREGROUND);
        title.setPadding(dp(8), 0, 0, 0);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        heading.addView(title);
        panel.addView(heading);
        status = label("", 14, MUTED);
        status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        panel.addView(status);
        transcript = label("", 17, FOREGROUND);
        transcript.setMaxLines(5);
        transcript.setGravity(Gravity.BOTTOM);
        transcript.setMovementMethod(android.text.method.ScrollingMovementMethod.getInstance());
        panel.addView(transcript);
        LinearLayout actions = new LinearLayout(this);
        actions.setGravity(Gravity.END);
        cancel = new Button(this);
        cancel.setText("Cancelar");
        cancel.setOnClickListener(view -> finish());
        actions.addView(cancel);
        primary = new Button(this);
        actions.addView(primary);
        panel.addView(actions);
        setContentView(frame);
        display(State.STARTING, "Preparando dictado…", null, null);
    }

    private TextView label(String text, int size, int color) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextSize(size);
        view.setTextColor(color);
        return view;
    }

    private GradientDrawable shape(int background, int radius, int stroke) {
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(background);
        shape.setCornerRadius(dp(radius));
        shape.setStroke(dp(2), stroke);
        return shape;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private static String bestText(Bundle results) {
        ArrayList<String> alternatives = results == null ? null : results.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION);
        return alternatives == null || alternatives.isEmpty() ? "" : alternatives.get(0).trim();
    }

    private static String join(String first, String second) {
        if (first.isEmpty()) return second;
        if (second.isEmpty()) return first;
        return first + " " + second;
    }
}
