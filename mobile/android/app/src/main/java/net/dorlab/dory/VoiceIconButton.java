package net.dorlab.dory;

import android.content.Context;
import android.content.res.ColorStateList;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RectF;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.view.View;

/** Botón circular con un icono dibujado a mano: pausa, reanudar o cerrar. */
public class VoiceIconButton extends View {
    public enum Icon { PLAY, CLOSE, PLUS, SEND }

    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path path = new Path();
    private final RectF box = new RectF();
    private final float density = getResources().getDisplayMetrics().density;
    private final int size;
    private final int ink;
    private Icon icon = Icon.CLOSE;

    public VoiceIconButton(Context context, int sizeDp, int fill, int ink) {
        super(context);
        this.size = Math.round(sizeDp * density);
        this.ink = ink;
        GradientDrawable disc = new GradientDrawable();
        disc.setShape(GradientDrawable.OVAL);
        disc.setColor(fill);
        setBackground(disc);
        GradientDrawable mask = new GradientDrawable();
        mask.setShape(GradientDrawable.OVAL);
        mask.setColor(0xffffffff);
        setForeground(new RippleDrawable(ColorStateList.valueOf(0x33ffffff), null, mask));
        setClickable(true);
        setFocusable(true);
    }

    public void setIcon(Icon value, String description) {
        icon = value;
        setContentDescription(description);
        invalidate();
    }

    @Override
    protected void onMeasure(int widthSpec, int heightSpec) {
        setMeasuredDimension(size, size);
    }

    @Override
    protected void onDraw(Canvas canvas) {
        float c = size / 2f;
        float u = size / 24f;
        paint.setColor(ink);
        paint.setStyle(Paint.Style.FILL);
        if (icon == Icon.PLAY) {
            path.reset();
            path.moveTo(c - 3.6f * u, c - 6f * u);
            path.lineTo(c + 6.4f * u, c);
            path.lineTo(c - 3.6f * u, c + 6f * u);
            path.close();
            paint.setStrokeJoin(Paint.Join.ROUND);
            paint.setStrokeWidth(2f * u);
            paint.setStyle(Paint.Style.FILL_AND_STROKE);
            canvas.drawPath(path, paint);
        } else if (icon == Icon.PLUS || icon == Icon.SEND) {
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeCap(Paint.Cap.ROUND);
            paint.setStrokeJoin(Paint.Join.ROUND);
            paint.setStrokeWidth((icon == Icon.SEND ? 1.2f : 2f) * u);
            float d = (icon == Icon.SEND ? 3f : 5f) * u;
            canvas.drawLine(c, c - d, c, c + d, paint);
            if (icon == Icon.PLUS) canvas.drawLine(c - d, c, c + d, c, paint);
            else {
                canvas.drawLine(c, c - d, c - d * 0.7f, c - d * 0.3f, paint);
                canvas.drawLine(c, c - d, c + d * 0.7f, c - d * 0.3f, paint);
            }
        } else {
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeCap(Paint.Cap.ROUND);
            paint.setStrokeWidth(2.2f * u);
            float d = 4.6f * u;
            canvas.drawLine(c - d, c - d, c + d, c + d, paint);
            canvas.drawLine(c + d, c - d, c - d, c + d, paint);
        }
    }
}
