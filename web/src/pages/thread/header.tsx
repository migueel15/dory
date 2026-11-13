import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowBendUpLeft, Cube, Cloud, FolderSimple, GitBranch, GitPullRequest, Info, Kanban, CaretRight, Check, Copy, Robot, GitDiff, GitMerge } from "@phosphor-icons/react";
import { api, type T3Thread, type T3ThreadView } from "../../api/client";
import { useVisible } from "../../api/live";
import { isUnread, isWorking, subagentStatus, threadMenu, useThreadAction } from "../../api/thread-actions";
import { ContextMenu } from "../../components/context-menu";
import { copyText, errMsg, StatusPill, pill } from "../../components/ui";
import { useArm } from "../../components/github-ui";
import { OpenInT3 } from "../../components/thread-link";
import { statusLabel, ThreadStatus } from "../../components/thread-status";
import { ModelLogo } from "../../components/model-logo";
import { Draft } from "./composer";
import { diffPoll, prOf } from "./changes";

export function ContextBar({ v, children }: { v: T3ThreadView; children?: React.ReactNode }) {
  const threads = useQuery({ queryKey: ["t3-threads"], queryFn: api.t3Threads, refetchInterval: 60_000 });
  const t = threads.data?.flatMap((x) => x.threads).find((x) => x.id === v.id);
  const all = threads.data?.flatMap((x) => x.threads) ?? [];
  const parent = t?.parent_id ? all.find((x) => x.id === t.parent_id) : undefined;
  const act = useThreadAction();
  const [renaming, setRenaming] = useState(false);
  const [menu, setMenu] = useState<{ x: number; y: number }>();
  // Opening a thread marks it read, as in T3 Code.
  const unread = !!t && isUnread(t);
  useEffect(() => { if (unread) act.mutate({ id: v.id, action: "read" }); }, [v.id, unread]); // eslint-disable-line react-hooks/exhaustive-deps
  const rename = (title: string) => { setRenaming(false); if (title && title !== v.title) act.mutate({ id: v.id, action: "rename", title }); };
  return (
    // Fixed row height (h-8), shared with the changes panel's top row, so their buttons line up.
    <header className="relative flex min-h-8 flex-wrap items-center gap-2">
      {/* Breadcrumb: the parent (grey) and the open thread (white), same size.
          Clicking the open thread's title opens its actions. */}
      {parent && (
        <Link to="/threads/$threadId" params={{ threadId: parent.id }} onClick={() => { if (detailsOpen(v.id)) rememberDetails(parent.id, true); }} title="Hilo padre"
          className="inline-flex min-w-0 max-w-[40%] items-center gap-1.5 text-[15px] font-semibold text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
          <span className="truncate">{parent.title || parent.id}</span><CaretRight weight="bold" className="size-3 shrink-0 text-slate-400" />
        </Link>
      )}
      {renaming ? (
        <input autoFocus defaultValue={v.title} aria-label="Nuevo título" onBlur={(e) => rename(e.currentTarget.value.trim())}
          onKeyDown={(e) => { if (e.key === "Enter") rename(e.currentTarget.value.trim()); if (e.key === "Escape") setRenaming(false); }}
          className="min-w-0 flex-1 rounded border border-slate-300 bg-white px-2 py-0.5 text-[15px] font-semibold dark:border-border-strong dark:bg-surface" />
      ) : (
        <h1 className="min-w-0 truncate text-[15px] font-semibold">
          {t ? (
            <button type="button" aria-haspopup="menu" aria-expanded={!!menu} title="Acciones del hilo"
              onClick={(e) => { const b = e.currentTarget.getBoundingClientRect(); setMenu({ x: b.left, y: b.bottom + 4 }); }}
              className="max-w-full truncate rounded px-1 -mx-1 text-left hover:bg-slate-200 dark:hover:bg-raised">{v.title || v.id}</button>
          ) : (v.title || v.id)}
        </h1>
      )}
      {t?.relation === "fork" && <StatusPill tone="slate">Bifurcación</StatusPill>}
      {t?.archived_at && <StatusPill tone="slate">Archivado</StatusPill>}
      {t?.url && <OpenInT3 url={t.url} environmentId={t.t3_environment_id} threadId={v.id} className="mr-1" />}
      {v.pending && <StatusPill tone="amber">Espera tu respuesta</StatusPill>}
      {children}
      {menu && t && <ContextMenu x={menu.x} y={menu.y} label="Acciones del hilo" submenuLabel="Posponer hasta" onClose={() => setMenu(undefined)}
        actions={threadMenu(t, (a) => act.mutate({ id: v.id, ...a }), { onRename: () => setRenaming(true) })} />}
    </header>
  );
}

// repoName turns a repository URL or key into owner/name, lowercase.
export const repoName = (url = "") => url.replace(/\.git$/, "").split("/").slice(-2).join("/").toLowerCase();

// diffStats counts files and added/removed lines in a unified diff.
export const diffStats = (diff = "") => {
  let files = 0, add = 0, del = 0;
  for (const l of diff.split("\n")) {
    if (l.startsWith("diff --git")) files++;
    else if (l.startsWith("+") && !l.startsWith("+++")) add++;
    else if (l.startsWith("-") && !l.startsWith("---")) del++;
  }
  return { files, add, del };
};

// SUBAGENTS_SHOWN is how many subagents the details card lists before "Mostrar N más".
const SUBAGENTS_SHOWN = 5;

// Each thread remembers whether its details card is open (per browser).
const DETAILS_KEY = "dory.thread-details";
const MAX_REMEMBERED = 300;
const detailsMap = (): Record<string, number> => { try { return JSON.parse(localStorage.getItem(DETAILS_KEY) ?? "{}") ?? {}; } catch { return {}; } };
export const detailsOpen = (id: string) => !!detailsMap()[id];
export function rememberDetails(id: string, open: boolean) {
  try {
    const m = detailsMap();
    if (open) m[id] = Date.now(); else delete m[id];
    const keep = Object.entries(m).sort((a, b) => b[1] - a[1]).slice(0, MAX_REMEMBERED);
    localStorage.setItem(DETAILS_KEY, JSON.stringify(Object.fromEntries(keep)));
  } catch { /* per-browser only */ }
}

// ThreadDetails is the header button that shows a card down-left with the
// thread's references, styled like T3 Code's: environment, branch, pull
// requests, items, changes and subagents. The card stays open while the chat
// is used, and each thread remembers it; going to a subagent or to the parent
// from it keeps it open there too.
export function ThreadDetails({ v, onChanges, panelOpen = false }: { v: T3ThreadView; onChanges?: () => void; panelOpen?: boolean }) {
  const [pinnedOpen, setPinnedOpen] = useState(() => detailsOpen(v.id));
  useEffect(() => { setPinnedOpen(detailsOpen(v.id)); }, [v.id]);
  // With the side panel open the card steps aside: the button shows it as a
  // pop-up that a click outside closes, and the remembered state is kept.
  const [popup, setPopup] = useState(false);
  useEffect(() => { setPopup(false); }, [panelOpen, v.id]);
  const open = panelOpen ? popup : pinnedOpen;
  const setOpen = (o: boolean) => {
    if (panelOpen) { setPopup(o); return; }
    rememberDetails(v.id, o); setPinnedOpen(o);
  };
  const keepOpenIn = (id: string) => rememberDetails(id, true);
  const [showDone, setShowDone] = useState(false);
  // At most SUBAGENTS_SHOWN rows at first; "Mostrar N más" lists them all in a scroll.
  const [showAll, setShowAll] = useState(false);
  useEffect(() => { setShowAll(false); }, [v.id]);
  // Subagents of this thread, as T3 Code lists them: the ones at work first,
  // then the finished ones folded away. A subagent leads back to its parent.
  const threads = useQuery({ queryKey: ["t3-threads"], queryFn: api.t3Threads, refetchInterval: 60_000 });
  const everything = threads.data?.flatMap((x) => x.threads) ?? [];
  const self = everything.find((x) => x.id === v.id);
  const parent = self?.parent_id ? everything.find((x) => x.id === self.parent_id) : undefined;
  const subagents = subagentRows(v, everything);
  const live = subagents.filter((x) => x.live);
  const done = subagents.filter((x) => !x.live);
  const liveShown = showAll ? live : live.slice(0, SUBAGENTS_SHOWN);
  const doneShown = showAll ? done : done.slice(0, Math.max(0, SUBAGENTS_SHOWN - liveShown.length));
  const hidden = live.length - liveShown.length + (showDone ? done.length - doneShown.length : 0);
  const box = useRef<HTMLDivElement>(null);
  // Everything loads in the background as the thread opens, so the card is ready
  // when opened; the chat never waits for it.
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list(), staleTime: 5 * 60_000 });
  const first = v.pull_requests.map((p) => prOf(p.url)).find(Boolean);
  // Same keys as the PR page and the changes panel, so the fetches are shared.
  const pull = useQuery({ queryKey: ["github-detail", first?.repo, "pulls", first?.number], queryFn: () => api.githubDetail(first!.repo, "pulls", first!.number), enabled: !!first, staleTime: 30_000 });
  const visible = useVisible();
  const diff = useQuery({ queryKey: ["t3-diff", v.id], queryFn: () => api.t3Diff(v.id), refetchInterval: diffPoll(visible) });
  const envId = t3.data?.find((x) => x.instance_id === v.instance_id)?.environment_id;
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const project = catalog.data?.projects.find((p) => p.id === v.project_id);
  const env = envs.data?.find((e) => e.id === envId);
  const pr = pull.data?.pull;
  // T3 only knows the branch of threads it runs in a worktree; else the PR's head branch fills in.
  // Dory's own git diff of the worktree wins: it includes uncommitted work. Else the PR's numbers.
  const git = diff.data?.source === "git" ? diff.data : undefined;
  const branch = v.branch || git?.branch || pr?.head_ref || "";
  const worktree = v.worktree_path || git?.worktree || "";
  const local = diffStats(diff.data?.diff);
  const changes = git || !pr ? { add: local.add, del: local.del, base: git?.base ?? "", pending: diff.isPending } : { add: pr.additions, del: pr.deletions, base: pr.base_ref, pending: pull.isPending };
  useEffect(() => {
    if (!panelOpen || !popup) return;
    const away = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setPopup(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") setPopup(false); };
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [panelOpen, popup]);
  const ico = "size-[18px] shrink-0 text-slate-500 dark:text-muted";
  const rowCls = "flex min-h-9 items-center gap-3 rounded-md px-2 text-sm";
  return (
    <div ref={box} className="ml-auto flex items-center">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} title="Detalles del hilo" aria-label="Detalles del hilo"
        className={`${pill} !px-2 ${open ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>
        <Info className="size-4" />
        {live.length > 0 && <span className="text-xs tabular-nums text-blue-600 dark:text-blue-400" title={`${live.length} subagentes trabajando`}>{live.length}</span>}
      </button>
      {open && (
        <div role="region" aria-label="Detalles del hilo" className="notif-in absolute right-0 top-full z-50 mt-1.5 w-80 max-w-[calc(100vw-2rem)] origin-top-right divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl dark:divide-border dark:border-border dark:bg-surface">
          <div className="pb-1.5">
            <div className={rowCls}>
              <Cloud className={ico} />
              <span className="min-w-0 flex-1 truncate">{env?.name ?? (envId ? envId.slice(0, 8) : "Entorno desconocido")}</span>
              {worktree && <CopyText text={worktree} hidden icon={<FolderSimple className="size-[18px]" />} title={`Copiar ruta: ${worktree}`} />}
            </div>
          </div>
          <div className="py-1.5">
            <div className={rowCls} title={project?.root}>
              <Cube className={ico} />
              {project ? <span className="min-w-0 flex-1 truncate"><span className="font-medium">{project.title}</span>{project.repo && <span className="text-slate-500 dark:text-muted"> · {repoName(project.repo)}</span>}</span>
                : <span className="flex-1 text-slate-400 dark:text-muted">{catalog.isPending ? "…" : "Sin proyecto"}</span>}
            </div>
            <div className={rowCls}>
              <GitBranch className={ico} />
              {branch ? <span className="min-w-0 flex-1 truncate font-medium" title={branch}>{branch}</span> : <span className="flex-1 text-slate-400 dark:text-muted">Sin rama</span>}
              {branch && <CopyText text={branch} hidden title="Copiar rama" />}
            </div>
            {v.pull_requests.map((p, i) => <PullRow key={p.url} link={p} pull={i === 0 ? pr ?? undefined : undefined} />)}
            {v.items.map((it) => (
              <Link key={it.item_id} to="/items/$id/define" params={{ id: it.item_id }} className={`${rowCls} hover:bg-slate-100 dark:hover:bg-raised`}>
                <Kanban className={ico} />
                <span className="flex-1">Item <span className="font-mono text-xs text-slate-500 dark:text-muted">{it.item_id.slice(0, 8)}</span></span>
                <CaretRight weight="regular" className="size-3.5 text-slate-400" />
              </Link>
            ))}
            <button type="button" disabled={!onChanges} onClick={() => { setPopup(false); onChanges?.(); }} title="Abrir el panel de cambios"
              className={`${rowCls} w-full text-left enabled:hover:bg-slate-100 dark:enabled:hover:bg-raised`}>
              <GitDiff className={ico} />
              <span className="min-w-0 flex-1 truncate"><span className="font-medium">Cambios</span>{changes.base && <span className="text-slate-500 dark:text-muted"> vs {changes.base}</span>}</span>
              {changes.pending ? <span className="text-xs text-slate-400">…</span> : (
                <span className="font-mono text-xs tabular-nums"><span className="text-emerald-600 dark:text-emerald-400">+{changes.add}</span> <span className="text-red-600 dark:text-red-400">−{changes.del}</span></span>
              )}
              {onChanges && <CaretRight weight="regular" className="size-3.5 text-slate-400" />}
            </button>
          </div>
          {(subagents.length > 0 || parent) && (
            <div className="pt-1.5">
              <p className="px-2 pb-1 pt-0.5 text-xs font-semibold text-slate-500 dark:text-muted">Linaje</p>
              {parent && (
                <Link to="/threads/$threadId" params={{ threadId: parent.id }} onClick={() => keepOpenIn(parent.id)} title="Volver al hilo padre"
                  className="flex min-h-8 items-center gap-2.5 rounded-md px-2 text-sm hover:bg-slate-100 dark:hover:bg-raised">
                  <ArrowBendUpLeft className="size-4 shrink-0 text-slate-500 dark:text-muted" />
                  <span className="min-w-0 flex-1 truncate">Volver a <span className="font-medium">{parent.title || parent.id}</span></span>
                </Link>
              )}
              <ul className={showAll ? "max-h-72 overflow-y-auto" : ""}>
                {liveShown.map((x) => <SubagentRow key={x.id} t={x} onOpen={() => keepOpenIn(x.id)} />)}
                {done.length > 0 && (
                  <li>
                    <button type="button" onClick={() => { setShowDone(!showDone); setShowAll(false); }} aria-expanded={showDone}
                      className={`${rowCls} w-full text-left hover:bg-slate-100 dark:hover:bg-raised`}>
                      <Robot className={ico} />
                      <span className="flex-1">Agentes previos</span>
                      <span className="text-xs tabular-nums text-slate-500 dark:text-muted">{done.length}</span>
                      <CaretRight weight="regular" className={`size-3.5 text-slate-400 transition-transform ${showDone ? "rotate-90" : ""}`} />
                    </button>
                    {showDone && <ul>{doneShown.map((x) => <SubagentRow key={x.id} t={x} onOpen={() => keepOpenIn(x.id)} />)}</ul>}
                  </li>
                )}
              </ul>
              {hidden > 0 && (
                <button type="button" onClick={() => setShowAll(true)}
                  className="mt-0.5 w-full rounded-md px-2 py-1 text-left text-xs font-medium text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
                  Mostrar {hidden} más
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// idleFor says how long ago a thread last changed: "hace 5 min".
const idleFor = (iso: string) => {
  const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000);
  if (!Number.isFinite(s)) return "";
  if (s < 60) return "ahora";
  if (s < 3600) return `hace ${Math.floor(s / 60)} min`;
  if (s < 86400) return `hace ${Math.floor(s / 3600)} h`;
  return `hace ${Math.floor(s / 86400)} d`;
};

// Sub is a subagent as the details card shows it. The parent's projection
// has the live state of every agent it launched (native ones included, which
// T3's shell lists as idle); the shell adds the ones it does not track.
export type Sub = { id: string; title: string; provider: string; model: string; status: string; attention: boolean; live: boolean; started: string; ended: string; progress: string };
const liveStatus = (s: string) => isWorking(s) || s === "waiting" || s === "pending";
export function subagentRows(v: T3ThreadView, all: T3Thread[]): Sub[] {
  const out = new Map<string, Sub>();
  for (const a of v.subagents ?? []) {
    out.set(a.thread_id, { id: a.thread_id, title: a.title, provider: a.provider, model: a.model, status: a.status, attention: false, live: liveStatus(a.status),
      started: a.started_at ?? "", ended: a.completed_at ?? "", progress: a.progress ?? "" });
  }
  for (const t of all) {
    if (t.parent_id !== v.id || t.relation !== "subagent") continue;
    const known = out.get(t.id);
    if (known) { out.set(t.id, { ...known, title: known.title || t.title, attention: t.attention, live: known.live || t.attention }); continue; }
    // The parent's projection holds only its latest turns, so older subagents
    // are known from the shell alone, where a native one is always "idle":
    // by then it has finished.
    const status = subagentStatus(t);
    out.set(t.id, { id: t.id, title: t.title, provider: t.provider ?? "", model: t.model, status, attention: t.attention, live: liveStatus(status) || t.attention,
      started: t.run_started_at || t.created_at || "", ended: t.run_completed_at || (isWorking(t.status) ? "" : t.updated_at), progress: "" });
  }
  return [...out.values()].sort((a, b) => (b.ended || b.started).localeCompare(a.ended || a.started));
}

// elapsed writes a duration the short way: "45s", "1m 59s", "1h 3m".
export const elapsed = (secs: number) => {
  const s = Math.max(0, Math.round(secs));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
};

// SubagentRow is one subagent in the details card: model logo, title, what it
// is doing and its state. A working one says for how long; a finished one,
// since when it is inactive.
function SubagentRow({ t, onOpen }: { t: Sub; onOpen: () => void }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!t.live) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [t.live]);
  const start = Date.parse(t.started);
  const note = t.live ? (t.attention ? "Te necesita" : `Trabajando${Number.isFinite(start) ? ` · ${elapsed((now - start) / 1000)}` : ""}`) : t.ended ? idleFor(t.ended) : "";
  return (
    <li>
      <Link to="/threads/$threadId" params={{ threadId: t.id }} onClick={onOpen} title={[t.model, t.progress || statusLabel(t.status, t.attention)].filter(Boolean).join(" · ")}
        className="flex min-h-8 items-center gap-2.5 rounded-md px-2 text-sm hover:bg-slate-100 dark:hover:bg-raised">
        <span className="grid size-[18px] shrink-0 place-items-center [&_svg]:size-4"><ModelLogo driver={t.provider} model={t.model} /></span>
        <span className="min-w-0 flex-1 truncate">{t.title || t.id}</span>
        {note && <span className={`shrink-0 text-[11px] tabular-nums ${t.live ? "text-blue-600 dark:text-blue-400" : "text-slate-400 dark:text-muted"}`}>{note}</span>}
        <ThreadStatus status={t.status} attention={t.attention} />
      </Link>
    </li>
  );
}

// PullRow is a pull request in the details card: its icon shows the state
// (draft, open, merged, closed), and the button moves it on, one step at a
// time and with a second click to confirm: mark a draft ready, merge a ready one.
export function PullRow({ link, pull }: { link: { url: string; number: number; repository: string }; pull?: { title: string; draft: boolean; state: string } }) {
  const qc = useQueryClient();
  const p = prOf(link.url);
  const [owner, repo] = (p?.repo ?? link.repository).split("/");
  const state = pull?.state.toLowerCase() ?? "";
  const look = state === "merged" ? { Glyph: GitMerge, cls: "text-violet-600 dark:text-violet-400", label: "Mergeada" }
    : state === "closed" ? { Glyph: GitPullRequest, cls: "text-red-600 dark:text-red-400", label: "Cerrada" }
    : pull?.draft ? { Glyph: GitPullRequest, cls: "text-slate-400 dark:text-muted", label: "Borrador" }
    : pull ? { Glyph: GitPullRequest, cls: "text-emerald-600 dark:text-emerald-400", label: "Abierta" }
    : { Glyph: GitPullRequest, cls: "text-slate-500 dark:text-muted", label: "" };
  const next = !p || !pull || state !== "open" ? undefined : pull.draft ? { type: "ready", label: "Marcar lista" } : { type: "merge", label: "Mergear" };
  const act = useMutation({
    mutationFn: (type: string) => api.githubAct(p!.repo, "pulls", p!.number, type === "merge" ? { type, method: "MERGE" } : { type }),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["github-detail", p?.repo, "pulls", p?.number] });
      void qc.invalidateQueries({ queryKey: ["github"] });
    },
  });
  const [armed, click] = useArm(() => next && act.mutate(next.type));
  const body = (
    <>
      <look.Glyph weight="bold" className={`mt-0.5 size-[18px] shrink-0 ${look.cls}`} aria-label={look.label || undefined} />
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 font-medium">{pull?.title ?? link.repository}</span>
        <span className="font-mono text-xs text-slate-500 dark:text-muted">#{link.number}{look.label && ` · ${look.label}`}</span>
      </span>
    </>
  );
  const cls = "flex min-w-0 flex-1 items-start gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div>
      <div className="flex items-center gap-1">
        {p ? <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} className={cls} title="Abrir la PR en Dory">{body}</Link>
          : <a href={link.url} target="_blank" rel="noreferrer" className={cls}>{body}</a>}
        {next && (
          <button type="button" onClick={click} disabled={act.isPending} title={armed ? "Pulsa otra vez para confirmar" : next.type === "merge" ? "Mergear la PR (merge commit)" : "Marcar la PR como lista para revisión"}
            className={`shrink-0 rounded-md px-2 py-1 text-xs font-semibold text-white transition-colors disabled:opacity-50 ${armed ? "bg-amber-600" : next.type === "merge" ? "bg-emerald-600 hover:bg-emerald-700" : "bg-blue-600 hover:bg-blue-700"}`}>
            {act.isPending ? "…" : armed ? "¿Confirmar?" : next.label}
          </button>
        )}
      </div>
      {act.error && <p className="px-2 pb-1 text-xs text-red-600 dark:text-red-400">{errMsg(act.error)}</p>}
    </div>
  );
}

// CopyText shows a value with a copy button; hidden shows only the button.
export function CopyText({ text, mono, small, hidden, icon, title = "Copiar" }: { text: string; mono?: boolean; small?: boolean; hidden?: boolean; icon?: React.ReactNode; title?: string }) {
  const [done, setDone] = useState(false);
  const copy = () => void copyText(text).then((ok) => { if (ok) { setDone(true); setTimeout(() => setDone(false), 1200); } });
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      {!hidden && <span className={`min-w-0 truncate ${mono ? "font-mono" : ""} ${small ? "text-xs" : "text-[13px]"}`} title={text}>{text}</span>}
      <button type="button" onClick={copy} title={title} aria-label={title} className="shrink-0 text-slate-400 hover:text-slate-900 dark:hover:text-white">
        {done ? <Check weight="regular" className="size-3.5" /> : icon ?? <Copy className="size-3.5" />}
      </button>
    </span>
  );
}
