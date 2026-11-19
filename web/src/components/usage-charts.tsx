import { useState } from "react";

// Provider and token-type colors, shared by dots, areas and bars so each
// series keeps one color on the whole page. They read on light and dark.
export const PROVIDER_COLORS: Record<string, string> = { claude: "#e07a52", codex: "#3b82f6" };
export const providerColor = (id: string) => PROVIDER_COLORS[id] ?? "#94a3b8";

export const usd = (n: number) => n.toLocaleString("es-ES", { style: "currency", currency: "USD", maximumFractionDigits: n >= 100 ? 0 : 2 });
export const compact = (n: number) => n.toLocaleString("es-ES", { notation: "compact", maximumFractionDigits: 1 });
export const percent = (n: number) => `${(n * 100).toLocaleString("es-ES", { maximumFractionDigits: 1 })} %`;

export type Series = { id: string; label: string; color: string; values: number[] };

const W = 640, H = 220, PAD = { l: 48, r: 8, t: 8, b: 24 };

// smooth draws a Catmull-Rom curve through the points as cubic Béziers.
function smooth(pts: [number, number][]) {
  if (pts.length < 2) return pts.length ? `M${pts[0][0]},${pts[0][1]}` : "";
  let d = `M${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const [p0, p1, p2, p3] = [pts[i - 1] ?? pts[i], pts[i], pts[i + 1], pts[i + 2] ?? pts[i + 1]];
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    // Clamp to the baseline so the curve never dips below zero.
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    d += ` C${c1[0]},${Math.min(c1[1], H - PAD.b)} ${c2[0]},${Math.min(c2[1], H - PAD.b)} ${p2[0]},${p2[1]}`;
  }
  return d;
}

const niceMax = (v: number) => {
  if (v <= 0) return 1;
  const p = 10 ** Math.floor(Math.log10(v));
  return [1, 2, 2.5, 5, 10].map((m) => m * p).find((m) => m >= v)!;
};

// AreaChart overlays one smoothed area per series over the same buckets.
export function AreaChart({ labels, series, format }: { labels: string[]; series: Series[]; format: (n: number) => string }) {
  const [hover, setHover] = useState<number | null>(null);
  const n = labels.length;
  const max = niceMax(Math.max(0, ...series.flatMap((s) => s.values)));
  const x = (i: number) => PAD.l + (n > 1 ? (i * (W - PAD.l - PAD.r)) / (n - 1) : (W - PAD.l - PAD.r) / 2);
  const y = (v: number) => H - PAD.b - (v / max) * (H - PAD.t - PAD.b);
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * max);
  const every = Math.max(1, Math.ceil(n / 6));
  const move = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const px = ((e.clientX - r.left) / r.width) * W;
    setHover(n > 1 ? Math.max(0, Math.min(n - 1, Math.round(((px - PAD.l) / (W - PAD.l - PAD.r)) * (n - 1)))) : 0);
  };
  return (
    <div className="relative">
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full touch-none select-none" onPointerMove={move} onPointerLeave={() => setHover(null)} role="img">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={PAD.l} x2={W - PAD.r} y1={y(t)} y2={y(t)} className="stroke-slate-200 dark:stroke-border" strokeDasharray={t ? "3 3" : undefined} />
            <text x={PAD.l - 6} y={y(t)} dy="0.32em" textAnchor="end" className="fill-slate-500 text-[10px] dark:fill-muted">{format(t)}</text>
          </g>
        ))}
        {labels.map((l, i) => (i % every === 0 || i === n - 1) && (i === n - 1 || n - 1 - i >= every / 2) ? (
          <text key={i} x={x(i)} y={H - 6} textAnchor="middle" className="fill-slate-500 text-[10px] dark:fill-muted">{l}</text>
        ) : null)}
        {series.map((s) => {
          const pts = s.values.map((v, i) => [x(i), y(v)] as [number, number]);
          const line = smooth(pts);
          return (
            <g key={s.id}>
              <path d={`${line} L${x(n - 1)},${y(0)} L${x(0)},${y(0)} Z`} fill={s.color} fillOpacity={0.18} />
              <path d={line} fill="none" stroke={s.color} strokeWidth={2} />
            </g>
          );
        })}
        {hover !== null && <line x1={x(hover)} x2={x(hover)} y1={PAD.t} y2={H - PAD.b} className="stroke-slate-400 dark:stroke-muted" />}
        {hover !== null && series.map((s) => <circle key={s.id} cx={x(hover)} cy={y(s.values[hover])} r={3.5} fill={s.color} />)}
      </svg>
      {hover !== null && (
        <div className="pointer-events-none absolute top-0 rounded-md border border-slate-200 bg-white px-2 py-1 text-xs shadow dark:border-border dark:bg-surface"
          style={{ left: `${(x(hover) / W) * 100}%`, transform: `translateX(${hover > n / 2 ? "-105%" : "5%"})` }}>
          <p className="font-medium">{labels[hover]}</p>
          {series.map((s) => (
            <p key={s.id} className="flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: s.color }} />{s.label}<span className="ml-auto pl-3 tabular-nums">{format(s.values[hover])}</span></p>
          ))}
        </div>
      )}
    </div>
  );
}

// StackedBar splits one total into colored parts, with a legend below.
export function StackedBar({ parts, format }: { parts: { label: string; color: string; value: number }[]; format: (n: number) => string }) {
  const total = parts.reduce((a, p) => a + p.value, 0);
  return (
    <div className="space-y-3">
      <div className="flex h-3 overflow-hidden rounded-full bg-slate-200 dark:bg-hover">
        {total > 0 && parts.map((p) => <div key={p.label} title={`${p.label}: ${format(p.value)}`} style={{ width: `${(p.value / total) * 100}%`, background: p.color }} />)}
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-4">
        {parts.map((p) => (
          <div key={p.label}>
            <p className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted"><span className="size-2 rounded-full" style={{ background: p.color }} />{p.label}</p>
            <p className="tabular-nums">{format(p.value)} <span className="text-xs text-slate-500 dark:text-muted">{total ? percent(p.value / total) : ""}</span></p>
          </div>
        ))}
      </div>
    </div>
  );
}
