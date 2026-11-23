import { useEffect, useRef, useState } from "react";
import type { Message } from "../api/client";
import { useTheme } from "../theme";

// HtmlCard shows the page of a render_html call. The page is a file of the
// thread's node; the message only carries its reference, and the frame loads
// it sandboxed (opaque origin, scripts on), so it cannot reach Dory.

type Ref = { attachment_id: string; title: string; height: number };

// htmlRef finds the reference in the tool's result, whatever wraps it: plain
// JSON, an MCP result, a code-mode script output, or JSON escaped as a string
// inside any of those. Any tool whose result carries one counts as a page.
export function htmlRef(m: Message): Ref | undefined {
  if (m.role !== "tool" || !m.text.includes("html_render")) return undefined;
  const valid = (r: any): Ref | undefined => typeof r?.attachment_id === "string" && /^[\w-]+$/.test(r.attachment_id) ? r : undefined;
  const walk = (v: any, depth: number): Ref | undefined => {
    if (depth > 8 || v == null) return undefined;
    if (typeof v === "string") {
      if (!v.includes("html_render")) return undefined;
      // The JSON may follow a header ("Script completed…"): try each start.
      const end = Math.max(v.lastIndexOf("}"), v.lastIndexOf("]"));
      for (let at = 0; at < end; at++) {
        if (v[at] !== "{" && v[at] !== "[") continue;
        let parsed: unknown;
        try { parsed = JSON.parse(v.slice(at, end + 1)); } catch { continue; }
        const r = walk(parsed, depth + 1);
        if (r) return r;
      }
      return undefined;
    }
    if (Array.isArray(v)) { for (const x of v) { const r = walk(x, depth + 1); if (r) return r; } return undefined; }
    if (typeof v === "object") {
      const r = valid(v.html_render);
      if (r) return r;
      for (const x of Object.values(v)) { const f = walk(x, depth + 1); if (f) return f; }
    }
    return undefined;
  };
  return walk(m.text, 0);
}

const MIN = 80, MAX = 20000;

export function HtmlCard({ m, threadId }: { m: Message; threadId: string }) {
  const [dark] = useTheme();
  const frame = useRef<HTMLIFrameElement>(null);
  const ref = htmlRef(m);
  const [height, setHeight] = useState(() => Math.min(2000, Math.max(MIN, ref?.height || 240)));
  useEffect(() => {
    const on = (e: MessageEvent) => {
      if (e.source === frame.current?.contentWindow && typeof e.data?.doryHtml === "number") setHeight(Math.min(MAX, Math.max(MIN, Math.ceil(e.data.doryHtml))));
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, []);
  if (!ref || !threadId) return null;
  return (
    <figure className="my-1">
      <figcaption className="mb-1 text-[0.8125rem] text-slate-500 dark:text-muted">{ref.title}</figcaption>
      <iframe ref={frame} title={ref.title} sandbox="allow-scripts allow-popups" loading="lazy"
        src={`/api/threads/${threadId}/renders/${ref.attachment_id}?theme=${dark ? "dark" : "light"}`}
        className="block w-full border-0" style={{ height }} />
    </figure>
  );
}
