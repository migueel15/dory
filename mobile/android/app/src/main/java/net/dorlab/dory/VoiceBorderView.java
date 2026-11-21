package net.dorlab.dory;

import android.animation.ArgbEvaluator;
import android.animation.ValueAnimator;
import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.RectF;
import android.graphics.SweepGradient;
import android.view.View;
import android.view.animation.LinearInterpolator;

/** Borde de pantalla completa con un degradado que gira y respira según el estado del dictado. */
public class VoiceBorderView extends View {
    public enum Mood {
        ACTIVE(new int[] {0xff60a5fa, 0xffa78bfa, 0xff22d3ee, 0xff60a5fa}, 1f, true),
        IDLE(new int[] {0xff64748b, 0xff94a3b8, 0xff64748b, 0xff94a3b8}, 0.45f, false),
        ERROR(new int[] {0xfff87171, 0xfffb923c, 0xfff87171, 0xfffb923c}, 0.9f, false),
        DONE(new int[] {0xff4ade80, 0xff22d3ee, 0xff4ade80, 0xff22d3ee}, 1f, false);

        final int[] colors;
        final float strength;
        final boolean spinning;

        Mood(int[] colors, float strength, boolean spinning) {
            this.colors = colors;
            this.strength = strength;
            this.spinning = spinning;
        }
    }

    private static final float RADIUS_DP = 38f;
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF rect = new RectF();
    private final Matrix rotate = new Matrix();
    private final int[] shown = Mood.IDLE.colors.clone();
    private final int[] from = Mood.IDLE.colors.clone();
    private final float density = getResources().getDisplayMetrics().density;
    private SweepGradient gradient;
    private Mood mood = Mood.IDLE;
    private float angle;
    private float mix = 1f;
    private float strength = Mood.IDLE.strength;
    private float strengthFrom = Mood.IDLE.strength;
    private float level;
    private float levelTarget;
    private float phase;
    private long lastFrame;
    private final ValueAnimator clock = ValueAnimator.ofFloat(0f, 1f);

    public VoiceBorderView(Context context) {
        super(context);
        paint.setStyle(Paint.Style.STROKE);
        paint.setStrokeCap(Paint.Cap.ROUND);
        clock.setDuration(1000);
        clock.setRepeatCount(ValueAnimator.INFINITE);
        clock.setInterpolator(new LinearInterpolator());
        clock.addUpdateListener(animation -> step());
        setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
    }

    public void setMood(Mood next) {
        if (next == mood) return;
        for (int i = 0; i < shown.length; i++) from[i] = shown[i];
        strengthFrom = strength;
        mood = next;
        mix = 0f;
        start();
    }

    /** Nivel de voz normalizado entre 0 y 1. */
    public void setLevel(float value) {
        levelTarget = Math.max(0f, Math.min(1f, value));
    }

    private void start() {
        if (!clock.isRunning() && ValueAnimator.areAnimatorsEnabled()) {
            lastFrame = 0;
            clock.start();
        } else if (!ValueAnimator.areAnimatorsEnabled()) {
            mix = 1f;
            step();
        }
    }

    private void step() {
        long now = System.nanoTime();
        float dt = lastFrame == 0 ? 0.016f : Math.min(0.05f, (now - lastFrame) / 1e9f);
        lastFrame = now;
        if (mood.spinning) angle = (angle + dt * 70f) % 360f;
        phase += dt * (mood == Mood.ACTIVE ? 2.4f : 1.2f);
        boolean mixing = mix < 1f;
        mix = Math.min(1f, mix + dt / 0.45f);
        level += (levelTarget - level) * Math.min(1f, dt * 10f);
        levelTarget *= Math.max(0f, 1f - dt * 3f);
        ArgbEvaluator evaluator = new ArgbEvaluator();
        for (int i = 0; i < shown.length; i++) shown[i] = (Integer) evaluator.evaluate(mix, from[i], mood.colors[i]);
        strength = strengthFrom + (mood.strength - strengthFrom) * mix;
        if (mixing) gradient = null;
        invalidate();
        boolean settled = mix >= 1f && !mood.spinning && mood != Mood.ERROR && mood != Mood.DONE && level < 0.01f;
        if (settled && clock.isRunning()) clock.cancel();
    }

    @Override
    protected void onDetachedFromWindow() {
        clock.cancel();
        super.onDetachedFromWindow();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        start();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float w = getWidth();
        float h = getHeight();
        if (gradient == null) gradient = new SweepGradient(w / 2f, h / 2f, shown.clone(), null);
        rotate.setRotate(angle, w / 2f, h / 2f);
        gradient.setLocalMatrix(rotate);
        paint.setShader(gradient);
        float breathe = 0.78f + 0.22f * (float) Math.sin(phase);
        float boost = 1f + level * 0.9f;
        float radius = RADIUS_DP * density;
        float[] widths = {16f, 10f, 5f, 2.2f};
        float[] alphas = {0.10f, 0.20f, 0.45f, 1f};
        float inset = 6f * density;
        rect.set(inset, inset, w - inset, h - inset);
        for (int i = 0; i < widths.length; i++) {
            float stroke = widths[i] * density * (i < 3 ? boost : 1f);
            paint.setStrokeWidth(stroke);
            paint.setAlpha(Math.round(255f * Math.min(1f, alphas[i] * strength * breathe)));
            canvas.drawRoundRect(rect, radius, radius, paint);
        }
    }
}
