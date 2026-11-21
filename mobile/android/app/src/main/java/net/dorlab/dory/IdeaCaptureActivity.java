package net.dorlab.dory;

import android.Manifest;
import android.app.KeyguardManager;
import android.app.role.RoleManager;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.graphics.Typeface;
import android.content.res.ColorStateList;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.media.AudioManager;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.speech.RecognitionListener;
import android.speech.RecognizerIntent;
import android.speech.SpeechRecognizer;
import android.view.Gravity;
import android.view.View;
import android.view.animation.DecelerateInterpolator;
import android.widget.FrameLayout;
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
    private enum State { STARTING, LISTENING, PAUSED, SPEECH_ERROR, SAVING, SAVE_ERROR, SAVED }

    private static final int ACCENT = 0xff93c5fd;
    private static final int FOREGROUND = 0xffe5e7eb;
    private static final int MUTED = 0xffaab7c8;
    private static final int GOOD = 0xff4ade80;
    private static final int BAD = 0xfff87171;
    private static final int INK = 0xff0b1220;
    private final Handler main = new Handler(Looper.getMainLooper());
    private static final long DRAFT_LIFETIME = 12L * 60 * 60 * 1000;
    private String ideaId = UUID.randomUUID().toString();
    private boolean resumeOnStart;
    private boolean ended;
    private SpeechRecognizer recognizer;
    private boolean usingOnDevice;
    private boolean systemFallback;
    private State state = State.STARTING;
    private String committed = "";
    private String partial = "";
    private final Runnable relisten = () -> {
        if (!isFinishing() && (state == State.LISTENING || state == State.STARTING)) listen();
    };
    private String destinationId;
    private TextView status;
    private TextView transcript;
    private FrameLayout frame;
    private VoiceBorderView border;
    private VoiceLevelView level;
    private VoiceIconButton primaryIcon;
    private TextView primaryText;
    private VoiceIconButton close;
    private TextView send;
    private boolean silenced;

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
        restoreDraft();
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
        if (state != State.LISTENING) display(State.STARTING, "Activando micrófono…", null, null);
        try {
            SpeechRecognizer session = usingOnDevice
                ? SpeechRecognizer.createOnDeviceSpeechRecognizer(this) : SpeechRecognizer.createSpeechRecognizer(this);
            recognizer = session;
            session.setRecognitionListener(new RecognitionListener() {
                private boolean current() {
                    return recognizer == session && !isFinishing();
                }

                @Override public void onReadyForSpeech(Bundle params) {
                    if (current() && state == State.STARTING) display(State.LISTENING, "Escuchando…", null, null);
                }
                @Override public void onPartialResults(Bundle results) {
                    if (!current()) return;
                    partial = bestText(results);
                    transcript.setText(join(committed, partial));
                    refreshSend();
                    if (state == State.STARTING) display(State.LISTENING, "Escuchando…", null, null);
                }
                @Override public void onResults(Bundle results) {
                    if (current()) keepListening(bestText(results));
                }
                @Override public void onError(int error) {
                    if (!current()) return;
                    if (error == SpeechRecognizer.ERROR_NO_MATCH || error == SpeechRecognizer.ERROR_SPEECH_TIMEOUT) {
                        keepListening(partial);
                    } else if (usingOnDevice && partial.isEmpty() && (error == SpeechRecognizer.ERROR_LANGUAGE_NOT_SUPPORTED
                        || error == SpeechRecognizer.ERROR_LANGUAGE_UNAVAILABLE)) {
                        systemFallback = true;
                        listen();
                    } else {
                        closeRecognizer();
                        committed = join(committed, partial);
                        partial = "";
                        transcript.setText(committed);
                        display(State.SPEECH_ERROR, "No se pudo completar el dictado.", "Continuar", IdeaCaptureActivity.this::requestMicrophone);
                    }
                }
                @Override public void onBeginningOfSpeech() {}
                @Override public void onEndOfSpeech() {}
                @Override public void onRmsChanged(float rms) {
                    if (!current()) return;
                    float value = Math.max(0f, Math.min(1f, (rms + 2f) / 12f));
                    border.setLevel(value);
                    level.setLevel(value);
                }
                @Override public void onBufferReceived(byte[] buffer) {}
                @Override public void onEvent(int eventType, Bundle params) {}
            });
            Intent dictation = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH)
                .putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
                .putExtra(RecognizerIntent.EXTRA_LANGUAGE, "es-ES")
                .putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
                .putExtra(RecognizerIntent.EXTRA_SPEECH_INPUT_COMPLETE_SILENCE_LENGTH_MILLIS, 30000L)
                .putExtra(RecognizerIntent.EXTRA_SPEECH_INPUT_POSSIBLY_COMPLETE_SILENCE_LENGTH_MILLIS, 30000L);
            session.startListening(dictation);
        } catch (RuntimeException e) {
            closeRecognizer();
            display(State.SPEECH_ERROR, "No se pudo iniciar el reconocedor.", "Reintentar", this::requestMicrophone);
        }
    }

    /** Silencia los pitidos del reconocedor mientras se dicta, para que reiniciarlo tras cada pausa no suene. */
    private void silence(boolean on) {
        if (silenced == on) return;
        AudioManager audio = getSystemService(AudioManager.class);
        if (audio == null) return;
        silenced = on;
        int direction = on ? AudioManager.ADJUST_MUTE : AudioManager.ADJUST_UNMUTE;
        for (int stream : new int[] {AudioManager.STREAM_MUSIC, AudioManager.STREAM_SYSTEM, AudioManager.STREAM_NOTIFICATION}) {
            try {
                audio.adjustStreamVolume(stream, direction, 0);
            } catch (SecurityException ignored) {
            }
        }
    }

    /** Una pausa al hablar no termina la idea: se conserva lo dictado y se vuelve a escuchar. */
    private void keepListening(String text) {
        closeRecognizer();
        committed = join(committed, text.isEmpty() ? partial : text);
        partial = "";
        transcript.setText(committed);
        refreshSend();
        saveDraft();
        main.postDelayed(relisten, 120);
    }

    /** El dictado en curso sobrevive a salir con Inicio o Atrás: solo Enviar o la X lo terminan. */
    private SharedPreferences drafts() {
        return getSharedPreferences("idea-draft", MODE_PRIVATE);
    }

    private void saveDraft() {
        String text = join(committed, partial);
        if (ended || text.isEmpty()) return;
        drafts().edit().putString("text", text).putString("id", ideaId).putLong("at", System.currentTimeMillis()).apply();
    }

    private void clearDraft() {
        ended = true;
        drafts().edit().clear().apply();
    }

    private void restoreDraft() {
        SharedPreferences saved = drafts();
        String text = saved.getString("text", "");
        if (text.isEmpty() || System.currentTimeMillis() - saved.getLong("at", 0) > DRAFT_LIFETIME) {
            saved.edit().clear().apply();
            return;
        }
        committed = text;
        ideaId = saved.getString("id", ideaId);
        transcript.setText(committed);
        refreshSend();
    }

    private void sendIdea() {
        if (state == State.SAVING || state == State.SAVED) return;
        String text = join(committed, partial);
        if (text.isEmpty()) return;
        closeRecognizer();
        committed = text;
        partial = "";
        save();
    }

    private void refreshSend() {
        boolean ready = !join(committed, partial).isEmpty();
        send.setEnabled(ready);
        send.setAlpha(ready ? 1f : 0.35f);
    }

    private void closeRecognizer() {
        main.removeCallbacks(relisten);
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
                    clearDraft();
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
        if (state == State.LISTENING || state == State.STARTING) {
            closeRecognizer();
            committed = join(committed, partial);
            partial = "";
            transcript.setText(committed);
            resumeOnStart = true;
            display(State.PAUSED, "En pausa", "Continuar", this::requestMicrophone);
        }
        saveDraft();
    }

    @Override
    protected void onStart() {
        super.onStart();
        if (resumeOnStart && state == State.PAUSED) {
            resumeOnStart = false;
            requestMicrophone();
        }
    }

    @Override
    protected void onDestroy() {
        silence(false);
        closeRecognizer();
        main.removeCallbacksAndMessages(null);
        super.onDestroy();
    }

    private void display(State next, String message, String action, Runnable onAction) {
        state = next;
        status.setText(message);
        boolean listening = next == State.LISTENING || next == State.STARTING;
        boolean failed = next == State.SPEECH_ERROR || next == State.SAVE_ERROR;
        int tint = next == State.SAVED ? GOOD : failed ? BAD : listening || next == State.SAVING ? ACCENT : MUTED;
        status.setTextColor(tint);
        silence(listening);
        level.setColor(tint);
        level.setLive(next == State.LISTENING);
        border.setMood(next == State.SAVED ? VoiceBorderView.Mood.DONE
            : failed ? VoiceBorderView.Mood.ERROR
            : next == State.PAUSED ? VoiceBorderView.Mood.IDLE : VoiceBorderView.Mood.ACTIVE);
        View.OnClickListener click = onAction == null ? null : view -> onAction.run();
        boolean resume = "Continuar".equals(action);
        primaryIcon.setVisibility(resume ? View.VISIBLE : View.GONE);
        if (resume) primaryIcon.setIcon(VoiceIconButton.Icon.PLAY, action);
        primaryIcon.setOnClickListener(click);
        primaryText.setVisibility(action == null || resume ? View.GONE : View.VISIBLE);
        primaryText.setText(action);
        primaryText.setOnClickListener(click);
        boolean dictating = listening || next == State.PAUSED || next == State.SPEECH_ERROR;
        send.setVisibility(dictating ? View.VISIBLE : View.GONE);
        refreshSend();
        close.setVisibility(next == State.SAVING || next == State.SAVED ? View.GONE : View.VISIBLE);
    }

    private void createView() {
        WindowCompat.setDecorFitsSystemWindows(getWindow(), false);
        frame = new FrameLayout(this);
        border = new VoiceBorderView(this);
        frame.addView(border, new FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(22), dp(20), dp(16), dp(16));
        panel.setBackground(shape(0xf2101318, 30, 0x2effffff, 1));
        FrameLayout.LayoutParams panelSize = new FrameLayout.LayoutParams(
            FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT, Gravity.BOTTOM | Gravity.CENTER_HORIZONTAL);
        frame.addView(panel, panelSize);
        ViewCompat.setOnApplyWindowInsetsListener(frame, (view, windowInsets) -> {
            Insets insets = windowInsets.getInsets(WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.displayCutout());
            panelSize.setMargins(insets.left + dp(22), insets.top + dp(22), insets.right + dp(22), insets.bottom + dp(24));
            panel.setLayoutParams(panelSize);
            return windowInsets;
        });
        transcript = label("", 21, FOREGROUND);
        transcript.setHint("Cuéntame tu idea…");
        transcript.setHintTextColor(0x66aab7c8);
        transcript.setMinLines(2);
        transcript.setMaxLines(6);
        transcript.setLineSpacing(0, 1.1f);
        transcript.setGravity(Gravity.BOTTOM);
        transcript.setMovementMethod(android.text.method.ScrollingMovementMethod.getInstance());
        panel.addView(transcript);
        LinearLayout controls = new LinearLayout(this);
        controls.setGravity(Gravity.CENTER_VERTICAL);
        LinearLayout.LayoutParams controlsSize = new LinearLayout.LayoutParams(
            LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        controlsSize.topMargin = dp(18);
        panel.addView(controls, controlsSize);
        level = new VoiceLevelView(this);
        controls.addView(level);
        status = label("", 14, MUTED);
        status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        status.setPadding(dp(12), 0, dp(8), 0);
        controls.addView(status, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));
        primaryText = label("", 14, INK);
        primaryText.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        primaryText.setGravity(Gravity.CENTER);
        primaryText.setPadding(dp(18), dp(11), dp(18), dp(11));
        primaryText.setBackground(shape(ACCENT, 24, ACCENT, 0));
        primaryText.setForeground(ripple());
        primaryText.setClickable(true);
        LinearLayout.LayoutParams gap = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        gap.setMarginEnd(dp(10));
        controls.addView(primaryText, gap);
        primaryIcon = new VoiceIconButton(this, 52, ACCENT, INK);
        controls.addView(primaryIcon, gap);
        close = new VoiceIconButton(this, 44, 0x1fffffff, FOREGROUND);
        close.setIcon(VoiceIconButton.Icon.CLOSE, "Cancelar");
        close.setOnClickListener(view -> {
            clearDraft();
            finish();
        });
        controls.addView(close);
        send = label("Enviar idea", 18, INK);
        send.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        send.setGravity(Gravity.CENTER);
        send.setBackground(shape(ACCENT, 30, ACCENT, 0));
        RippleDrawable sendRipple = ripple();
        sendRipple.setRadius(dp(200));
        send.setForeground(sendRipple);
        send.setClickable(true);
        send.setOnClickListener(view -> sendIdea());
        LinearLayout.LayoutParams sendSize = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(64));
        sendSize.topMargin = dp(14);
        sendSize.setMarginEnd(dp(6));
        panel.addView(send, sendSize);
        setContentView(frame);
        panel.setAlpha(0f);
        panel.setTranslationY(dp(36));
        panel.animate().alpha(1f).translationY(0f).setDuration(320).setInterpolator(new DecelerateInterpolator()).start();
        display(State.STARTING, "Preparando dictado…", null, null);
    }

    private RippleDrawable ripple() {
        GradientDrawable mask = new GradientDrawable();
        mask.setCornerRadius(dp(24));
        mask.setColor(0xffffffff);
        return new RippleDrawable(ColorStateList.valueOf(0x33000000), null, mask);
    }

    private TextView label(String text, int size, int color) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextSize(size);
        view.setTextColor(color);
        return view;
    }

    private GradientDrawable shape(int background, int radius, int stroke, int strokeDp) {
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(background);
        shape.setCornerRadius(dp(radius));
        if (strokeDp > 0) shape.setStroke(dp(strokeDp), stroke);
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
