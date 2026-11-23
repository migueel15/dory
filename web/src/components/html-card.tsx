import { useEffect, useMemo, useRef, useState } from "react";
import type { Message } from "../api/client";
import { useTheme } from "../theme";

// HtmlCard shows the page of a render_html call in a sandboxed frame: an
// opaque origin with scripts on, so the page cannot reach Dory or its cookies.
// The theme goes in as CSS variables and the frame fits the page's height.

export const isHtml = (m: Message) => m.role === "tool" && !!m.tool && (m.tool.tool === "render_html" || /(^|_)render_html$/.test(m.tool.name));

const MIN = 80, MAX = 20000;

const palette = (dark: boolean) => dark
  ? "--background:#0f0f11;--foreground:#e6e6e8;--muted:#9898a2;--card:#17171a;--border:#2a2a30;--accent:#5e6ad2;--accent-foreground:#fff"
  : "--background:#fff;--foreground:#0f172a;--muted:#64748b;--card:#f8fafc;--border:#e2e8f0;--accent:#5e6ad2;--accent-foreground:#fff";

// The base sheet and a script that reports the page height to the parent.
const head = (dark: boolean) => `<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>:root{${palette(dark)};--radius:8px;--font-sans:"Adwaita Sans",Inter,system-ui,sans-serif;--font-mono:ui-monospace,monospace;color-scheme:${dark ? "dark" : "light"}}
html,body{margin:0;background:transparent;color:var(--foreground);font:14px/1.5 var(--font-sans)}html{padding:8px}</style>
<script>(()=>{const s=()=>parent.postMessage({doryHtml:document.documentElement.scrollHeight},"*");addEventListener("load",s);new ResizeObserver(s).observe(document.documentElement);})()</script>`;

export function HtmlCard({ m }: { m: Message }) {
  const [dark] = useTheme();
  const frame = useRef<HTMLIFrameElement>(null);
  const input = useMemo(() => { try { return JSON.parse(m.tool?.input ?? "{}"); } catch { return {}; } }, [m.tool?.input]);
  const [height, setHeight] = useState<number>(Math.min(2000, Math.max(MIN, Number(input.height) || 240)));
  useEffect(() => {
    const on = (e: MessageEvent) => {
      if (e.source === frame.current?.contentWindow && typeof e.data?.doryHtml === "number") setHeight(Math.min(MAX, Math.max(MIN, Math.ceil(e.data.doryHtml))));
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, []);
  const html = typeof input.html === "string" ? input.html : "";
  if (!html) return null;
  return (
    <figure className="my-1">
      {input.title && <figcaption className="mb-1 text-[0.8125rem] text-slate-500 dark:text-muted">{input.title}</figcaption>}
      <iframe ref={frame} title={input.title || "HTML"} sandbox="allow-scripts allow-popups" srcDoc={head(dark) + html}
        className="block w-full border-0" style={{ height }} />
    </figure>
  );
}
