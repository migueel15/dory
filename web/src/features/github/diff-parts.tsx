import { memo, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Lightning, X, CaretDown, CaretRight, ChatCircle, File as FileIcon, Folder, FolderOpen } from "@phosphor-icons/react";
import { useLocalDictation } from "../composer/local-dictation";
import { autosizeTextarea } from "../composer/composer-styles";
import { DictationMic, DictationPreview, useEnterFinishesDictation } from "../composer/dictation-ui";
import { ErrorText, keepAboveKeyboard } from "../../components/ui";
import { PublishButton } from "./diff-review";
import { type Dir, type Review, type Line, type Side, type Row, type File } from "./diff-parse";

export function Tree({ dir, jump, active, review, depth = 0 }: { dir: Dir; jump: (path: string) => void; active?: string; review: Review; depth?: number }) {
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

export const side = (l: Line): Side => (l.kind === "del" ? "LEFT" : "RIGHT");
export const number = (l: Line) => (l.kind === "del" ? l.old! : l.new!);
export const color = {
  add: "bg-green-50 dark:bg-green-950/60",
  del: "bg-red-50 dark:bg-red-950/60",
  hunk: "border-y border-blue-100 bg-blue-50 text-blue-700 dark:border-border dark:bg-raised dark:text-muted",
  ctx: "",
};
export const gutter = { add: "bg-green-100 dark:bg-green-900/40", del: "bg-red-100 dark:bg-red-900/40", hunk: "", ctx: "bg-slate-50 dark:bg-raised" };
export const panel = "rounded-lg border border-slate-200 bg-white dark:border-border dark:bg-surface";

export const markClass = "rounded-sm bg-amber-300 text-slate-900 dark:bg-amber-500/70 dark:text-white";
export const escRe = (t: string) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
export const escHtml = (t: string) => t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");

// markHtml wraps the search term inside highlighted code, touching only the text between tags.
export function markHtml(html: string, q: string) {
  if (!q) return html;
  const re = new RegExp(`(${escRe(escHtml(q))})`, "gi");
  return html.split(/(<[^>]+>)/).map((part) => (part.startsWith("<") ? part : part.replace(re, `<mark class="${markClass}">$1</mark>`))).join("");
}

// markText does the same for plain lines.
export function markText(text: string, q: string) {
  if (!q) return text;
  return text.split(new RegExp(`(${escRe(q)})`, "gi")).map((part, i) => (i % 2 ? <mark key={i} className={markClass}>{part}</mark> : part));
}

// Suggestion is a selected range sent to a chat instead of GitHub.
export type Suggestion = { path: string; start: number; end: number; snippet: string; question: string; steer: boolean };

export type Choose = (file: string, i: number, shift: boolean) => void;
export type Extend = (file: string, i: number) => void;
export type Hl = (path: string, i: number, text: string) => string | undefined;
export type RowProps = { q: string; hl: Hl; choose: Choose; extend: Extend; touch: { current: boolean } };

export const pickedBg = "bg-yellow-100 dark:bg-yellow-900";
export const ring = "ring-1 ring-inset ring-amber-400/70";

// UnifiedRow is one line of the unified view; it only re-renders when its own inputs change.
export const UnifiedRow = memo(function UnifiedRow({ path, i, l, picked, q, hl, choose, extend, touch }: RowProps & { path: string; i: number; l: Line; picked: boolean }) {
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
export const SplitRow = memo(function SplitRow({ path, lines, r, pickedL, pickedR, q, hl, choose, extend, touch }: RowProps & { path: string; lines: Line[]; r: Row; pickedL: boolean; pickedR: boolean }) {
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
export const FileHead = memo(function FileHead({ f, folded, hits, gh, reviewed, onFold, onReview, onComment }: { f: File; folded: boolean; hits: number; gh: boolean; reviewed: boolean; onFold: (path: string) => void; onReview: (path: string) => void; onComment: (path: string) => void }) {
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
export const formAction = "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 disabled:hover:bg-transparent dark:text-text-secondary dark:hover:bg-raised";

// CommentForm writes a note on the selected lines. It looks like the reply to
// a quote in the chat: a card with a borderless box that grows, and the
// dictation and main action on its right; the rest goes in a quiet row below.
export function CommentForm({ draft, placeholder, suggest, steerable = true, rangeless, pending, agentPending, agentAvailable, onAsk, onSuggest, onPublish, onCancel }: {
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
    autosizeTextarea(area);
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
        <button type="button" onClick={onCancel} aria-label="Cancelar" title="Cancelar (Esc)" className="-mr-1 grid size-8 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><X weight="regular" className="size-4" /></button>
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

