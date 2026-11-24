import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowDown } from "@phosphor-icons/react";
import { events } from "../../lib/store";

// useFollow keeps a scroller pinned to the end unless the user scrolled up,
// and says when the end is far enough to offer a way back.
export function useFollow(deps: unknown[]) {
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  // Where the last scroll left the view: only a move up is the user leaving the end.
  const lastTop = useRef(0);
  // True while "Ir al final" glides down, so its own steps do not unpin.
  const chasing = useRef(false);
  const [away, setAway] = useState(false);
  const anchor = useRef<{ id: string; offset: number } | null>(null);
  const toEnd = () => {
    const el = scroller.current;
    if (el) { el.scrollTop = el.scrollHeight; lastTop.current = el.scrollTop; }
    anchor.current = null; // an anchor from an earlier visit up would jump the view back there
  };
  // Away from the end, the message under the top of the view is the anchor:
  // whatever grows or shrinks around it (a step list collapsing, the answer
  // ending, heights painted late), the view keeps it at the same place.
  const mark = () => {
    const el = scroller.current;
    if (!el) return;
    const top = el.getBoundingClientRect().top;
    anchor.current = null;
    for (const n of Array.from(el.querySelectorAll<HTMLElement>("[data-anchor]"))) {
      const r = n.getBoundingClientRect();
      if (r.bottom > top) { anchor.current = { id: n.dataset.anchor!, offset: r.top - top }; return; }
    }
  };
  const hold = () => {
    const el = scroller.current;
    const a = anchor.current;
    if (!el || !a) return;
    const n = el.querySelector<HTMLElement>(`[data-anchor="${CSS.escape(a.id)}"]`);
    if (!n) return mark();
    const delta = n.getBoundingClientRect().top - el.getBoundingClientRect().top - a.offset;
    if (Math.abs(delta) >= 1) { el.scrollTop += delta; lastTop.current = el.scrollTop; }
  };
  const settle = () => { if (pinned.current) toEnd(); else hold(); };
  useLayoutEffect(() => {
    settle();
  }, deps); // eslint-disable-line react-hooks/exhaustive-deps
  // Content also grows without new messages (code highlighted late, a step
  // opened, the composer taller): keep the end in view while pinned.
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(() => { if (pinned.current) chasing.current = false; settle(); });
    const watch = () => { ro.disconnect(); ro.observe(el); for (const c of Array.from(el.children)) ro.observe(c); };
    watch();
    const mo = new MutationObserver(watch);
    mo.observe(el, { childList: true });
    return () => { ro.disconnect(); mo.disconnect(); };
  }, []);
  // Only the user's own gesture leaves the end: the browser also moves the
  // view by itself (heights of messages painted late, scroll anchoring), and
  // those moves must not unpin.
  const touched = useRef(0);
  const held = useRef(false);
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const touch = () => { touched.current = Date.now(); chasing.current = false; };
    const down = () => { touched.current = Date.now(); };
    // A gesture up leaves the end at once, before content growing in the same
    // frame can pull the view back down and make it flicker.
    // Mark the anchor now: content may grow before the scroll event arrives,
    // and holding with no anchor or an old one moves the view.
    const leave = () => { touch(); if (pinned.current) { pinned.current = false; mark(); } };
    const wheel = (e: WheelEvent) => { if (e.deltaY < 0) leave(); else down(); };
    const key = (e: KeyboardEvent) => { if (["ArrowUp", "PageUp", "Home"].includes(e.key)) leave(); else if (["ArrowDown", "PageDown", "End", " "].includes(e.key)) down(); };
    const pointer = (e: PointerEvent) => { if (e.target === el) { held.current = true; touch(); } }; // the scrollbar
    const release = () => { held.current = false; };
    // A page rendered in a frame is another document: its wheel and keys never
    // reach el, so the page reports a gesture up itself.
    const framed = (e: MessageEvent) => { if (e.data?.doryUp && Array.from(el.querySelectorAll("iframe")).some((f) => f.contentWindow === e.source)) leave(); };
    window.addEventListener("message", framed);
    window.addEventListener("pointerup", release);
    el.addEventListener("wheel", wheel, { passive: true });
    el.addEventListener("touchmove", touch, { passive: true });
    el.addEventListener("keydown", key);
    el.addEventListener("pointerdown", pointer);
    return () => { el.removeEventListener("wheel", wheel); el.removeEventListener("touchmove", touch); el.removeEventListener("keydown", key); el.removeEventListener("pointerdown", pointer); window.removeEventListener("pointerup", release); window.removeEventListener("message", framed); };
  }, []);
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    const up = el.scrollTop < lastTop.current - 1;
    lastTop.current = el.scrollTop;
    // A move is the user's after a gesture, or with the pointer over a frame,
    // whose page may not have reported it. A move up by the user leaves the end
    // even within the last pixels; only the user (or "Ir al final") pins there:
    // content shrinking under the view clamps it there too.
    const mine = held.current || chasing.current || Date.now() - touched.current < 1000 || !!el.querySelector("iframe:hover");
    if (up && !chasing.current && mine) { if (pinned.current) anchor.current = null; pinned.current = false; }
    else if (distance < 40 && !up && (pinned.current || mine)) { pinned.current = true; chasing.current = false; }
    else if (pinned.current) toEnd(); // the browser moved the view; go back to the end
    if (!pinned.current) mark();
    setAway(!pinned.current && distance > 200);
  };
  const follow = (smooth = false) => {
    pinned.current = true;
    const el = scroller.current;
    if (!el) return;
    const glide = smooth && !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    chasing.current = glide;
    el.scrollTo({ top: el.scrollHeight, behavior: glide ? "smooth" : "auto" });
  };
  return { scroller, onScroll, follow, away };
}

// ToEnd is the floating "Ir al final" button.
export function ToEnd({ away, follow }: { away: boolean; follow: (smooth?: boolean) => void }) {
  return (
    <div className="relative h-0">
      <button type="button" onClick={() => follow(true)} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
        className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
        <ArrowDown weight="bold" className="size-3.5" />Ir al final
      </button>
    </div>
  );
}

