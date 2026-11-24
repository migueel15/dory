import { createContext, memo, useCallback, useContext, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { createPortal } from "react-dom";
import { ArrowUp, GithubLogo, Lightning, X, ArrowsIn, CaretDown, ArrowsOut, CaretRight, Columns, Rows, SidebarSimple, ChatCircle, TreeView, File as FileIcon, Folder, FolderOpen } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { DictationMic, DictationPreview, useEnterFinishesDictation, useLocalDictation } from "../../components/local-dictation";
import { Button, ErrorText, Input, keepAboveKeyboard } from "../../components/ui";
import { Modal } from "../../components/ui";
import { useLaunchForm } from "../../components/launch-picker";
import type { LaunchChoice } from "../../api/client";
import type { FileRef } from "../../components/refs";
import { useArm } from "./github-ui";
import { highlight, languageOf, loadLanguage } from "../../highlight";
import { qk } from "../../api/queries/keys";

type Side = "LEFT" | "RIGHT";
type Line = { text: string; kind: "add" | "del" | "ctx" | "hunk"; old?: number; new?: number };
type File = { path: string; lines: Line[]; added: number; removed: number };
type Pick = { file: string; start: number; end: number }; // start -1: whole file

// patchHash fingerprints a file patch so a review mark expires when the file changes.
async function patchHash(f: File): Promise<string> {
  const data = new TextEncoder().encode(f.lines.map((l) => l.text).join("\n"));
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-1", data)), (b) => b.toString(16).padStart(2, "0")).join("");
}

// parseDiff splits a unified diff into files with old and new line numbers.
function parseDiff(diff: string): File[] {
  const files: File[] = [];
  let f: File | undefined;
  let o = 0, n = 0;
  for (const text of diff.split("\n")) {
    if (text.startsWith("diff --git ")) {
      f = { path: text.replace(/^diff --git a\/.* b\//, ""), lines: [], added: 0, removed: 0 };
      files.push(f);
      continue;
    }
    if (!f) continue;
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(text);
    if (h) {
      o = Number(h[1]); n = Number(h[2]);
      f.lines.push({ text, kind: "hunk" });
    } else if (!f.lines.length) {
      continue; // file header: index, ---, +++
    } else if (text.startsWith("+")) {
      f.lines.push({ text, kind: "add", new: n++ }); f.added++;
    } else if (text.startsWith("-")) {
      f.lines.push({ text, kind: "del", old: o++ }); f.removed++;
    } else if (text.startsWith(" ")) {
      f.lines.push({ text, kind: "ctx", old: o++, new: n++ });
    }
  }
  return files;
}

type Row = { hunk?: number; left?: number; right?: number };

// splitRows pairs removed and added lines side by side, like GitHub's split view.
function splitRows(lines: Line[]): Row[] {
  const rows: Row[] = [];
  let dels: number[] = [], adds: number[] = [];
  const flush = () => {
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) rows.push({ left: dels[k], right: adds[k] });
    dels = []; adds = [];
  };
  lines.forEach((l, i) => {
    if (l.kind === "del") { if (adds.length) flush(); dels.push(i); }
    else if (l.kind === "add") adds.push(i);
    else { flush(); rows.push(l.kind === "hunk" ? { hunk: i } : { left: i, right: i }); }
  });
  flush();
  return rows;
}

export type Dir<F = File> = { name: string; dirs: Dir<F>[]; files: F[] };

// tree groups files by folder and joins folders that hold a single folder.
export function tree<F extends { path: string } = File>(files: F[]): Dir<F> {
  const root: Dir<F> = { name: "", dirs: [], files: [] };
  for (const f of files) {
    let d = root;
    for (const part of f.path.split("/").slice(0, -1)) {
      let next = d.dirs.find((x) => x.name === part);
      if (!next) d.dirs.push((next = { name: part, dirs: [], files: [] }));
      d = next;
    }
    d.files.push(f);
  }
  const squash = (d: Dir<F>): Dir<F> => {
    d.dirs = d.dirs.map(squash);
    if (d.name && d.dirs.length === 1 && !d.files.length) return { ...d.dirs[0], name: `${d.name}/${d.dirs[0].name}` };
    return d;
  };
  return squash(root);
}

type Review = { reviewed: Set<string>; toggle?: (path: string) => void };

function Tree({ dir, jump, active, review, depth = 0 }: { dir: Dir; jump: (path: string) => void; active?: string; review: Review; depth?: number }) {
  return (
    <ul className={depth ? "ml-2 border-l border-slate-200 pl-1.5 dark:border-border" : ""}>
      {dir.dirs.map((d) => (
        <li key={d.name}>
          <details open className="group">
            <summary title={d.name} className="flex cursor-pointer list-none items-center gap-1 rounded px-1 py-0.5 text-xs font-medium hover:bg-slate-100 dark:hover:bg-raised [&::-webkit-details-marker]:hidden">
              <CaretRight weight="regular" aria-hidden="true" className="size-3 shrink-0 text-slate-400 transition-transform group-open:rotate-90" />
              <Folder aria-hidden="true" className="size-4 shrink-0 text-slate-400 group-open:hidden" />
              <FolderOpen aria-hidden="true" className="hidden size-4 shrink-0 text-slate-400 group-open:block" />
              <span className="truncate">{d.name}</span>
            </summary>
            <Tree dir={d} jump={jump} active={active} review={review} depth={depth + 1} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => (
        <li key={f.path} className="flex items-center gap-1">
          {review.toggle && <input type="checkbox" aria-label={`Revisado: ${f.path}`} title="Revisado" checked={review.reviewed.has(f.path)} onChange={() => review.toggle!(f.path)} className="ml-1 shrink-0" />}
          <button type="button" title={f.path} onClick={() => jump(f.path)} className={`flex min-w-0 flex-1 items-center gap-1 rounded px-1 py-0.5 text-left text-xs hover:bg-slate-100 dark:hover:bg-raised ${review.reviewed.has(f.path) ? "text-slate-400" : ""} ${active === f.path ? "bg-blue-50 font-medium text-blue-700 dark:bg-raised dark:text-accent-text" : ""}`}>
            <FileIcon aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
            <span className="min-w-0 flex-1 truncate">{f.path.split("/").pop()}</span>
            <span className="shrink-0 font-mono text-[0.6875rem] text-green-700 dark:text-green-400">+{f.added}</span>
            <span className="shrink-0 font-mono text-[0.6875rem] text-red-700 dark:text-red-400">−{f.removed}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

const side = (l: Line): Side => (l.kind === "del" ? "LEFT" : "RIGHT");
const number = (l: Line) => (l.kind === "del" ? l.old! : l.new!);
const color = {
  add: "bg-green-50 dark:bg-green-950/60",
  del: "bg-red-50 dark:bg-red-950/60",
  hunk: "border-y border-blue-100 bg-blue-50 text-blue-700 dark:border-border dark:bg-raised dark:text-muted",
  ctx: "",
};
const gutter = { add: "bg-green-100 dark:bg-green-900/40", del: "bg-red-100 dark:bg-red-900/40", hunk: "", ctx: "bg-slate-50 dark:bg-raised" };
const panel = "rounded-lg border border-slate-200 bg-white dark:border-border dark:bg-surface";

const markClass = "rounded-sm bg-amber-300 text-slate-900 dark:bg-amber-500/70 dark:text-white";
const escRe = (t: string) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const escHtml = (t: string) => t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");

// markHtml wraps the search term inside highlighted code, touching only the text between tags.
function markHtml(html: string, q: string) {
  if (!q) return html;
  const re = new RegExp(`(${escRe(escHtml(q))})`, "gi");
  return html.split(/(<[^>]+>)/).map((part) => (part.startsWith("<") ? part : part.replace(re, `<mark class="${markClass}">$1</mark>`))).join("");
}

// markText does the same for plain lines.
function markText(text: string, q: string) {
  if (!q) return text;
  return text.split(new RegExp(`(${escRe(q)})`, "gi")).map((part, i) => (i % 2 ? <mark key={i} className={markClass}>{part}</mark> : part));
}

// Suggestion is a selected range sent to a chat instead of GitHub.
export type Suggestion = { path: string; start: number; end: number; snippet: string; question: string; steer: boolean };

type Choose = (file: string, i: number, shift: boolean) => void;
type Extend = (file: string, i: number) => void;
type Hl = (path: string, i: number, text: string) => string | undefined;
type RowProps = { q: string; hl: Hl; choose: Choose; extend: Extend; touch: { current: boolean } };

const pickedBg = "bg-yellow-100 dark:bg-yellow-900";
const ring = "ring-1 ring-inset ring-amber-400/70";

// UnifiedRow is one line of the unified view; it only re-renders when its own inputs change.
const UnifiedRow = memo(function UnifiedRow({ path, i, l, picked, q, hl, choose, extend, touch }: RowProps & { path: string; i: number; l: Line; picked: boolean }) {
  const h = l.kind === "hunk" ? undefined : hl(path, i, l.text);
  const hit = !!q && l.kind !== "hunk" && l.text.slice(1).toLowerCase().includes(q);
  return (
    <div data-ref={`${path}:${i}`} onMouseEnter={() => l.kind !== "hunk" && extend(path, i)} className={`flex ${picked ? pickedBg : color[l.kind]} ${hit ? ring : ""}`}>
      {l.kind === "hunk" ? null : (
        <button type="button" title="Comentar esta línea (en móvil, toca otra para ampliar el rango)" onPointerDown={(e) => { touch.current = e.pointerType !== "mouse"; }} onMouseDown={(e) => { e.preventDefault(); choose(path, i, e.shiftKey || touch.current); }} onMouseEnter={() => extend(path, i)} className={`flex w-24 shrink-0 touch-manipulation select-none whitespace-nowrap border-r border-slate-200 pr-2 text-right tabular-nums leading-5 text-slate-400 hover:text-blue-600 dark:border-border dark:hover:text-accent-text ${picked ? "" : gutter[l.kind]}`}>
          <span className="w-1/2">{l.old ?? ""}</span><span className="w-1/2">{l.new ?? ""}</span>
        </button>
      )}
      {h !== undefined ? (
        <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2"><span>{l.text[0]}</span><span dangerouslySetInnerHTML={{ __html: markHtml(h, q) }} /></span>
      ) : (
        <span className={`min-w-0 flex-1 whitespace-pre-wrap break-all px-2 ${l.kind === "hunk" ? "pl-3" : ""}`}>{l.kind === "hunk" || !l.text ? l.text || " " : <>{l.text[0]}{markText(l.text.slice(1), q)}</>}</span>
      )}
    </div>
  );
});

// SplitRow is one row of the split view: a removed and an added line side by side, or a hunk header.
const SplitRow = memo(function SplitRow({ path, lines, r, pickedL, pickedR, q, hl, choose, extend, touch }: RowProps & { path: string; lines: Line[]; r: Row; pickedL: boolean; pickedR: boolean }) {
  if (r.hunk !== undefined) return <div className={`whitespace-pre py-0 pl-3 ${color.hunk}`}>{lines[r.hunk].text}</div>;
  const cell = (i: number | undefined, at: "old" | "new", picked: boolean) => {
    if (i === undefined) return <div className="flex min-w-0 bg-slate-50 dark:bg-raised/50"><span className="w-10 shrink-0 border-r border-slate-200 dark:border-border" /></div>;
    const l = lines[i];
    const h = hl(path, i, l.text);
    const hit = !!q && l.text.slice(1).toLowerCase().includes(q);
    return (
      <div data-ref={`${path}:${i}`} onMouseEnter={() => extend(path, i)} className={`flex min-w-0 ${picked ? pickedBg : color[l.kind]} ${hit ? ring : ""}`}>
        <button type="button" title="Comentar esta línea (en móvil, toca otra para ampliar el rango)" onPointerDown={(e) => { touch.current = e.pointerType !== "mouse"; }} onMouseDown={(e) => { e.preventDefault(); choose(path, i, e.shiftKey || touch.current); }} className={`w-10 shrink-0 touch-manipulation select-none border-r border-slate-200 pr-2 text-right text-slate-400 hover:text-blue-600 dark:border-border dark:hover:text-accent-text ${picked ? "" : gutter[l.kind]}`}>{l[at] ?? ""}</button>
        {h !== undefined
          ? <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2"><span>{l.text[0]}</span><span dangerouslySetInnerHTML={{ __html: markHtml(h, q) }} /></span>
          : <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2">{l.text ? <>{l.text[0]}{markText(l.text.slice(1), q)}</> : " "}</span>}
      </div>
    );
  };
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] divide-x divide-slate-200 dark:divide-border">
      {cell(r.left, "old", pickedL)}
      {cell(r.right, "new", pickedR)}
    </div>
  );
});

// FileHead is the header of a file: fold, counters, review mark and comment button.
const FileHead = memo(function FileHead({ f, folded, hits, gh, reviewed, onFold, onReview, onComment }: { f: File; folded: boolean; hits: number; gh: boolean; reviewed: boolean; onFold: (path: string) => void; onReview: (path: string) => void; onComment: (path: string) => void }) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-t-lg border border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-border dark:bg-raised">
      <span className="flex min-w-0 items-center gap-1.5">
        <button type="button" onClick={() => onFold(f.path)} aria-label={folded ? "Desplegar fichero" : "Plegar fichero"} aria-expanded={!folded} className="shrink-0 rounded text-slate-400 hover:text-blue-600">
          {folded ? <CaretRight weight="regular" aria-hidden="true" className="size-3.5" /> : <CaretDown weight="regular" aria-hidden="true" className="size-3.5" />}
        </button>
        <span className="min-w-0 truncate font-mono text-xs font-semibold" title={f.path}>{f.path}</span>
      </span>
      <span className="flex shrink-0 items-center gap-2 font-mono text-xs">
        {hits > 0 && <span className="rounded bg-amber-100 px-1.5 text-amber-800 dark:bg-amber-900/50 dark:text-amber-300">{hits} coincidencia{hits === 1 ? "" : "s"}</span>}
        <span className="text-green-700 dark:text-green-400">+{f.added}</span>
        <span className="text-red-700 dark:text-red-400">−{f.removed}</span>
        {gh && <label className="flex cursor-pointer items-center gap-1 font-sans text-slate-500 dark:text-muted">
          <input type="checkbox" checked={reviewed} onChange={() => onReview(f.path)} />Revisado
        </label>}
        {gh && <button type="button" title="Comentar el fichero" aria-label="Comentar el fichero" onClick={() => onComment(f.path)} className="rounded p-0.5 text-slate-400 hover:bg-slate-200 hover:text-blue-600 dark:hover:bg-border dark:hover:text-accent-text">
          <ChatCircle aria-hidden="true" className="size-4" />
        </button>}
      </span>
    </div>
  );
});

// CommentForm owns the text being typed, so typing never re-renders the diff rows.
// The draft survives the form unmounting when it scrolls out of the virtual list.
const formAction = "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 disabled:hover:bg-transparent dark:text-text-secondary dark:hover:bg-raised";

// CommentForm writes a note on the selected lines. It looks like the reply to
// a quote in the chat: a card with a borderless box that grows, and the
// dictation and main action on its right; the rest goes in a quiet row below.
function CommentForm({ draft, placeholder, suggest, steerable = true, rangeless, pending, agentPending, agentAvailable, onAsk, onSuggest, onPublish, onCancel }: {
  draft: { current: string }; placeholder: string; suggest: boolean; steerable?: boolean; rangeless: boolean; pending: boolean; agentPending: boolean; agentAvailable: boolean;
  onAsk: (body: string) => void; onSuggest: (body: string, steer: boolean) => void; onPublish: (body: string) => void; onCancel: () => void;
}) {
  const [body, setBodyState] = useState(draft.current);
  const setBody = (text: string) => { setBodyState(text); draft.current = text; };
  const input = useRef<HTMLTextAreaElement>(null);
  const dictation = useLocalDictation(body, setBody, input);
  useEnterFinishesDictation(dictation, input);
  const showingDictation = !!dictation.preview;
  useLayoutEffect(() => {
    const area = input.current;
    if (!area || showingDictation) return;
    area.style.height = "auto";
    area.style.height = `${area.scrollHeight}px`;
  }, [body, showingDictation]);
  const empty = !body.trim();
  // A reference to code can go into the message box without a comment.
  const mainDisabled = dictation.active || pending || (empty && (!suggest || steerable)) || (suggest ? rangeless : agentPending);
  const main = () => { if (!mainDisabled) { if (suggest) onSuggest(body, false); else onAsk(body); } };
  return (
    <div data-dictation-panel className="whitespace-normal py-2 font-sans">
      <div className="flex items-start gap-2">
        {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} className="min-w-0 flex-1 !max-h-[7rem] !px-1 !py-1.5 !text-sm !leading-5" />}
        <textarea ref={input} autoFocus rows={1} value={dictation.value} readOnly={dictation.active} aria-label="Comentario de revisión" placeholder={placeholder}
          onFocus={(e) => keepAboveKeyboard(e.currentTarget.parentElement!)}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={(e) => {
            // Enter sends to the agent or chat, like the chat box (Shift+Enter adds a line); Escape cancels. GitHub only by its button.
            if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onCancel(); return; }
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); main(); }
          }}
          className={showingDictation ? "sr-only" : "max-h-[7rem] min-w-0 flex-1 resize-none overflow-y-auto bg-transparent px-1 py-1.5 text-sm leading-5 outline-none! focus-visible:outline-none!"} />
        <button type="button" onClick={onCancel} aria-label="Cancelar" title="Cancelar (Esc)" className="-mr-1 grid size-8 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><X weight="light" className="size-4" /></button>
      </div>
      <div className="mt-1 flex items-center gap-1 border-t border-slate-100 pt-1 dark:border-border">
        {suggest && steerable && <button type="button" disabled={dictation.active || pending || empty || rangeless} onClick={() => onSuggest(body, true)} className={formAction} title="Inyecta el comentario en el turno en curso"><Lightning weight="light" className="size-4" />Steer</button>}
        {!suggest && <PublishButton disabled={dictation.active || pending || empty} onPublish={() => onPublish(body)} />}
        <span className="flex-1" />
        <DictationMic dictation={dictation} />
        <button type="button" disabled={mainDisabled} onClick={main} aria-label={suggest ? (steerable ? "Enviar" : "Añadir al mensaje") : "Preguntar al agente"} className={`${formAction} w-8 justify-center px-0`}
          title={suggest ? (steerable ? "Envía el comentario al hilo; si está trabajando, se encola (Enter)" : "Añade la referencia al mensaje del hilo, sin enviarlo (Enter)") : `${agentAvailable ? "Consulta privada al hilo de revisión de la PR" : "La primera pregunta crea el hilo de revisión de la PR"}; no se publica en GitHub (Enter)`}>
          <ArrowUp weight="light" className="size-4" />
        </button>
      </div>
      <ErrorText error={dictation.error} />
    </div>
  );
}

type Item =
  | { key: string; kind: "head"; fi: number }
  | { key: string; kind: "end"; fi: number }
  | { key: string; kind: "line"; fi: number; i: number }
  | { key: string; kind: "pair"; fi: number; r: Row }
  | { key: string; kind: "form"; fi: number; whole: boolean };
const estimate = { head: 36, end: 12, line: 20, pair: 20, form: 150 };

// Diff shows a pull request diff, or the given diff text when onSuggest sends
// selections to a chat; GitHub reviews and comments only apply to the first.
// FullscreenHost is the element a diff fills in full screen, instead of the window.
export const FullscreenHost = createContext<HTMLElement | null>(null);

// fill makes the diff take its container's height instead of 80vh.
// onAsked runs after a question reaches the pull request's review thread.
// focus opens a file on a line (or range) and scrolls to it, each time its seq changes.
// Only the rows near the viewport are in the DOM; search and jumps work on the parsed files.
export const Diff = memo(function Diff({ repo = "", pr = 0, text, onSuggest, steerable = true, onAsked, focus, fill = false, toolsHost }: { repo?: string; pr?: number; text?: string; onSuggest?: (s: Suggestion) => Promise<void>; steerable?: boolean; onAsked?: () => void; focus?: FileRef & { seq: number }; fill?: boolean; toolsHost?: HTMLElement | null }) {
  const qc = useQueryClient();
  const gh = !onSuggest && !!repo && !!pr;
  const diff = useQuery({ queryKey: qk.githubDiff(repo, pr), queryFn: () => api.githubDiff(repo, pr), enabled: text === undefined });
  const source = text ?? diff.data?.diff ?? "";
  const files = useMemo(() => parseDiff(source), [source]);
  const [filter, setFilter] = useState("");
  const [pick, setPick] = useState<Pick>();
  const draft = useRef(""); // text of the open comment form
  const [drag, setDrag] = useState<{ file: string; from: number }>();
  const [open, setOpen] = useState(false);
  const [full, setFull] = useState(false);
  // The last chosen layout is remembered in this browser for every diff.
  // In a side panel (fill) the file explorer starts closed to leave room for the diff.
  const [panelOpen, setPanelOpen] = useState(!fill);
  const [split, setSplitState] = useState(() => { try { return localStorage.getItem("dory.diff-view") === "split"; } catch { return false; } });
  const setSplit = (f: (v: boolean) => boolean) => setSplitState((v) => {
    const next = f(v);
    try { localStorage.setItem("dory.diff-view", next ? "split" : "unified"); } catch { /* storage unavailable */ }
    return next;
  });
  const [loaded, setLoaded] = useState(0); // bumps when a language finishes loading
  useEffect(() => {
    if (!full) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setFull(false);
    window.addEventListener("keydown", esc);
    // Lock the page behind so only the diff scrolls.
    const html = document.documentElement, prev = [html.style.overflow, document.body.style.overflow];
    html.style.overflow = document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", esc);
      [html.style.overflow, document.body.style.overflow] = prev;
    };
  }, [full]);
  useEffect(() => {
    const langs = new Set(files.map((f) => languageOf(f.path)).filter((l): l is string => !!l));
    langs.forEach((l) => loadLanguage(l).then((ok) => ok && setLoaded((n) => n + 1)));
  }, [files]);
  // Highlighting is lazy: a line is highlighted the first time its row renders.
  const hl = useMemo<Hl>(() => {
    const cache = new Map<string, string | undefined>();
    return (path, i, text) => {
      const key = `${path}:${i}`;
      if (cache.has(key)) return cache.get(key);
      const lang = languageOf(path);
      const out = lang ? highlight(text.slice(1), lang) : undefined;
      cache.set(key, out);
      return out;
    };
  }, [loaded]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const stop = () => setDrag((d) => { if (d) setOpen(true); return undefined; });
    window.addEventListener("mouseup", stop);
    return () => window.removeEventListener("mouseup", stop);
  }, []);
  // Review marks live in Dory and only count while the file patch is unchanged.
  const [hashes, setHashes] = useState(new Map<string, string>());
  useEffect(() => {
    let live = true;
    Promise.all(files.map(async (f) => [f.path, await patchHash(f)] as const)).then((h) => live && setHashes(new Map(h)));
    return () => { live = false; };
  }, [files]);
  const reviews = useQuery({ queryKey: qk.pullReviews(repo, pr), queryFn: () => api.pullReviews(repo, pr), enabled: gh });
  const reviewed = useMemo(() => new Set((reviews.data ?? []).filter((v) => v.patch_hash === hashes.get(v.path)).map((v) => v.path)), [reviews.data, hashes]);
  const mark = useMutation({
    mutationFn: ({ path, on }: { path: string; on: boolean }) => (on ? api.markPullReview(repo, pr, path, hashes.get(path)!) : api.unmarkPullReview(repo, pr, path)),
    onSettled: () => qc.invalidateQueries({ queryKey: qk.pullReviews(repo, pr) }),
  });
  // Reviewed files fold unless the user opens them.
  const [folded, setFolded] = useState(new Map<string, boolean>());
  const isFolded = (path: string) => folded.get(path) ?? reviewed.has(path);
  // Latest state for callbacks that must keep a stable identity, so the memoized rows do not re-render.
  const now = useRef({ pick, drag, reviewed, hashes, mark: mark.mutate, at: new Map<string, number>() });
  now.current = { ...now.current, pick, drag, reviewed, hashes, mark: mark.mutate };
  const scroller = useRef<HTMLDivElement>(null);
  // Focus a referenced file and line: unfold it, highlight the lines and scroll there.
  const focused = useRef(0);
  const want = useRef<{ path: string; i: number }>(undefined);
  const [, wake] = useState(0);
  useEffect(() => {
    if (!focus || focused.current === focus.seq || !files.length) return;
    focused.current = focus.seq;
    const f = files.find((x) => x.path === focus.path) ?? files.find((x) => x.path.endsWith(`/${focus.path}`) || focus.path.endsWith(`/${x.path}`));
    if (!f) return;
    setFolded((m) => new Map(m).set(f.path, false));
    setOpen(false);
    const at = (n: number) => f.lines.findIndex((l) => l.kind !== "hunk" && l.kind !== "del" && l.new === n);
    const start = focus.line ? at(focus.line) : -1;
    const end = focus.end ? Math.max(start, at(focus.end)) : start;
    setPick(start >= 0 ? { file: f.path, start, end } : undefined);
    want.current = { path: f.path, i: start };
    wake((n) => n + 1);
  }, [focus, files]); // eslint-disable-line react-hooks/exhaustive-deps
  const toggleReview = useCallback((path: string) => {
    const n = now.current;
    if (!n.hashes.has(path)) return;
    setFolded((m) => { const x = new Map(m); x.delete(path); return x; });
    n.mark({ path, on: !n.reviewed.has(path) });
  }, []);
  const toggleFold = useCallback((path: string) => setFolded((m) => new Map(m).set(path, !(m.get(path) ?? now.current.reviewed.has(path)))), []);
  const commentFile = useCallback((path: string) => { setPick({ file: path, start: -1, end: -1 }); setOpen(true); }, []);
  const agent = useQuery({ queryKey: qk.pullAgent(repo, pr), queryFn: () => api.pullAgent(repo, pr), enabled: gh });
  // The first question creates the review thread: it waits in launching while
  // the user picks the machine and model.
  const [launching, setLaunching] = useState<{ f: File; p: Pick; body: string }>();
  const done = () => { setPick(undefined); draft.current = ""; };
  const ask = useMutation({
    mutationFn: ({ f, p, body, launch }: { f: File; p: Pick; body: string; launch?: LaunchChoice }) => {
      // A whole-file question goes with lines 0–0 and no snippet.
      if (p.start < 0) return api.askPullAgent(repo, pr, { path: f.path, start: 0, end: 0, snippet: "", question: body.trim(), launch });
      const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
      const nums = lines.map(number);
      return api.askPullAgent(repo, pr, { path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim(), launch });
    },
    onSuccess: () => { done(); setLaunching(undefined); qc.invalidateQueries({ queryKey: qk.pullAgent(repo, pr) }); onAsked?.(); },
  });
  const askOrLaunch = (f: File, p: Pick, body: string) => (agent.data?.available ? ask.mutate({ f, p, body }) : setLaunching({ f, p, body }));
  const suggest = useMutation({
    mutationFn: ({ f, p, body, steer }: { f: File; p: Pick; body: string; steer: boolean }) => {
      const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
      const nums = lines.map(number);
      return onSuggest!({ path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim(), steer });
    },
    onSuccess: done,
  });
  const comment = useMutation({
    mutationFn: (a: Parameters<typeof api.githubAct>[3]) => api.githubAct(repo, "pulls", pr, a),
    onSuccess: () => {
      done();
      qc.invalidateQueries({ queryKey: qk.githubDetail(repo, "pulls", pr) });
    },
  });
  // The search matches file paths and the text of the changed lines; typing stays responsive
  // because the filtering follows a deferred copy of the term.
  const deferred = useDeferredValue(filter);
  const q = deferred.trim().toLowerCase();
  const hits = useMemo(() => new Map(files.map((f) => [f.path, q ? f.lines.filter((l) => l.kind !== "hunk" && l.text.slice(1).toLowerCase().includes(q)).length : 0])), [files, q]);
  const shown = useMemo(() => files.filter((f) => !q || f.path.toLowerCase().includes(q) || hits.get(f.path)! > 0), [files, q, hits]);

  // On touch screens a drag scrolls; tapping a second line number extends the selection instead.
  const touch = useRef(false);
  const choose = useCallback<Choose>((file, i, shift) => {
    const { pick } = now.current;
    const from = shift && pick?.file === file && pick.start >= 0 ? pick.start : i;
    setPick({ file, start: Math.min(from, i), end: Math.max(from, i) });
    // A tap has no drag: the comment box moves when the range changes, and a drag
    // would keep extending to whatever line slides under the finger.
    if (touch.current) { setDrag(undefined); setOpen(true); return; }
    setOpen(false);
    setDrag({ file, from });
  }, []);
  const extend = useCallback<Extend>((file, i) => {
    const { drag, pick } = now.current;
    if (drag?.file !== file) return;
    const start = Math.min(drag.from, i), end = Math.max(drag.from, i);
    if (pick?.file === file && pick.start === start && pick.end === end) return;
    setPick({ file, start, end });
  }, []);
  const publish = (f: File, p: Pick, body: string) => {
    if (p.start < 0) return comment.mutate({ type: "comment_file", body, path: f.path });
    const first = f.lines[p.start], last = f.lines[p.end];
    comment.mutate({ type: "comment_lines", body, path: f.path, line: number(last), side: side(last), start_line: number(first), start_side: side(first) });
  };

  // The flat list the virtual scroller shows: file headers, lines (or split pairs), the open form and file ends.
  const form = open && !drag && pick && pick.start >= 0 ? pick : undefined;
  const whole = pick && pick.start < 0 ? pick.file : undefined;
  const splits = useMemo(() => (split ? new Map(shown.map((f) => [f.path, splitRows(f.lines)])) : undefined), [shown, split]);
  const { items, at } = useMemo(() => {
    const out: Item[] = [];
    const at = new Map<string, number>(); // path → header row, path:i → line row
    shown.forEach((f, fi) => {
      at.set(f.path, out.length);
      out.push({ key: `h:${f.path}`, kind: "head", fi });
      if (whole === f.path) out.push({ key: "form", kind: "form", fi, whole: true });
      if (!(folded.get(f.path) ?? reviewed.has(f.path))) {
        if (splits) {
          splits.get(f.path)!.forEach((r, k) => {
            if (r.left !== undefined) at.set(`${f.path}:${r.left}`, out.length);
            if (r.right !== undefined) at.set(`${f.path}:${r.right}`, out.length);
            out.push({ key: `s:${f.path}:${k}`, kind: "pair", fi, r });
            if (form?.file === f.path && (r.left === form.end || r.right === form.end)) out.push({ key: "form", kind: "form", fi, whole: false });
          });
        } else {
          f.lines.forEach((_, i) => {
            at.set(`${f.path}:${i}`, out.length);
            out.push({ key: `l:${f.path}:${i}`, kind: "line", fi, i });
            if (form?.file === f.path && form.end === i) out.push({ key: "form", kind: "form", fi, whole: false });
          });
        }
      }
      out.push({ key: `e:${f.path}`, kind: "end", fi });
    });
    return { items: out, at };
  }, [shown, splits, folded, reviewed, whole, form?.file, form?.end]); // eslint-disable-line react-hooks/exhaustive-deps
  now.current.at = at;
  const virt = useVirtualizer({
    count: items.length, getScrollElement: () => scroller.current, overscan: 20,
    estimateSize: (i) => estimate[items[i].kind], getItemKey: (i) => items[i]?.key ?? i,
  });
  useEffect(() => {
    const w = want.current;
    if (!w) return;
    want.current = undefined;
    const idx = at.get(w.i >= 0 ? `${w.path}:${w.i}` : w.path);
    if (idx !== undefined) virt.scrollToIndex(idx, { align: "center" });
  });
  const jump = useCallback((path: string) => {
    const idx = now.current.at.get(path);
    if (idx !== undefined) virt.scrollToIndex(idx, { align: "start" });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const vitems = virt.getVirtualItems();
  const offset = virt.scrollOffset ?? 0;
  const topItem = vitems.find((v) => v.end > offset) ?? vitems[0];
  const topFile = topItem && items[topItem.index] ? shown[items[topItem.index].fi] : undefined;
  const treeRoot = useMemo(() => tree(shown), [shown]);
  const review: Review = { reviewed, toggle: gh ? toggleReview : undefined };
  const treeEl = useMemo(() => <Tree dir={treeRoot} jump={jump} active={topFile?.path} review={review} />, [treeRoot, jump, topFile?.path, reviewed, gh]); // eslint-disable-line react-hooks/exhaustive-deps

  const head = (f: File) => <FileHead f={f} folded={isFolded(f.path)} hits={hits.get(f.path) ?? 0} gh={gh} reviewed={reviewed.has(f.path)} onFold={toggleFold} onReview={toggleReview} onComment={commentFile} />;
  const rowProps = { q, hl, choose, extend, touch };
  const content = (it: Item) => {
    const f = shown[it.fi];
    if (it.kind === "head") return head(f);
    if (it.kind === "end") return <div className="pb-3"><div className={`h-2 rounded-b-lg border border-t-0 border-slate-200 bg-white dark:border-border dark:bg-surface`} /></div>;
    const box = "border-x border-slate-200 bg-white dark:border-border dark:bg-surface";
    if (it.kind === "form") {
      const p = pick!;
      const label = it.whole ? `Comentario sobre ${f.path}…` : `Comentario en líneas ${number(f.lines[p.start])}–${number(f.lines[p.end])}…`;
      return (
        <div className={`${box} border-y px-3`}>
          <CommentForm draft={draft} placeholder={label} suggest={!!onSuggest} steerable={steerable} rangeless={p.start < 0} pending={onSuggest ? suggest.isPending : ask.isPending} agentPending={agent.isPending} agentAvailable={!!agent.data?.available}
            onAsk={(body) => askOrLaunch(f, p, body)} onSuggest={(body, steer) => suggest.mutate({ f, p, body, steer })} onPublish={(body) => publish(f, p, body)} onCancel={() => setPick(undefined)} />
        </div>
      );
    }
    const inPick = (i: number | undefined) => i !== undefined && pick?.file === f.path && i >= pick.start && i <= pick.end;
    return (
      <div className={`${box} font-mono text-xs leading-5`}>
        {it.kind === "line"
          ? <UnifiedRow path={f.path} i={it.i} l={f.lines[it.i]} picked={inPick(it.i)} {...rowProps} />
          : <SplitRow path={f.path} lines={f.lines} r={it.r} pickedL={inPick(it.r.left)} pickedR={inPick(it.r.right)} {...rowProps} />}
      </div>
    );
  };

  // While the diff loads, the real layout renders with placeholder bars in place of files and code.
  const loading = text === undefined && diff.isPending;
  const bar = "rounded bg-slate-200 motion-safe:animate-pulse dark:bg-raised";
  const widths = [72, 55, 88, 40, 64, 80, 35, 70, 50, 60];
  const treeSkeleton = (
    <div role="status" aria-label="Cargando ficheros" className="space-y-2 px-1 pt-1">
      {widths.slice(0, 8).map((w, i) => <div key={i} className={`h-3.5 ${bar}`} style={{ width: `${w}%`, marginLeft: i % 3 ? 12 : 0 }} />)}
    </div>
  );
  const filesSkeleton = [0, 1].map((k) => (
    <section key={k} className={panel} aria-hidden="true">
      <div className="flex items-center justify-between gap-2 rounded-t-lg border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-border dark:bg-raised">
        <div className={`h-3.5 w-56 max-w-[60%] ${bar}`} />
        <div className={`h-3.5 w-16 ${bar}`} />
      </div>
      <div className="space-y-2 p-3">
        {widths.map((w, i) => <div key={i} className={`h-3 ${bar}`} style={{ width: `${(w + k * 13) % 90 + 10}%` }} />)}
      </div>
    </section>
  ));

  const toolBtn = "grid h-8 w-8 shrink-0 place-items-center rounded-md border border-slate-300 bg-white hover:bg-slate-100 dark:border-border dark:bg-surface dark:hover:bg-raised";
  const tools = (
    <>
      <button type="button" onClick={() => setPanelOpen((v) => !v)} title={panelOpen ? "Ocultar explorador de archivos" : "Mostrar explorador de archivos"} aria-label={panelOpen ? "Ocultar explorador de archivos" : "Mostrar explorador de archivos"} aria-expanded={panelOpen} className={toolBtn}>
        <TreeView aria-hidden="true" weight={panelOpen ? "fill" : "regular"} className="size-4" />
      </button>
      <button type="button" onClick={() => setSplit((v) => !v)} title={split ? "Vista unificada" : "Vista dividida"} aria-label={split ? "Vista unificada" : "Vista dividida"} aria-pressed={split} className={toolBtn}>
        {split ? <Rows aria-hidden="true" className="size-4" /> : <Columns aria-hidden="true" className="size-4" />}
      </button>
      <button type="button" onClick={() => setFull((v) => !v)} title={full ? "Salir de pantalla completa (Esc)" : "Pantalla completa"} aria-label={full ? "Salir de pantalla completa" : "Pantalla completa"} className={toolBtn}>
        {full ? <ArrowsIn weight="regular" aria-hidden="true" className="size-4" /> : <ArrowsOut weight="regular" aria-hidden="true" className="size-4" />}
      </button>
    </>
  );

  // Full screen fills the host column when the page provides one (the PR page
  // with its chat open), else the whole window.
  const host = useContext(FullscreenHost);
  const view = (
    <div className={full ? `${host ? "absolute" : "fixed h-dvh"} inset-0 z-50 overflow-hidden overscroll-none bg-slate-50 p-3 dark:bg-canvas` : fill ? "h-full min-h-0" : ""}>
    {/* A host (the thread's changes panel) can take the buttons into its own header. */}
    {toolsHost && !full && createPortal(tools, toolsHost)}
    <div className={`grid gap-3 ${panelOpen ? "md:grid-cols-[16rem_1fr]" : toolsHost && !full ? "" : fill && !full ? "md:grid-cols-[3rem_1fr]" : "md:grid-cols-[2.25rem_1fr]"} ${full || fill ? "h-full grid-rows-[auto_minmax(0,1fr)] md:grid-rows-1" : ""}`}>
      {panelOpen ? (
      <aside className={`${full || fill ? "h-[30vh] md:h-auto" : "h-[30vh] md:h-[80vh]"} ${panel} flex min-h-0 flex-col overflow-hidden`}>
          <div className="flex shrink-0 gap-1.5 p-2">
            <Input aria-label="Buscar fichero o texto" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero o texto…" className="min-w-0 flex-1 py-1" />
            {(!toolsHost || full) && tools}
          </div>
          {!loading && gh && <div className="shrink-0 px-3 pb-1 text-xs text-slate-500 dark:text-muted">{files.filter((f) => reviewed.has(f.path)).length}/{files.length} revisados</div>}
          <div className="min-h-0 flex-1 overflow-auto overscroll-contain px-2 pb-2">{loading ? treeSkeleton : treeEl}</div>
      </aside>
      ) : (
      (!toolsHost || full) && <aside className={`flex gap-1.5 md:flex-col ${fill && !full ? "justify-center p-2 md:items-center md:justify-start md:px-0" : ""}`}>{tools}</aside>
      )}
      <div className={`flex min-h-0 min-w-0 flex-col gap-3 ${full || fill ? "" : "h-[80vh]"}`}>
        <ErrorText error={diff.error ?? comment.error ?? ask.error ?? suggest.error ?? mark.error} />
        {launching && <ReviewLaunch repo={repo} pr={pr} busy={ask.isPending} error={ask.error} onClose={() => setLaunching(undefined)} onLaunch={(launch) => ask.mutate({ ...launching, launch })} />}
        <div ref={scroller} className="min-h-0 flex-1 overflow-auto overscroll-contain">
          {loading ? <div className="space-y-3">{filesSkeleton}</div> : (
            <>
              <div className="sticky top-0 z-10 h-0">{topFile && head(topFile)}</div>
              <div className="relative w-full" style={{ height: virt.getTotalSize() }}>
                {vitems.map((v) => items[v.index] && (
                  <div key={v.key} data-index={v.index} ref={virt.measureElement} className="absolute left-0 top-0 w-full" style={{ transform: `translateY(${v.start}px)` }}>
                    {content(items[v.index])}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
    </div>
  );
  return full && host ? createPortal(view, host) : view;
});

// ReviewLaunch picks the agent of the pull request's review thread; the first
// question creates it.
export function ReviewLaunch({ pr, busy, error, onClose, onLaunch }: { repo?: string; pr: number; busy: boolean; error: unknown; onClose: () => void; onLaunch: (l: LaunchChoice) => void }) {
  const form = useLaunchForm({ section: "pull-review" });
  return (
    <Modal title={`Crear el hilo de revisión de #${pr}`} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">Es la primera pregunta sobre esta PR: se crea un hilo para revisarla, sin modificar nada salvo que lo pidas, y recibe tu pregunta. Las siguientes van al mismo hilo.</p>
      <ErrorText error={form.error ?? error} />
      <Button disabled={!form.ready || busy} onClick={() => { form.remember(); onLaunch(form.value); }}>{busy ? "Creando…" : "Crear y preguntar"}</Button>
    </Modal>
  );
}

// PublishButton posts to GitHub only on a second click: the first arms it and
// changes its label, and it disarms by itself after a few seconds.
function PublishButton({ disabled, onPublish }: { disabled: boolean; onPublish: () => void }) {
  const [armed, click] = useArm(onPublish);
  return (
    <button type="button" disabled={disabled} onClick={click} title="Publica el comentario en GitHub; pide confirmación"
      className={`${formAction} ${armed ? "!text-amber-700 dark:!text-amber-400" : ""}`}><GithubLogo weight="light" className="size-4" />{armed ? "Confirmar publicación" : "Publicar en GitHub"}</button>
  );
}
