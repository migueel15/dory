package net.dorlab.dory;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;
import android.view.View;

/** Barras que laten al ritmo de la voz; planas cuando no se escucha. */
public class VoiceLevelView extends View {
    private static final int BARS = 5;
    private static final float[] SHAPE = {0.55f, 0.85f, 1f, 0.8f, 0.5f};
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF bar = new RectF();
    private final float[] heights = new float[BARS];
    private final float density = getResources().getDisplayMetrics().density;
    private boolean live;
    private float level;
    private long started = System.nanoTime();

    public VoiceLevelView(Context context) {
        super(context);
        setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
    }

    public void setColor(int color) {
        paint.setColor(color);
        invalidate();
    }

    public void setLive(boolean value) {
        if (live == value) return;
        live = value;
        invalidate();
    }

    public void setLevel(float value) {
        level = Math.max(level, Math.max(0f, Math.min(1f, value)));
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float w = getWidth();
        float h = getHeight();
        float gap = w / (BARS * 2f - 1f);
        float t = (System.nanoTime() - started) / 1e9f;
        boolean moving = false;
        for (int i = 0; i < BARS; i++) {
            float idle = 0.18f + (live ? 0.12f * (0.5f + 0.5f * (float) Math.sin(t * 3.2f + i * 0.9f)) : 0f);
            float target = live ? Math.min(1f, idle + level * SHAPE[i] * 1.1f) : 0.18f;
            heights[i] += (target - heights[i]) * 0.3f;
            if (Math.abs(target - heights[i]) > 0.005f) moving = true;
            float bh = Math.max(gap, h * heights[i]);
            float left = i * gap * 2f;
            bar.set(left, (h - bh) / 2f, left + gap, (h + bh) / 2f);
            canvas.drawRoundRect(bar, gap / 2f, gap / 2f, paint);
        }
        level *= 0.88f;
        if (live || moving) postInvalidateOnAnimation();
    }

    @Override
    protected void onMeasure(int widthSpec, int heightSpec) {
        setMeasuredDimension(Math.round(30 * density), Math.round(26 * density));
    }
}
