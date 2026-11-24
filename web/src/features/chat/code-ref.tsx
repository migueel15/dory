import { useEffect, useState } from "react";
import { Code } from "@phosphor-icons/react";
import { HoverCard } from "../../components/tooltip";
import { highlight, languageOf, loadLanguage } from "../../highlight";

// A code reference is lines of a file from a diff, sent with a message and
// an optional comment. It goes to the agent as plain text that reads well and
// that the chat recognises again to draw it as an object:
//
//   Sobre `path` (líneas 12-18):
//   ```
//   …the lines…
//   ```
//   > the comment, one "> " per line
export type CodeRefData = { path: string; start: number; end: number; code: string; comment: string };

const header = /^Sobre `([^`]+)` \(líneas? (\d+)(?:-(\d+))?\):$/;

export const lineLabel = (start: number, end: number) => (start === end ? `${start}` : `${start}-${end}`);

export function codeRefText({ path, start, end, code, comment }: CodeRefData) {
  const head = `Sobre \`${path}\` (${start === end ? `línea ${start}` : `líneas ${start}-${end}`}):`;
  const body = `${head}\n\`\`\`\n${code}\n\`\`\``;
  return comment.trim() ? `${body}\n${comment.trim().split("\n").map((l) => `> ${l}`).join("\n")}` : body;
}

// readCodeRef reads a reference starting at lines[i]; it returns it and the
// index of its last line, or undefined when those lines are not one.
export function readCodeRef(lines: string[], i: number): { ref: CodeRefData; last: number } | undefined {
  const m = header.exec(lines[i]);
  if (!m) return undefined;
  let j = i + 1;
  while (lines[j] === "") j++;
  const fence = /^(`{3,})/.exec(lines[j] ?? "");
  if (!fence) return undefined;
  const code: string[] = [];
  let k = j + 1;
  for (; k < lines.length && !lines[k].startsWith(fence[1]); k++) code.push(lines[k]);
  if (k >= lines.length) return undefined;
  const comment: string[] = [];
  while (lines[k + 1]?.startsWith(">")) comment.push(lines[++k].replace(/^> ?/, ""));
  const start = Number(m[2]);
  return { ref: { path: m[1], start, end: m[3] ? Number(m[3]) : start, code: code.join("\n"), comment: comment.join("\n") }, last: k };
}

// CodeRefCard is the full reference: file and lines, the code coloured by
// its language like in the diff (added lines on green, removed on red,
// scrolling when long) and the comment.
export function CodeRefCard({ data }: { data: CodeRefData }) {
  const lang = languageOf(data.path);
  const [, setLoaded] = useState(0);
  useEffect(() => { if (lang) loadLanguage(lang).then((ok) => ok && setLoaded((n) => n + 1)); }, [lang]);
  return (
    <div className="w-[min(36rem,calc(100vw-2rem))] space-y-1.5 text-left">
      <p className="truncate font-mono text-[0.6875rem] text-slate-300 dark:text-muted">{data.path}<span className="opacity-70">:{lineLabel(data.start, data.end)}</span></p>
      <pre className="code-dark max-h-64 overflow-auto overscroll-contain rounded bg-[#1e1e1e] py-1.5 font-mono text-[0.6875rem] leading-4 text-[#d4d4d4]">{data.code.split("\n").map((line, i) => {
        // Lines from a diff start with their mark: + added, - removed, space kept.
        const mark = line[0] === "+" || line[0] === "-" || line[0] === " " ? line[0] : "";
        const body = mark ? line.slice(1) : line;
        const html = lang ? highlight(body, lang) : undefined;
        return (
          <div key={i} className={`flex px-2 ${mark === "+" ? "bg-emerald-500/15" : mark === "-" ? "bg-red-500/15" : ""}`}>
            <span aria-hidden="true" className={`w-3 shrink-0 select-none ${mark === "+" ? "text-emerald-400" : mark === "-" ? "text-red-400" : "text-transparent"}`}>{mark || " "}</span>
            {html !== undefined ? <span className="whitespace-pre" dangerouslySetInnerHTML={{ __html: html || " " }} /> : <span className="whitespace-pre">{body || " "}</span>}
          </div>
        );
      })}</pre>
      {data.comment && <p className="whitespace-pre-line text-xs leading-snug">{data.comment}</p>}
    </div>
  );
}

// CodeRefChip draws a sent reference in the chat as an object: the file, its
// lines and the start of the comment. A click opens the whole reference.
export function CodeRefChip({ data }: { data: CodeRefData }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  useEffect(() => {
    if (!anchor) return;
    // Clicks and scrolls inside the card itself keep it open.
    const inside = (e: Event) => e.target instanceof Element && (anchor.contains(e.target) || !!e.target.closest("[data-hover-card]"));
    const close = (e: Event) => { if (!inside(e)) setAnchor(null); };
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") setAnchor(null); };
    document.addEventListener("pointerdown", close, true);
    document.addEventListener("keydown", key);
    window.addEventListener("scroll", close, true);
    return () => { document.removeEventListener("pointerdown", close, true); document.removeEventListener("keydown", key); window.removeEventListener("scroll", close, true); };
  }, [anchor]);
  return (
    <>
      <button type="button" onClick={(e) => setAnchor(anchor ? null : e.currentTarget)} aria-expanded={!!anchor} aria-label={`Referencia a ${data.path}, líneas ${lineLabel(data.start, data.end)}`}
        className="flex w-full min-w-0 items-center gap-2 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-left text-sm hover:bg-slate-50 dark:border-border dark:bg-surface dark:hover:bg-raised">
        <Code weight="light" aria-hidden="true" className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="shrink-0 font-mono text-xs">{data.path.split("/").pop()}<span className="text-slate-500 dark:text-muted">:{lineLabel(data.start, data.end)}</span></span>
        {data.comment && <span className="min-w-0 flex-1 truncate text-slate-600 dark:text-text-secondary">{data.comment}</span>}
      </button>
      {anchor && <HoverCard anchor={anchor} interactive><CodeRefCard data={data} /></HoverCard>}
    </>
  );
}
