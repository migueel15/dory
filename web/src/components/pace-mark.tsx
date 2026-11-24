import { useLayoutEffect, useRef } from "react";

// PaceMark draws the expected-pace tick on a usage bar. It snaps its edge and
// width to whole device pixels, so it keeps the same thickness wherever the
// bar starts and at fractional screen scales.
export function PaceMark({ pace, className = "bg-slate-900 dark:bg-white" }: { pace: number; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = ref.current, bar = el?.parentElement;
    if (!el || !bar) return;
    const place = () => {
      const dpr = window.devicePixelRatio || 1, r = bar.getBoundingClientRect();
      const width = Math.max(1, Math.round(2 * dpr)) / dpr;
      const x = Math.round((r.left + r.width * pace / 100 - width / 2) * dpr) / dpr;
      el.style.left = `${x - r.left}px`;
      el.style.width = `${width}px`;
    };
    place();
    const ro = new ResizeObserver(place);
    ro.observe(bar);
    return () => ro.disconnect();
  }, [pace]);
  return <div ref={ref} aria-hidden="true" className={`absolute -bottom-0.5 -top-0.5 ${className}`} />;
}
