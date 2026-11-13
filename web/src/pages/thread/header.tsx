import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowSquareOut, Cube, Cloud, FolderSimple, GitBranch, GitPullRequest, Info, Kanban, CaretRight, Check, Copy, DotsThree, Robot, GitDiff } from "@phosphor-icons/react";
import { api, type T3Thread, type T3ThreadView } from "../../api/client";
import { useVisible } from "../../api/live";
import { isUnread, isWorking, threadMenu, useThreadAction } from "../../api/thread-actions";
import { ContextMenu } from "../../components/context-menu";
import { copyText, StatusPill, pill } from "../../components/ui";
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
    <header className="relative flex flex-wrap items-center gap-2">
      {parent && (
        <Link to="/threads/$threadId" params={{ threadId: parent.id }} title="Hilo padre" className="inline-flex max-w-[12rem] items-center gap-1 truncate text-sm text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
          <span className="truncate">{parent.title}</span><CaretRight className="size-3 shrink-0" />
        </Link>
      )}
      {renaming ? (
        <input autoFocus defaultValue={v.title} aria-label="Nuevo título" onBlur={(e) => rename(e.currentTarget.value.trim())}
          onKeyDown={(e) => { if (e.key === "Enter") rename(e.currentTarget.value.trim()); if (e.key === "Escape") setRenaming(false); }}
          className="min-w-0 flex-1 rounded border border-slate-300 bg-white px-2 py-0.5 text-lg font-semibold dark:border-border-strong dark:bg-surface" />
      ) : (
        <h1 className="min-w-0 cursor-text truncate text-lg font-semibold" title="Doble clic para renombrar" onDoubleClick={() => t && setRenaming(true)}>{v.title || v.id}</h1>
      )}
      {t?.relation === "subagent" && <StatusPill tone="slate">Subagente</StatusPill>}
      {t?.relation === "fork" && <StatusPill tone="slate">Bifurcación</StatusPill>}
      {t?.archived_at && <StatusPill tone="slate">Archivado</StatusPill>}
      {t?.url && <OpenInT3 url={t.url} environmentId={t.t3_environment_id} threadId={v.id} className="mr-1" />}
      {v.pending && <StatusPill tone="amber">Espera tu respuesta</StatusPill>}
      {t && (
        <button type="button" title="Acciones del hilo" aria-label="Acciones del hilo" className={`${pill} !px-2`}
          onClick={(e) => { const b = e.currentTarget.getBoundingClientRect(); setMenu({ x: b.left, y: b.bottom + 4 }); }}><DotsThree weight="bold" className="size-4" /></button>
      )}
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

// ThreadDetails is the header button that drops a card down-left with the
// thread's references, styled like T3 Code's: environment, branch, pull
// requests, items and changes.
export function ThreadDetails({ v }: { v: T3ThreadView }) {
  const [open, setOpen] = useState(false);
  const [showDone, setShowDone] = useState(false);
  // Subagents of this thread, as T3 Code lists them: the ones at work first,
  // then the finished ones folded away.
  const threads = useQuery({ queryKey: ["t3-threads"], queryFn: api.t3Threads, refetchInterval: 60_000 });
  const subagents = (threads.data?.flatMap((x) => x.threads) ?? [])
    .filter((x) => x.parent_id === v.id && x.relation === "subagent")
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const live = subagents.filter((x) => isWorking(x.status) || x.attention);
  const done = subagents.filter((x) => !live.includes(x));
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
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [open]);
  const ico = "size-[18px] shrink-0 text-slate-500 dark:text-muted";
  const rowCls = "flex min-h-9 items-center gap-3 rounded-md px-2 text-sm";
  return (
    <div ref={box} className="ml-auto">
      <button type="button" aria-expanded={open} aria-haspopup="dialog" onClick={() => setOpen(!open)} title="Detalles del hilo" aria-label="Detalles del hilo"
        className={`${pill} !px-2 ${open ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>
        <Info className="size-4" />
        {live.length > 0 && <span className="text-xs tabular-nums text-blue-600 dark:text-blue-400" title={`${live.length} subagentes trabajando`}>{live.length}</span>}
      </button>
      {open && (
        <div role="dialog" aria-label="Detalles del hilo" className="notif-in absolute right-0 top-full z-50 mt-1.5 w-80 max-w-[calc(100vw-2rem)] origin-top-right divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl dark:divide-border dark:border-border dark:bg-surface">
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
            {v.pull_requests.map((p, i) => <PullRow key={p.url} link={p} pull={i === 0 ? pr ?? undefined : undefined} onOpen={() => setOpen(false)} />)}
            {v.items.map((it) => (
              <Link key={it.item_id} to="/items/$id/define" params={{ id: it.item_id }} onClick={() => setOpen(false)} className={`${rowCls} hover:bg-slate-100 dark:hover:bg-raised`}>
                <Kanban className={ico} />
                <span className="flex-1">Item <span className="font-mono text-xs text-slate-500 dark:text-muted">{it.item_id.slice(0, 8)}</span></span>
                <CaretRight weight="regular" className="size-3.5 text-slate-400" />
              </Link>
            ))}
          </div>
          <div className="pt-1.5">
            <div className={rowCls}>
              <GitDiff className={ico} />
              <span className="min-w-0 flex-1 truncate"><span className="font-medium">Cambios</span>{changes.base && <span className="text-slate-500 dark:text-muted"> vs {changes.base}</span>}</span>
              {changes.pending ? <span className="text-xs text-slate-400">…</span> : (
                <span className="font-mono text-xs tabular-nums"><span className="text-emerald-600 dark:text-emerald-400">+{changes.add}</span> <span className="text-red-600 dark:text-red-400">−{changes.del}</span></span>
              )}
            </div>
          </div>
          {subagents.length > 0 && (
            <div className="pt-1.5">
              <div className={`${rowCls} min-h-8`}>
                <Robot className={ico} />
                <span className="flex-1 font-medium">Subagentes</span>
                <span className="text-xs tabular-nums text-slate-500 dark:text-muted">{live.length ? `${live.length} activos` : "ninguno activo"}</span>
              </div>
              <ul className="max-h-72 overflow-y-auto">
                {live.map((x) => <SubagentRow key={x.id} t={x} onOpen={() => setOpen(false)} />)}
                {done.length > 0 && (
                  <li>
                    <button type="button" onClick={() => setShowDone(!showDone)} aria-expanded={showDone}
                      className="flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
                      <CaretRight weight="bold" className={`size-3 transition-transform ${showDone ? "rotate-90" : ""}`} />Finalizados<span className="tabular-nums">{done.length}</span>
                    </button>
                    {showDone && <ul>{done.map((x) => <SubagentRow key={x.id} t={x} onOpen={() => setOpen(false)} />)}</ul>}
                  </li>
                )}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// SubagentRow is one subagent in the details card: model logo, title and state.
function SubagentRow({ t, onOpen }: { t: T3Thread; onOpen: () => void }) {
  return (
    <li>
      <Link to="/threads/$threadId" params={{ threadId: t.id }} onClick={onOpen} title={[t.model, statusLabel(t.status, t.attention)].filter(Boolean).join(" · ")}
        className="flex min-h-8 items-center gap-2.5 rounded-md px-2 text-sm hover:bg-slate-100 dark:hover:bg-raised">
        <span className="grid size-[18px] shrink-0 place-items-center [&_svg]:size-4"><ModelLogo driver={t.provider ?? ""} model={t.model} /></span>
        <span className="min-w-0 flex-1 truncate">{t.title || t.id}</span>
        <ThreadStatus status={t.status} attention={t.attention} />
      </Link>
    </li>
  );
}

export const prBadge = (p?: { draft: boolean; state: string }) => {
  if (!p) return null;
  const s = p.state.toLowerCase();
  const [label, cls] = s === "merged" ? ["Merged", "bg-violet-600"] : s === "closed" ? ["Closed", "bg-red-600"] : p.draft ? ["Draft", "bg-slate-500"] : ["Ready", "bg-blue-600"];
  return <span className={`rounded-md px-2 py-0.5 text-xs font-semibold text-white ${cls}`}>{label}</span>;
};

// PullRow is a pull request in the details card: title, number and state;
// it opens the PR inside Dory, and the arrow opens it on GitHub.
export function PullRow({ link, pull, onOpen }: { link: { url: string; number: number; repository: string }; pull?: { title: string; draft: boolean; state: string }; onOpen: () => void }) {
  const p = prOf(link.url);
  const [owner, repo] = (p?.repo ?? link.repository).split("/");
  const body = (
    <>
      <GitPullRequest className="mt-0.5 size-[18px] shrink-0 text-slate-500 dark:text-muted" />
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 font-medium">{pull?.title ?? link.repository}</span>
        <span className="mt-1 flex items-center gap-2">
          <span className="font-mono text-xs text-slate-500 dark:text-muted">#{link.number}</span>
          <span className="ml-auto">{prBadge(pull)}</span>
        </span>
      </span>
    </>
  );
  const cls = "flex min-w-0 flex-1 items-start gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div className="flex items-start gap-0.5">
      {p ? <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} onClick={onOpen} className={cls} title="Abrir la PR en Dory">{body}</Link>
        : <a href={link.url} target="_blank" rel="noreferrer" className={cls}>{body}</a>}
      <a href={link.url} target="_blank" rel="noreferrer" title="Abrir en GitHub" aria-label="Abrir en GitHub"
        className="mt-1 grid size-7 shrink-0 place-items-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-raised dark:hover:text-white"><ArrowSquareOut className="size-4" /></a>
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
