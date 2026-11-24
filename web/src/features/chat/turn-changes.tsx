import { useEffect, useState } from "react";
import { CaretRight, Folder, FolderOpen, GitDiff } from "@phosphor-icons/react";
import type { Message } from "../../api/client";
import { mergeTurnFiles, type TurnFile } from "../../lib/turn-changes";
import { tree, type Dir } from "../github/diff-parse";
import { showPanelTab } from "../thread/side-panel";
import { events } from "../../lib/store";

// A turn's changes: the box at the end of each agent turn, and the filter the
// side panel's "Cambios" tab reads to show one turn instead of the thread.

export type Turn = { id: string; files: TurnFile[] };

// turnFiles reads a turn's changes: the git diff the runtime saves when the
// turn ends, or, for turns from before it, the files its edit calls reported.
export const turnFiles = (ms: Message[]) => {
  const diff = ms.find((m) => m.role === "changes");
  if (diff) return mergeTurnFiles(diff.tool?.files ?? []);
  return mergeTurnFiles(ms.flatMap((m) => (m.tool?.kind === "edit" ? m.tool.files ?? [] : [])));
};

// The filter is per thread and per viewer; the panel listens for its changes.
export type TurnFilter = { turn: string | null; path?: string; seq: number };
const filters = new Map<string, TurnFilter>();
let seq = 0;

export function setTurnFilter(threadId: string, turn: string | null, path?: string) {
  filters.set(threadId, { turn, path, seq: ++seq });
  events.turnFilterChanged.emit(threadId);
}

export function useTurnFilter(threadId: string): TurnFilter {
  const [f, setF] = useState<TurnFilter>(() => filters.get(threadId) ?? { turn: null, seq: 0 });
  useEffect(() => {
    setF(filters.get(threadId) ?? { turn: null, seq: 0 });
    return events.turnFilterChanged.on((id) => { if (id === threadId) setF(filters.get(threadId)!); });
  }, [threadId]);
  return f;
}

const open = (threadId: string, turn: string, path?: string) => { setTurnFilter(threadId, turn, path); showPanelTab(threadId, "diff"); };

// Counts shows only the non-zero sides, so a new file reads "+3", not "+3 −0".
const Counts = ({ added, removed }: { added: number; removed: number }) => (
  <span className="shrink-0 font-mono text-[0.6875rem] leading-4 tabular-nums">
    {added > 0 && <span className="text-green-700 dark:text-green-400">+{added}</span>}
    {added > 0 && removed > 0 && " "}
    {removed > 0 && <span className="text-red-700 dark:text-red-400">−{removed}</span>}
  </span>
);

// Bar is five blocks: how many are lit scales with the size against the
// biggest file of the turn, split green and red by added and removed lines.
function Bar({ added, removed, max, className = "self-center" }: { added: number; removed: number; max: number; className?: string }) {
  const total = added + removed;
  const lit = total ? Math.max(1, Math.round((5 * total) / Math.max(max, 1))) : 0;
  const green = total ? Math.round((lit * added) / total) : 0;
  return (
    <span aria-hidden="true" className={`flex shrink-0 gap-px ${className}`}>
      {Array.from({ length: 5 }, (_, i) => (
        <span key={i} className={`size-1.5 rounded-[1px] ${i < green ? "bg-green-600 dark:bg-green-500" : i < lit ? "bg-red-600 dark:bg-red-500" : "bg-slate-200 dark:bg-raised"}`} />
      ))}
    </span>
  );
}

const STATUS: Record<string, { letter: string; label: string; cls: string }> = {
  add: { letter: "A", label: "nuevo", cls: "text-green-700 dark:text-green-400" },
  delete: { letter: "D", label: "borrado", cls: "text-red-700 dark:text-red-400" },
  rename: { letter: "R", label: "renombrado", cls: "text-sky-700 dark:text-sky-400" },
  update: { letter: "M", label: "modificado", cls: "text-amber-700 dark:text-amber-400" },
};
const status = (f: TurnFile) => STATUS[f.change ?? "update"] ?? STATUS.update;

// summary counts files by status: "3 nuevos · 1 modificado".
function summary(files: TurnFile[]) {
  const by = new Map<string, number>();
  for (const f of files) { const s = status(f).label; by.set(s, (by.get(s) ?? 0) + 1); }
  return [...by].map(([s, n]) => `${n} ${n === 1 ? s : s + "s"}`).join(" · ");
}

// TurnChanges is the box with a turn's files as a folder tree.
export function TurnChanges({ threadId, turn }: { threadId: string; turn: Turn }) {
  const added = turn.files.reduce((n, f) => n + f.added, 0), removed = turn.files.reduce((n, f) => n + f.removed, 0);
  const max = Math.max(0, ...turn.files.map((f) => f.added + f.removed));
  const n = turn.files.length;
  // Clicking the header folds the box to that single line and back; the list
  // stays mounted, so the folders keep whatever the user opened or closed.
  const [folded, setFolded] = useState(false);
  const toggle = () => setFolded((v) => !v);
  const canOpen = !!threadId;
  return (
    <div data-message className="w-full overflow-hidden rounded-xl border border-slate-200 bg-white text-sm dark:border-border dark:bg-surface">
      <div role="button" tabIndex={0} aria-expanded={!folded} onClick={toggle}
        onKeyDown={(e) => { if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); toggle(); } }}
        className={`flex cursor-pointer select-none items-center gap-2 px-3 py-2 hover:bg-slate-50 dark:hover:bg-raised/50 ${folded ? "" : "border-b border-slate-200 dark:border-border"}`}>
        <GitDiff className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="shrink-0 font-medium">{n} {n === 1 ? "fichero" : "ficheros"}</span>
          <Counts added={added} removed={removed} />
          <Bar added={added} removed={removed} max={added + removed} className="" />
          <span className="min-w-0 truncate text-xs text-slate-500 dark:text-muted">{summary(turn.files)}</span>
        </span>
        {canOpen && <button type="button" onClick={(e) => { e.stopPropagation(); open(threadId, turn.id); }}
          className="ml-auto shrink-0 self-center rounded-md px-2 py-1 text-xs font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-text">Abrir diff</button>}
      </div>
      <div hidden={folded} className="max-h-72 overflow-auto p-1.5"><Node dir={tree(turn.files)} max={max} pick={canOpen ? (p) => open(threadId, turn.id, p) : undefined} /></div>
    </div>
  );
}

function Row({ f, prefix, max, pick }: { f: TurnFile; prefix?: string; max: number; pick?: (path: string) => void }) {
  const s = status(f);
  return (
    <button type="button" title={`${f.path} (${s.label})`} disabled={!pick} onClick={() => pick?.(f.path)} className="flex w-full min-w-0 items-baseline gap-2 rounded px-1 py-0.5 text-left text-xs leading-4 enabled:hover:bg-slate-100 dark:enabled:hover:bg-raised">
      <span className={`w-3 shrink-0 text-center font-mono text-[0.6875rem] font-semibold leading-4 ${s.cls}`}>{s.letter}</span>
      <span className={`min-w-0 flex-1 truncate ${f.change === "delete" ? "text-slate-400 line-through" : ""}`}>
        {prefix && <span className="text-slate-400 dark:text-muted">{prefix}/</span>}{f.path.split("/").pop()}
      </span>
      <Counts added={f.added} removed={f.removed} />
      <Bar added={f.added} removed={f.removed} max={max} />
    </button>
  );
}

// Node draws a folder; a folder with a single file collapses into one row.
function Node({ dir, max, pick, depth = 0 }: { dir: Dir<TurnFile>; max: number; pick?: (path: string) => void; depth?: number }) {
  return (
    <ul className={depth ? "ml-2 border-l border-slate-200 pl-1 dark:border-border" : ""}>
      {dir.dirs.map((d) => d.dirs.length === 0 && d.files.length === 1 ? (
        <li key={d.name}><Row f={d.files[0]} prefix={d.name} max={max} pick={pick} /></li>
      ) : (
        <li key={d.name}>
          <details className="group">
            <summary title={d.name} className="flex cursor-pointer list-none items-center gap-1 rounded px-1 py-0.5 text-xs font-medium text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised [&::-webkit-details-marker]:hidden">
              <CaretRight aria-hidden="true" className="size-3 shrink-0 text-slate-400 transition-transform group-open:rotate-90" />
              <Folder aria-hidden="true" className="size-4 shrink-0 text-slate-400 group-open:hidden" />
              <FolderOpen aria-hidden="true" className="hidden size-4 shrink-0 text-slate-400 group-open:block" />
              <span className="min-w-0 truncate">{d.name}/</span>
            </summary>
            <Node dir={d} max={max} pick={pick} depth={depth + 1} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => <li key={f.path}><Row f={f} max={max} pick={pick} /></li>)}
    </ul>
  );
}
