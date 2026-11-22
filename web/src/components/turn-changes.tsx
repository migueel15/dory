import { useEffect, useState } from "react";
import { CaretRight, File as FileIcon, Folder, FolderOpen, GitDiff } from "@phosphor-icons/react";
import type { Message } from "../api/client";
import { mergeTurnFiles, type TurnFile } from "../lib/turn-changes";
import { tree, type Dir } from "../pages/github-diff";
import { showPanelTab } from "./side-panel";

// A turn's changes: the box at the end of each agent turn, and the filter the
// side panel's "Cambios" tab reads to show one turn instead of the thread.

export type Turn = { id: string; files: TurnFile[] };

// turnFiles gathers the files every edit call of a turn reported.
export const turnFiles = (ms: Message[]) => mergeTurnFiles(ms.flatMap((m) => (m.tool?.kind === "edit" ? m.tool.files ?? [] : [])));

// The filter is per thread and per viewer; the panel listens for its changes.
export type TurnFilter = { turn: string | null; path?: string; seq: number };
const filters = new Map<string, TurnFilter>();
const EVENT = "dory-turn-filter";
let seq = 0;

export function setTurnFilter(threadId: string, turn: string | null, path?: string) {
  filters.set(threadId, { turn, path, seq: ++seq });
  window.dispatchEvent(new CustomEvent(EVENT, { detail: threadId }));
}

export function useTurnFilter(threadId: string): TurnFilter {
  const [f, setF] = useState<TurnFilter>(() => filters.get(threadId) ?? { turn: null, seq: 0 });
  useEffect(() => {
    setF(filters.get(threadId) ?? { turn: null, seq: 0 });
    const on = (e: Event) => { if ((e as CustomEvent).detail === threadId) setF(filters.get(threadId)!); };
    window.addEventListener(EVENT, on);
    return () => window.removeEventListener(EVENT, on);
  }, [threadId]);
  return f;
}

const open = (threadId: string, turn: string, path?: string) => { setTurnFilter(threadId, turn, path); showPanelTab(threadId, "diff"); };

const Counts = ({ added, removed }: { added: number; removed: number }) => (
  <span className="shrink-0 font-mono text-[0.6875rem] tabular-nums"><span className="text-green-700 dark:text-green-400">+{added}</span> <span className="text-red-700 dark:text-red-400">−{removed}</span></span>
);

// TurnChanges is the box with a turn's files as a folder tree.
export function TurnChanges({ threadId, turn }: { threadId: string; turn: Turn }) {
  const added = turn.files.reduce((n, f) => n + f.added, 0), removed = turn.files.reduce((n, f) => n + f.removed, 0);
  const n = turn.files.length;
  const canOpen = !!threadId;
  return (
    <div data-message className="w-full overflow-hidden rounded-xl border border-slate-200 bg-white text-sm dark:border-border dark:bg-surface">
      <div className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-border dark:bg-raised/50">
        <GitDiff className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="min-w-0 truncate font-medium">{n} {n === 1 ? "fichero cambiado" : "ficheros cambiados"}</span>
        <Counts added={added} removed={removed} />
        {canOpen && <button type="button" onClick={() => open(threadId, turn.id)}
          className="ml-auto shrink-0 rounded-md border border-slate-200 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100 dark:border-border dark:text-text-secondary dark:hover:bg-raised">Abrir diff</button>}
      </div>
      <div className="max-h-72 overflow-auto p-1.5"><Node dir={tree(turn.files)} pick={canOpen ? (p) => open(threadId, turn.id, p) : undefined} /></div>
    </div>
  );
}

function Node({ dir, pick, depth = 0 }: { dir: Dir<TurnFile>; pick?: (path: string) => void; depth?: number }) {
  return (
    <ul className={depth ? "ml-3 border-l border-slate-200 pl-1 dark:border-border" : ""}>
      {dir.dirs.map((d) => (
        <li key={d.name}>
          <details open className="group">
            <summary title={d.name} className="flex cursor-pointer list-none items-center gap-1 rounded px-1 py-0.5 text-xs font-medium hover:bg-slate-100 dark:hover:bg-raised [&::-webkit-details-marker]:hidden">
              <CaretRight aria-hidden="true" className="size-3 shrink-0 text-slate-400 transition-transform group-open:rotate-90" />
              <Folder aria-hidden="true" className="size-4 shrink-0 text-slate-400 group-open:hidden" />
              <FolderOpen aria-hidden="true" className="hidden size-4 shrink-0 text-slate-400 group-open:block" />
              <span className="min-w-0 truncate">{d.name}</span>
            </summary>
            <Node dir={d} pick={pick} depth={depth + 1} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => (
        <li key={f.path}>
          <button type="button" title={f.path} disabled={!pick} onClick={() => pick?.(f.path)} className="flex w-full min-w-0 items-center gap-1.5 rounded px-1 py-0.5 text-left text-xs enabled:hover:bg-slate-100 dark:enabled:hover:bg-raised">
            <FileIcon aria-hidden="true" className="ml-4 size-4 shrink-0 text-slate-400" />
            <span className={`min-w-0 flex-1 truncate ${f.change === "delete" ? "text-slate-400 line-through" : ""}`}>{f.path.split("/").pop()}</span>
            {f.change === "add" && <span className="shrink-0 rounded bg-green-100 px-1 text-[0.625rem] font-semibold text-green-800 dark:bg-green-900/40 dark:text-green-300">nuevo</span>}
            {f.change === "delete" && <span className="shrink-0 rounded bg-red-100 px-1 text-[0.625rem] font-semibold text-red-800 dark:bg-red-900/40 dark:text-red-300">borrado</span>}
            <Counts added={f.added} removed={f.removed} />
          </button>
        </li>
      ))}
    </ul>
  );
}
