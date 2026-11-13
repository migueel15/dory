import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowsIn, ArrowsOutLineVertical, FolderSimple, CaretDown, CaretRight, FileText, GitDiff, X } from "@phosphor-icons/react";
import { api, type T3ChangedFile, type T3ThreadView, type T3ViewMessage } from "../../api/client";
import { useVisible } from "../../api/live";
import { ErrorText, Notice, Select } from "../../components/ui";
import { Diff, type Suggestion } from "../github-diff";

// prOf reads owner/repo and number from a pull request link.
export const prOf = (url: string) => {
  const m = url.match(/github\.com\/([^/]+\/[^/]+)\/pull\/(\d+)/);
  return m ? { repo: m[1], number: Number(m[2]) } : undefined;
};

export const DIFF_POLL_MS = 3000;
// diffPoll polls a local worktree's diff while the tab is on screen: it can
// change from outside the chat, and Dory's git diff answers 304 when nothing
// changed. A diff T3 computes changes only with the thread's turns, and the
// thread's live events already refresh it.
export const diffPoll = (visible: boolean) => (q: { state: { data?: { source?: string } } }) =>
  visible && q.state.data?.source === "git" ? DIFF_POLL_MS : false;

export type TurnTarget = { checkpoint: string; path?: string };
// OpenTurnContext opens the side panel on a turn's changes; null where the chat has no panel.
export const OpenTurnContext = createContext<((t: TurnTarget) => void) | null>(null);

export const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
export const Counts = ({ add, del }: { add: number; del: number }) => (
  <span className="shrink-0 font-mono text-xs tabular-nums"><span className="text-emerald-600 dark:text-emerald-400">+{add}</span> <span className="text-red-600 dark:text-red-400">-{del}</span></span>
);

// ChangesCard lists the files a turn changed, grouped by folder, like T3 Code.
// Clicking a file, a folder or "Abrir diff" opens the side panel on that turn.
export function ChangesCard({ m }: { m: T3ViewMessage }) {
  const open = useContext(OpenTurnContext);
  const files = m.files ?? [];
  const dirs = useMemo(() => {
    const by = new Map<string, T3ChangedFile[]>();
    for (const f of files) {
      const i = f.path.lastIndexOf("/");
      const d = i < 0 ? "" : f.path.slice(0, i);
      by.set(d, [...(by.get(d) ?? []), f]);
    }
    return [...by].sort(([a], [b]) => a.localeCompare(b)).map(([dir, fs]) => ({ dir, files: fs, add: fs.reduce((n, f) => n + f.additions, 0), del: fs.reduce((n, f) => n + f.deletions, 0) }));
  }, [files]);
  const [shown, setShown] = useState<Set<string>>(() => new Set());
  const all = dirs.length > 0 && dirs.every((d) => shown.has(d.dir));
  const toggle = (d: string) => setShown((s) => { const n = new Set(s); if (!n.delete(d)) n.add(d); return n; });
  const add = files.reduce((n, f) => n + f.additions, 0), del = files.reduce((n, f) => n + f.deletions, 0);
  const row = "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 dark:border-border dark:bg-surface">
      <div className="flex items-center gap-3 px-3 py-2.5">
        <span className="text-xs font-semibold text-slate-900 dark:text-white">{plural(files.length, "archivo cambiado", "archivos cambiados")}</span>
        <Counts add={add} del={del} />
        <span className="flex-1" />
        <button type="button" onClick={() => setShown(all ? new Set() : new Set(dirs.map((d) => d.dir)))} title={all ? "Plegar todo" : "Desplegar todo"} aria-label={all ? "Plegar todo" : "Desplegar todo"}
          className="rounded p-1 text-slate-500 hover:bg-slate-200 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
          {all ? <ArrowsIn className="size-3.5" /> : <ArrowsOutLineVertical className="size-3.5" />}
        </button>
        {open && (
          <button type="button" onClick={() => open({ checkpoint: m.id })} className="inline-flex items-center gap-1 rounded px-1.5 py-1 text-xs text-slate-500 hover:bg-slate-200 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
            <GitDiff className="size-3.5" />Abrir diff
          </button>
        )}
      </div>
      <div className="px-1 pb-1.5">
        {dirs.map((d) => (
          <div key={d.dir}>
            <button type="button" onClick={() => toggle(d.dir)} aria-expanded={shown.has(d.dir)} className={`${row} text-slate-500 dark:text-muted`}>
              {shown.has(d.dir) ? <CaretDown className="size-3 shrink-0" /> : <CaretRight className="size-3 shrink-0" />}
              <FolderSimple className="size-3.5 shrink-0" />
              <span className="min-w-0 flex-1 truncate font-mono">{d.dir || "."}</span>
              <Counts add={d.add} del={d.del} />
            </button>
            {shown.has(d.dir) && d.files.map((f) => (
              <button type="button" key={f.path} disabled={!open} onClick={() => open?.({ checkpoint: m.id, path: f.path })} title={f.path}
                className={`${row} pl-9 text-slate-700 disabled:cursor-default disabled:hover:bg-transparent dark:text-text-secondary`}>
                <FileText className="size-3.5 shrink-0" />
                <span className="min-w-0 flex-1 truncate font-mono">{f.path.slice(f.path.lastIndexOf("/") + 1)}</span>
                <Counts add={f.additions} del={f.deletions} />
              </button>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

// Changes shows the thread's own changes or its pull request, and sends a
// selected range to the chat through the same queue or steer as the composer.
export function Changes({ v, queryKey, target, onClose }: { v: T3ThreadView; queryKey: unknown[]; target?: TurnTarget & { seq: number } ; onClose?: () => void }) {
  const qc = useQueryClient();
  const prs = v.pull_requests.map((p) => ({ ...p, parsed: prOf(p.url) })).filter((p) => p.parsed);
  const [source, setSource] = useState(target ? `turn:${target.checkpoint}` : "thread");
  // A changes card in the chat switches the panel to its turn each time it is clicked.
  useEffect(() => { if (target) setSource(`turn:${target.checkpoint}`); }, [target]);
  const turn = source.startsWith("turn:") ? source.slice(5) : "";
  const turnDiff = useQuery({ queryKey: ["t3-turn-diff", v.id, turn], queryFn: () => api.t3Diff(v.id, turn), enabled: !!turn, staleTime: Infinity });
  const focus = useMemo(() => turn && target?.path && target.checkpoint === turn ? { path: target.path, seq: target.seq } : undefined, [turn, target]);
  const pr = prs.find((p) => p.url === source)?.parsed;
  const visible = useVisible();
  const local = useQuery({ queryKey: ["t3-diff", v.id], queryFn: () => api.t3Diff(v.id), enabled: !pr, refetchInterval: diffPoll(visible) });
  // Refresh in place so new messages keep the mounted diff and its scroll.
  useEffect(() => {
    void qc.invalidateQueries({ queryKey: ["t3-diff", v.id] });
  }, [qc, v.id, v.messages.length]);
  const prDiff = useQuery({ queryKey: ["github-diff", pr?.repo, pr?.number], queryFn: () => api.githubDiff(pr!.repo, pr!.number), enabled: !!pr });
  const text = turn ? turnDiff.data?.diff : pr ? prDiff.data?.diff : local.data?.diff;
  const suggest = async (s: Suggestion) => {
    const where = s.start === s.end ? `línea ${s.start}` : `líneas ${s.start}–${s.end}`;
    const body = `Sugerencia sobre \`${s.path}\` (${where}):\n\n\`\`\`diff\n${s.snippet}\n\`\`\`\n\n${s.question}`;
    await api.t3Dispatch(v.id, { text: body, mode: s.steer ? "steer" : "auto" });
    qc.invalidateQueries({ queryKey });
  };
  const [tools, setTools] = useState<HTMLSpanElement | null>(null);
  return (
    <section className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        {onClose && (
          <button type="button" onClick={onClose} title="Cerrar el panel de cambios" aria-label="Cerrar el panel de cambios"
            className="grid h-8 w-8 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><X className="size-4" /></button>
        )}
        <Select className="w-64" value={source} onChange={setSource} options={[
          { value: "thread", label: "Cambios del hilo (worktree)" },
          ...(target ? [{ value: `turn:${target.checkpoint}`, label: "Cambios de este turno" }] : []),
          ...prs.map((p) => ({ value: p.url, label: `PR #${p.number} · ${p.repository}` })),
        ]} />
        {/* The diff's buttons (file explorer, split view, full screen) land here. */}
        <span ref={setTools} className="flex items-center gap-1.5" />
        {v.worktree_path && <span className="min-w-0 truncate font-mono text-xs text-slate-500 dark:text-muted" title={v.worktree_path}>{v.worktree_path}</span>}
      </div>
      <ErrorText error={turn ? turnDiff.error : pr ? prDiff.error : local.error} />
      {text === undefined ? <Notice loading /> : text === "" ? <p className="text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>
        : <div className="min-h-0 flex-1"><Diff key={source} text={text} onSuggest={suggest} focus={focus} fill toolsHost={tools} /></div>}
    </section>
  );
}
