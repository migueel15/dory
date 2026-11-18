import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

// TooltipHost is the only tooltip of the app. Everywhere a `title` is set, the
// browser's own tooltip would show; this host takes the text over while the
// pointer or focus is on the element and draws it itself: centred over the
// element, rising into place, kept inside the viewport and flipped below when
// the top is short. Mount it once; a `title` needs nothing else.
const delay = 700; // like a native title: a label only shows after resting on the element

type Target = { el: HTMLElement; text: string };

export function TooltipHost() {
  const [target, setTarget] = useState<Target | null>(null);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const tip = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let current: HTMLElement | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    // Gives the text back to the element so its markup is as React left it.
    const release = () => {
      clearTimeout(timer);
      if (current) {
        const text = current.dataset.tip;
        if (text !== undefined && !current.hasAttribute("title")) current.setAttribute("title", text);
        delete current.dataset.tip;
      }
      current = null;
      setTarget(null);
      setPos(null);
    };
    const take = (el: HTMLElement) => {
      if (el === current) return;
      release();
      const text = el.getAttribute("title")?.trim();
      if (!text) return;
      el.dataset.tip = text;
      el.removeAttribute("title"); // so the browser draws nothing
      current = el;
      timer = setTimeout(() => setTarget({ el, text }), delay);
    };
    const find = (n: EventTarget | null) => (n instanceof Element ? (n.closest("[title]:not(svg *), [data-tip]") as HTMLElement | null) : null);
    const over = (e: PointerEvent) => { if (e.pointerType === "mouse") { const el = find(e.target); if (el) take(el); } };
    const out = (e: PointerEvent) => { if (current && e.pointerType === "mouse" && !(e.relatedTarget instanceof Node && current.contains(e.relatedTarget))) release(); };
    const focus = (e: FocusEvent) => { const el = find(e.target); if (el && el.matches(":focus-visible")) take(el); };
    const blur = () => release();
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") release(); };
    document.addEventListener("pointerover", over);
    document.addEventListener("pointerout", out);
    document.addEventListener("pointerdown", blur, true);
    document.addEventListener("focusin", focus);
    document.addEventListener("focusout", blur);
    document.addEventListener("keydown", key);
    window.addEventListener("scroll", blur, true);
    window.addEventListener("blur", blur);
    return () => {
      release();
      document.removeEventListener("pointerover", over);
      document.removeEventListener("pointerout", out);
      document.removeEventListener("pointerdown", blur, true);
      document.removeEventListener("focusin", focus);
      document.removeEventListener("focusout", blur);
      document.removeEventListener("keydown", key);
      window.removeEventListener("scroll", blur, true);
      window.removeEventListener("blur", blur);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useLayoutEffect(() => {
    if (!target || !tip.current) return;
    if (!target.el.isConnected) { setTarget(null); return; }
    const a = target.el.getBoundingClientRect(), t = tip.current.getBoundingClientRect(), m = 8, gap = 8;
    const left = Math.min(Math.max(m, a.left + a.width / 2 - t.width / 2), window.innerWidth - t.width - m);
    const top = a.top - t.height - gap >= m ? a.top - t.height - gap : a.bottom + gap;
    setPos({ left, top });
  }, [target]);

  if (!target) return null;
  return createPortal(
    <div ref={tip} role="tooltip" style={pos ?? { left: 0, top: 0, visibility: "hidden" }}
      className="tip-in pointer-events-none fixed z-[100] max-w-xs whitespace-pre-line rounded-md bg-slate-900 px-2.5 py-1.5 text-center text-xs font-medium leading-snug text-white shadow-lg dark:bg-text dark:text-canvas">
      {target.text}
    </div>,
    document.body,
  );
}

// HoverCard is information the pointer asks for by resting on a control (the
// remaining usage, what can be updated): unlike a label it shows at once. It
// is centred over the anchor and rises into place like the tooltips.
// With side="left" it sits beside the anchor instead, or on its right when
// the left is short.
// `edge` lends its sides when the anchor sits inside a box the card must clear.
export function HoverCard({ anchor, children, side = "top", edge, gap: sideGap = 8 }: { anchor: HTMLElement | null; children: React.ReactNode; side?: "top" | "left"; edge?: HTMLElement | null; gap?: number }) {
  const card = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  useLayoutEffect(() => {
    if (!anchor || !card.current) return;
    const a = anchor.getBoundingClientRect(), t = card.current.getBoundingClientRect(), m = 8, gap = 8;
    if (side === "left") {
      const e = (edge ?? anchor).getBoundingClientRect();
      const top = Math.min(Math.max(m, a.top + a.height / 2 - t.height / 2), window.innerHeight - t.height - m);
      setPos({ left: e.left - t.width - sideGap >= m ? e.left - t.width - sideGap : Math.min(e.right + sideGap, window.innerWidth - t.width - m), top });
      return;
    }
    setPos({ left: Math.min(Math.max(m, a.left + a.width / 2 - t.width / 2), window.innerWidth - t.width - m), top: a.top - t.height - gap >= m ? a.top - t.height - gap : a.bottom + gap });
  }, [anchor, side, edge, sideGap]);
  if (!anchor) return null;
  return createPortal(
    <div ref={card} role="tooltip" style={pos ?? { left: 0, top: 0, visibility: "hidden" }}
      className="tip-in pointer-events-none fixed z-[100] rounded-md bg-slate-900 px-2.5 py-1.5 text-xs text-white shadow-lg dark:bg-raised dark:text-text">
      {children}
    </div>,
    anchor.closest("dialog") ?? document.body,
  );
}
