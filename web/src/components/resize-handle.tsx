import type { RefObject } from "react";

// ResizeHandle lets a side panel be dragged wider or narrower.
export function ResizeHandle({ panel, width, onResize, min = 320, rest = 420 }: { panel: RefObject<HTMLElement | null>; width: number; onResize: (w: number) => void; storageKey?: string | null; min?: number; rest?: number }) {
  return (
    <div role="separator" aria-orientation="vertical" tabIndex={0}
      className="w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-slate-300 dark:hover:bg-border"
      onPointerDown={(e) => {
        // Dragging must not select the conversation or the diff it crosses.
        e.preventDefault();
        window.getSelection()?.removeAllRanges();
        const body = document.body.style, prev = [body.userSelect, body.cursor];
        body.userSelect = "none";
        body.cursor = "col-resize";
        // The panel animates its width when it opens or closes; while dragging
        // it must follow the pointer at once, or widening lags behind the content.
        const el = panel.current, transition = el?.style.transition ?? "";
        if (el) el.style.transition = "none";
        const start = e.clientX, from = width;
        const right = panel.current?.getBoundingClientRect().right ?? window.innerWidth;
        const move = (ev: PointerEvent) => onResize(Math.max(min, Math.min(from + start - ev.clientX, right - rest)));
        const up = () => {
          [body.userSelect, body.cursor] = prev;
          if (el) el.style.transition = transition;
          window.removeEventListener("pointermove", move);
          window.removeEventListener("pointerup", up);
          window.removeEventListener("pointercancel", up);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up);
        window.addEventListener("pointercancel", up);
      }} />
  );
}
