import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";

// A SelectionAction runs on the selected text and the element matched by the
// menu's scope (for example the plan section it came from).
export type SelectionAction = { label: string; icon?: ReactNode; run: (text: string, scope: HTMLElement) => void };

// place centres the bar on the pointer, so every option is equally close:
// below it when there is room, otherwise above, never off screen.
const BAR = { w: 220, h: 34, gap: 10 };
const place = ({ top, left }: { top: number; left: number }) => ({
  top: top + BAR.gap + BAR.h <= window.innerHeight - 8 ? top + BAR.gap : Math.max(8, top - BAR.gap - BAR.h),
  left: Math.min(Math.max(left, BAR.w / 2 + 8), window.innerWidth - BAR.w / 2 - 8),
});

// SelectionMenu floats action buttons where the pointer releases a text
// selection made inside box, within an element matching scope. Any press
// outside it, a cleared selection or a scroll closes it.
export function SelectionMenu({ box, scope, actions, label = "Acciones sobre la selección" }: { box: RefObject<HTMLElement | null>; scope: string; actions: SelectionAction[]; label?: string }) {
  const [sel, setSel] = useState<{ text: string; el: HTMLElement; top: number; left: number } | null>(null);
  const bar = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const read = (e: Event) => {
      const s = window.getSelection();
      const text = s?.toString().trim() ?? "";
      const node = s && s.rangeCount ? s.getRangeAt(0).commonAncestorContainer : null;
      const el = (node instanceof Element ? node : node?.parentElement)?.closest<HTMLElement>(scope);
      if (!text || !el || !box.current?.contains(el)) { setSel(null); return; }
      const rects = s!.getRangeAt(0).getClientRects();
      const last = rects[rects.length - 1] ?? s!.getRangeAt(0).getBoundingClientRect();
      // TouchEvent does not exist on desktop Firefox: detect touches by shape.
      const t = "changedTouches" in e ? (e as TouchEvent).changedTouches[0] : undefined;
      const at = e instanceof MouseEvent ? { x: e.clientX, y: e.clientY } : t ? { x: t.clientX, y: t.clientY } : { x: last.right, y: last.bottom };
      setSel({ text, el, top: at.y, left: at.x });
    };
    const press = (e: Event) => { if (!bar.current?.contains(e.target as Node)) setSel(null); };
    const change = () => { if (!window.getSelection()?.toString().trim()) setSel(null); };
    const hide = () => setSel(null);
    const on: [string, EventListener, boolean?][] = [["mouseup", read], ["keyup", read], ["touchend", read], ["mousedown", press], ["touchstart", press], ["selectionchange", change], ["scroll", hide, true]];
    on.forEach(([n, f, c]) => document.addEventListener(n, f, c));
    return () => on.forEach(([n, f, c]) => document.removeEventListener(n, f, c));
  }, [box, scope]);
  if (!sel) return null;
  const btn = "inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium hover:bg-white/15";
  return createPortal(
    <div ref={bar} role="toolbar" aria-label={label} onMouseDown={(e) => e.preventDefault()} style={place(sel)}
      className="fixed z-50 flex -translate-x-1/2 items-center rounded-full bg-slate-900 p-0.5 text-white shadow-lg dark:bg-raised">
      {actions.map((a) => (
        <button key={a.label} type="button" className={btn}
          onClick={() => { a.run(sel.text, sel.el); window.getSelection()?.removeAllRanges(); setSel(null); }}>{a.icon}{a.label}</button>
      ))}
    </div>,
    document.body,
  );
}

// quote turns text into a Markdown block quote.
export const quote = (text: string) => text.split("\n").filter(Boolean).map((l) => `> ${l}`).join("\n");
