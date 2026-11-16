import { useEffect, useRef, useState } from "react";

// How text being written appears, as in Claude: it is revealed at a steady
// pace instead of in bursts, and each new word fades in.

const FADE_MS = 500;

// useSmoothText reveals text that grows while streaming at a pace that keeps
// up with what arrived; once the stream ends the rest shows at once.
export function useSmoothText(text: string, streaming: boolean): string {
  const [shown, setShown] = useState(streaming ? 0 : text.length);
  const target = useRef(text);
  target.current = text;
  useEffect(() => {
    if (!streaming) { setShown(text.length); return; }
    let frame = 0, last = performance.now();
    const tick = (now: number) => {
      setShown((n) => {
        const backlog = target.current.length - n;
        if (backlog <= 0) return n;
        // Faster when far behind, never slower than ~60 characters a second.
        return Math.min(target.current.length, n + Math.max(1, Math.round(((now - last) / 1000) * Math.max(60, backlog * 3))));
      });
      last = now;
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [streaming]); // eslint-disable-line react-hooks/exhaustive-deps
  return streaming ? text.slice(0, shown) : text;
}

type Node = { type: string; value?: string; tagName?: string; properties?: Record<string, unknown>; children?: Node[] };

// rehypeFade wraps the words that appeared in the last moments in spans that
// fade in. seen keeps when each word (by position) first showed; a span's
// animation starts in the past by its age, so a word re-rendered mid-fade
// continues where it was instead of starting again.
export function rehypeFade(seen: number[]) {
  return () => (tree: Node) => {
    const now = performance.now();
    let word = 0;
    const walk = (node: Node) => {
      if (!node.children) return;
      const out: Node[] = [];
      for (const child of node.children) {
        if (child.type === "text" && child.value) {
          for (const part of child.value.split(/(\s+)/)) {
            if (!part.trim()) { if (part) out.push({ type: "text", value: part }); continue; }
            const age = now - (seen[word] ??= now);
            word++;
            out.push(age >= FADE_MS ? { type: "text", value: part }
              : { type: "element", tagName: "span", properties: { className: ["word-in"], style: `animation-delay:-${Math.round(age)}ms` }, children: [{ type: "text", value: part }] });
          }
          continue;
        }
        // Code blocks and formulas keep their own layout.
        const cls = child.properties?.className;
        if (child.tagName !== "pre" && !(Array.isArray(cls) && cls.some((c) => String(c).startsWith("katex")))) walk(child);
        out.push(child);
      }
      node.children = out;
    };
    walk(tree);
  };
}
