import { useEffect, useRef, useState } from "react";
import type { Message } from "../api/client";
import { useTheme } from "../theme";

// HtmlCard shows the page of a render_html call. The page is a file of the
// thread's node; the message only carries its reference, and the frame loads
// it sandboxed (opaque origin, scripts on), so it cannot reach Dory.

export const isHtml = (m: Message) => m.role === "tool" && !!m.tool && (m.tool.tool === "render_html" || /(^|_)render_html$/.test(m.tool.name));

type Ref = { attachment_id: string; title: string; height: number };

// htmlRef reads the reference from the tool's result.
export function htmlRef(m: Message): Ref | undefined {
  const at = m.text.indexOf("{");
  if (at < 0) return undefined;
  try {
    const r = JSON.parse(m.text.slice(at))?.html_render;
    return typeof r?.attachment_id === "string" && /^[\w-]+$/.test(r.attachment_id) ? r : undefined;
  } catch { return undefined; }
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
