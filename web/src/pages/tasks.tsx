import { DictationField } from "../components/dictation-field";
import { useGithubWatch } from "../api/live";
import { PlanDoc } from "../components/plan-doc";
import { useDictationInProgress } from "../components/local-dictation";
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { Check, CaretDown, CaretRight, PencilSimple, Chats, GithubLogo, LinkSimple, CircleNotch, ClipboardText, DotsThree, GitBranch, GitMerge, GitPullRequest, ChatCircle, CircleDashed, Plus, Rocket, X } from "@phosphor-icons/react";
import { api, COLUMN_LABELS, repoName, type Card, type Column, type Pull, type Stage, type Thread } from "../api/client";
import { Button, ErrorText, iconButton, Input, Label, Md, Modal, Select } from "../components/ui";
import { useThreadActivity } from "../api/activity";
import { useArm } from "../components/github-ui";
import { useLaunchForm } from "../components/launch-picker";
import { useProject } from "../project";
import { ModelLogo } from "../components/model-logo";
import { askConfirm } from "../components/confirm";
import { IdeaModal } from "../components/capture";
import { TaskQuickMenu, type QuickLaunch } from "../components/task-quick-menu";
import { githubRoute } from "../components/refs";
import { planMessage } from "../lib/auto-message";

// Fixed meaning of each state color (requirement RF-48).
const TONE = {
  working: "text-blue-500", attention: "text-orange-500", ci: "text-yellow-500", review: "text-amber-500",
  ready: "text-emerald-500", merged: "text-purple-500", failed: "text-red-500", idle: "text-slate-400",
};

// slot is the single state icon of a card (requirement RF-47), with its meaning for the tooltip.
function slot(c: Card): { icon: React.ReactNode; label: string } {
  if (c.threads.some((t) => t.status === "working")) return { icon: <CircleNotch weight="regular" className={`size-4 animate-spin ${TONE.working}`} />, label: "Un agente está trabajando" };
  const pull = c.pull;
  if (c.pr_state === "merged") return { icon: <GitMerge className={`size-4 ${TONE.merged}`} weight="fill" />, label: "PR fusionada" };
  if (c.pr_state === "closed") return { icon: <GitPullRequest className={`size-4 ${TONE.idle}`} />, label: "PR cerrada sin fusionar" };
  if (pull) {
    if (pull.ci === "FAILURE" || pull.ci === "ERROR") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "CI fallido" };
    if (pull.ci === "PENDING" || pull.ci === "EXPECTED") return { icon: <GitPullRequest className={`size-4 ${TONE.ci}`} weight="fill" />, label: "CI en ejecución" };
    if (pull.mergeable === "CONFLICTING") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "Conflictos de merge" };
    if (pull.review_decision === "CHANGES_REQUESTED" || pull.review_requests.length > 0) return { icon: <GitPullRequest className={`size-4 ${TONE.review}`} weight="fill" />, label: pull.review_decision === "CHANGES_REQUESTED" ? "Cambios pedidos" : "Review pendiente" };
    if (pull.draft) return { icon: <GitPullRequest className={`size-4 ${TONE.working}`} />, label: "PR en draft" };
    return { icon: <GitPullRequest className={`size-4 ${TONE.ready}`} weight="fill" />, label: "Lista para mergear" };
  }
  if (c.threads.some((t) => t.status === "waiting")) return { icon: <span className="size-2.5 rounded-full bg-orange-500" />, label: "Necesita tu respuesta" };
  if (c.threads.some((t) => t.status === "error")) return { icon: <span className="size-2.5 rounded-full bg-red-500" />, label: "El hilo falló" };
  return { icon: <span className="size-2.5 rounded-full bg-slate-300 dark:bg-slate-600" />, label: "Inactivo" };
}

// Only these columns can be chosen by hand; the rest come from threads and pull requests.
const MANUAL: Partial<Record<Column, Stage>> = { inbox: "inbox", ready: "ready", cancelled: "cancelled" };
// COLUMNS draws the empty board before the first answer arrives.
const COLUMNS: Column[] = ["inbox", "ready", "working", "needs_you", "in_review", "ready_to_merge", "done", "cancelled"];
// MAIN are the columns drawn as the board; done and cancelled are summarised in the footer.
const MAIN: Column[] = ["inbox", "ready", "working", "needs_you", "in_review", "ready_to_merge"];
const ARCHIVED: Column[] = ["done", "cancelled"];
const DOT: Record<Column, string> = {
  inbox: "bg-slate-400", ready: "bg-indigo-500", working: "bg-blue-500", needs_you: "bg-red-500",
  in_review: "bg-amber-500", ready_to_merge: "bg-emerald-500", done: "bg-emerald-500", cancelled: "bg-slate-400",
};
// Cards take the height their content needs; the column measures them to know how many fit.
const MORE_H = 32;
const GAP = 8;
const PAD = 8;

// ago is the short relative time of the last activity.
function ago(iso?: string | null): string {
  const t = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(t)) return "";
  const m = Math.floor((Date.now() - t) / 60_000);
  if (m < 1) return "ahora";
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} h`;
  return `${Math.floor(h / 24)} d`;
}

const lastActivity = (c: Card) => c.threads.reduce((a, t) => { const x = t.last_activity_at ?? t.updated_at; return x > a ? x : a; }, c.updated_at);

// status is the colored text at the foot of a card. busy marks work in progress on GitHub (checks running).
function status(c: Card): { text: string; cls: string; busy?: boolean } {
  if (c.blockers.length > 0) return { text: "Bloqueada", cls: "text-red-600 dark:text-red-400" };
  if (c.threads.some((t) => t.status === "working")) return { text: "Trabajando", cls: "text-blue-600 dark:text-blue-400" };
  if (c.threads.some((t) => t.status === "waiting")) return { text: "Te necesita", cls: "text-orange-600 dark:text-orange-400" };
  if (c.threads.some((t) => t.status === "error")) return { text: "Hilo fallido", cls: "text-red-600 dark:text-red-400" };
  const p = c.pull;
  if (p && c.pr_state !== "merged" && c.pr_state !== "closed") {
    const passed = p.ci_passed ?? 0, failed = p.ci_failed ?? 0, running = p.ci_pending ?? 0;
    const total = passed + failed + running;
    if (failed > 0 || p.ci === "FAILURE" || p.ci === "ERROR") return { text: failed > 0 ? `CI: ${failed} ${failed === 1 ? "falla" : "fallan"}` : "CI fallido", cls: "text-red-600 dark:text-red-400" };
    if (p.mergeable === "CONFLICTING") return { text: "Conflictos", cls: "text-red-600 dark:text-red-400" };
    if (running > 0 || p.ci === "PENDING" || p.ci === "EXPECTED") return { text: total > 0 ? `CI ${passed}/${total}` : "CI en curso", cls: "text-amber-600 dark:text-amber-400", busy: true };
    const approved = p.approved_by?.length ?? 0, waiting = p.review_requests.length;
    if (p.review_decision === "CHANGES_REQUESTED" || (p.changes_by?.length ?? 0) > 0) return { text: "Cambios pedidos", cls: "text-red-600 dark:text-red-400" };
    if (p.draft) return { text: "Draft", cls: "text-slate-500 dark:text-muted" };
    if (waiting > 0) return { text: approved > 0 ? `${approved}/${approved + waiting} aprobadas` : "Review pendiente", cls: "text-amber-600 dark:text-amber-400" };
    if (c.column === "ready_to_merge") return { text: approved > 1 ? `${approved}/${approved} aprobadas` : "Lista para merge", cls: "text-emerald-600 dark:text-emerald-400" };
    return { text: slot(c).label, cls: "text-amber-600 dark:text-amber-400" };
  }
  if (c.pr_state) return { text: slot(c).label, cls: "text-slate-500 dark:text-muted" };
  if (c.has_plan) return { text: "Con plan", cls: "text-indigo-600 dark:text-indigo-400" };
  return { text: c.column === "ready" ? "Lista" : "Sin empezar", cls: "text-slate-500 dark:text-muted" };
}

// Tasks is the kanban, the main view (requirement RF-28).
export function Tasks() {
  const project = useProject();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { task?: string; launch?: QuickLaunch };
  useGithubWatch();
  const board = useQuery({ queryKey: ["board", project], queryFn: () => api.board(project || undefined), // Faster while a pull request has checks running, so CI progress shows up soon.
    refetchInterval: (q) => (q.state.data?.cards.some((c) => c.pull && ((c.pull.ci_pending ?? 0) > 0 || c.pull.ci === "PENDING")) ? 15_000 : 30_000) });
  const [adding, setAdding] = useState(false);
  const [drag, setDrag] = useState<string>();
  const [quick, setQuick] = useState<{ c: Card; x: number; y: number }>();
  const [quickError, setQuickError] = useState<unknown>();
  // expanded is the column showing all its cards with its own scroll.
  const [expanded, setExpanded] = useState<Column>();
  const [panel, setPanel] = useState<Column>();
  const footer = useRef<HTMLDivElement>(null);
  const move = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: Stage }) => api.tasks.update(id, { stage }),
    onSettled: () => qc.invalidateQueries({ queryKey: ["board"] }),
  });
  // Newest activity first, in every column.
  const cards = [...(board.data?.cards ?? [])].sort((x, y) => lastActivity(y).localeCompare(lastActivity(x)));
  const open = (id?: string, launch?: QuickLaunch) => { setPanel(undefined); navigate({ to: "/tasks", search: (id ? { task: id, ...(launch && { launch }) } : {}) as never }); };
  const cardEl = (c: Card) => <TaskCard key={c.id} c={c} onOpen={() => open(c.id)} onDrag={() => { setQuick(undefined); setDrag(c.id); }} onMenu={(x, y) => { setQuickError(undefined); setQuick({ c, x, y }); }} />;
  useEffect(() => {
    if (!panel) return;
    const key = (e: KeyboardEvent) => { if (e.key === "Escape" && !search.task) setPanel(undefined); };
    const down = (e: MouseEvent) => { if (!footer.current?.contains(e.target as Node)) setPanel(undefined); };
    window.addEventListener("keydown", key);
    window.addEventListener("mousedown", down);
    return () => { window.removeEventListener("keydown", key); window.removeEventListener("mousedown", down); };
  }, [panel, search.task]);
  const drop = (col: Column) => { if (drag && MANUAL[col]) move.mutate({ id: drag, stage: MANUAL[col]! }); setDrag(undefined); };
  return (
    <div className="flex flex-col gap-3 sm:h-[calc(100dvh-3rem-2px)]">
      <div className="flex shrink-0 items-center gap-2">
        <h2 className="font-semibold">Kanban</h2>
        <Button className="ml-auto" onClick={() => setAdding(true)}><Plus weight="regular" className="size-4" />Nueva idea</Button>
      </div>
      <ErrorText error={board.error ?? move.error ?? quickError} />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-slate-200 bg-slate-100 dark:border-border-strong/60 dark:bg-canvas">
        <div className="grid min-h-0 flex-1 grid-cols-1 sm:grid-cols-6 sm:overflow-x-auto sm:[&>section]:min-w-44">
          {MAIN.map((col, i) => {
            const list = cards.filter((c) => c.column === col);
            const all = expanded === col;
            return (
              <section key={col} className={`flex min-h-24 min-w-0 flex-col border-slate-200 dark:border-border max-sm:border-b sm:min-h-0 ${i > 0 ? "sm:border-l" : ""} ${drag && MANUAL[col] ? "bg-blue-50/60 dark:bg-accent-subtle/40" : ""}`}
                onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }}
                onDrop={() => drop(col)}>
                <header className="flex h-10 shrink-0 items-center gap-2 border-b border-slate-200 px-3 dark:border-border">
                  <span className={`size-2 rounded-full ${DOT[col]}`} />
                  <h3 className="min-w-0 truncate text-sm font-medium">{COLUMN_LABELS[col]}</h3>
                  {all && <button type="button" onClick={() => setExpanded(undefined)} className="rounded px-1.5 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">Mostrar menos</button>}
                  <span className="ml-auto text-xs tabular-nums text-slate-500 dark:text-muted">{list.length}</span>
                </header>
                <ColumnCards list={list} all={all} onExpand={() => setExpanded(col)} card={cardEl} />
              </section>
            );
          })}
        </div>
        <div ref={footer} className="relative flex shrink-0 items-center gap-2 border-t border-slate-200 px-3 py-2 dark:border-border">
          {ARCHIVED.map((col) => {
            const n = cards.filter((c) => c.column === col).length;
            return (
              <button key={col} type="button" aria-expanded={panel === col} onClick={() => setPanel(panel === col ? undefined : col)}
                onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }} onDrop={() => drop(col)}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium ${panel === col ? "border-slate-400 bg-slate-100 dark:border-border-strong dark:bg-raised" : "border-slate-200 hover:bg-slate-50 dark:border-border dark:hover:bg-raised"} ${drag && MANUAL[col] ? "border-blue-400 bg-blue-50 dark:bg-accent-subtle" : ""}`}>
                <span className={`size-2 rounded-full ${DOT[col]}`} />{COLUMN_LABELS[col]} · <span className="tabular-nums">{n}</span>
              </button>
            );
          })}
          {panel && (
            <div role="region" aria-label={COLUMN_LABELS[panel]} className="notif-in absolute inset-x-0 bottom-full z-20 border-t border-slate-200 bg-slate-100 shadow-[0_-8px_24px_-12px_rgba(0,0,0,0.3)] dark:border-border-strong/60 dark:bg-canvas">
              <div className="flex items-start gap-2 overflow-x-auto p-2" onWheel={(e) => { if (e.deltaX === 0) e.currentTarget.scrollLeft += e.deltaY; }}>
                {cards.filter((c) => c.column === panel).map((c) => (
                  <div key={c.id} className="w-60 shrink-0 [&>div]:cursor-pointer">{cardEl(c)}</div>
                ))}
                {!cards.some((c) => c.column === panel) && <p className="px-2 py-3 text-sm text-slate-500 dark:text-muted">Sin tareas.</p>}
              </div>
            </div>
          )}
        </div>
      </div>
      {adding && <IdeaModal onClose={() => setAdding(false)} projectId={project || undefined} />}
      {quick && <TaskQuickMenu key={quick.c.id} {...quick} onClose={() => setQuick(undefined)} onOpen={(launch) => open(quick.c.id, launch)} onError={setQuickError} />}
      {search.task && <TaskDetail key={search.task} id={search.task} launch={search.launch} onClose={() => open()} />}
    </div>
  );
}

// ColumnCards shows the cards that fit in the visible height. It measures the real height of each card in a hidden copy,
// so cards with less content leave room for more, and the rest go behind a "+N más" button.
function ColumnCards({ list, all, onExpand, card }: { list: Card[]; all: boolean; onExpand: () => void; card: (c: Card) => React.ReactNode }) {
  const zone = useRef<HTMLDivElement>(null);
  const probe = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState(Infinity);
  // A column cannot show more than a screenful, so measuring the first cards is enough.
  const probed = list.slice(0, 40);
  const key = probed.map((c) => `${c.id}:${c.title}:${c.column}:${c.threads.length}:${c.pr_number}:${c.branch}`).join("|");
  useLayoutEffect(() => {
    const z = zone.current, p = probe.current;
    if (!z || !p) return setFit(Infinity);
    const desktop = window.matchMedia("(min-width: 640px)");
    const measure = () => {
      if (!desktop.matches || all) return setFit(Infinity);
      const avail = z.clientHeight - PAD * 2;
      const hs = Array.from(p.children).map((e) => (e as HTMLElement).offsetHeight);
      let used = 0, n = 0;
      for (const h of hs) {
        const next = used + (n ? GAP : 0) + h;
        if (next > avail) break;
        used = next;
        n++;
      }
      if (n >= list.length) return setFit(Infinity);
      // Keep room for the "+N más" button.
      while (n > 1 && used + GAP + MORE_H > avail) { n--; used -= hs[n] + (n ? GAP : 0); }
      setFit(Math.max(1, n));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(z);
    ro.observe(p);
    desktop.addEventListener("change", measure);
    return () => { ro.disconnect(); desktop.removeEventListener("change", measure); };
  }, [key, all, list.length]); // eslint-disable-line react-hooks/exhaustive-deps
  const shown = fit === Infinity || all ? list : list.slice(0, fit);
  return (
    <div ref={zone} className={`relative flex min-h-0 flex-1 flex-col gap-2 p-2 ${all ? "overflow-y-auto" : "sm:overflow-hidden"}`}>
      {shown.map(card)}
      {shown.length < list.length && <button type="button" onClick={onExpand} className="shrink-0 rounded-lg border border-dashed border-slate-300 py-1.5 text-xs font-medium text-slate-500 hover:bg-slate-50 dark:border-border-strong dark:text-muted dark:hover:bg-raised">+{list.length - shown.length} más</button>}
      {!all && <div ref={probe} aria-hidden="true" className="pointer-events-none invisible absolute inset-x-0 top-0 flex flex-col gap-2 p-2 max-sm:hidden">{probed.map(card)}</div>}
    </div>
  );
}

// MergeButton squash-merges the card's pull request with the existing GitHub action, asking for a second click first.
function MergeButton({ c, pull }: { c: Card; pull: Pull }) {
  const qc = useQueryClient();
  const merge = useMutation({
    mutationFn: () => api.githubAct(pull.repo, "pulls", pull.number, { type: "merge", method: "SQUASH" }),
    onSettled: () => qc.invalidateQueries({ queryKey: ["board"] }),
  });
  const [armed, click] = useArm(() => merge.mutate());
  return (
    <button type="button" draggable={false} disabled={merge.isPending} title={merge.error ? merge.error.message : "Fusionar con squash"}
      onClick={(e) => { e.stopPropagation(); click(); }}
      className={`inline-flex shrink-0 items-center gap-1 rounded-md px-2.5 py-1.5 text-xs font-medium text-white disabled:opacity-50 ${merge.error ? "bg-red-600 hover:bg-red-700" : "bg-emerald-600 hover:bg-emerald-700"}`}>
      <GitMerge weight="bold" aria-hidden="true" className="size-3.5" />{merge.isPending ? "Fusionando…" : armed ? "¿Confirmar?" : "Merge"}
    </button>
  );
}

// linkedAt is when the thread was assigned to its task.
const linkedAt = (t: Thread) => t.task_linked_at ?? t.created_at;

// Avatar is a GitHub user's picture with its review state: green approved, amber pending, red changes requested.
function Avatar({ login, state }: { login: string; state: "approved" | "pending" | "changes" }) {
  const [failed, setFailed] = useState(false);
  const dot = state === "approved" ? "bg-emerald-500" : state === "changes" ? "bg-red-500" : "bg-amber-400";
  const label = state === "approved" ? "aprobó" : state === "changes" ? "pidió cambios" : "review pendiente";
  return (
    <span className="relative inline-block size-5 shrink-0" title={`${login} · ${label}`}>
      {failed
        ? <span className="grid size-5 place-items-center rounded-full bg-slate-200 text-[10px] font-semibold uppercase text-slate-600 dark:bg-hover dark:text-text-secondary">{login[0]}</span>
        : <img src={`https://github.com/${login}.png?size=40`} alt={login} loading="lazy" draggable={false} onError={() => setFailed(true)} className="size-5 rounded-full bg-slate-200 dark:bg-hover" />}
      <span className={`absolute -bottom-0.5 -right-0.5 size-2 rounded-full ring-2 ring-white dark:ring-raised ${dot}`} />
    </span>
  );
}

// RefLink goes to the issue or pull request inside Dory's GitHub view.
function RefLink({ repo, kind, number }: { repo?: string; kind: "issues" | "pulls"; number: number }) {
  const Icon = kind === "pulls" ? GitPullRequest : CircleDashed;
  const label = kind === "pulls" ? `Pull request #${number}` : `Issue #${number}`;
  const cls = "inline-flex shrink-0 items-center gap-1 rounded-md px-1 py-0.5 text-slate-600 no-underline dark:text-text-secondary";
  const m = repo?.match(/^([\w.-]+)\/([\w.-]+)$/);
  const body = <><Icon aria-hidden="true" className="size-3.5 shrink-0" />#{number}</>;
  if (!m) return <span className={cls} title={label}>{body}</span>;
  return <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: m[1], repo: m[2], kind, number: String(number) }} draggable={false} title={label} aria-label={label} className={`${cls} hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-white/5 dark:hover:text-white`}>{body}</Link>;
}

// Reviewers shows who reviewed the pull request and who still has to.
function Reviewers({ pull }: { pull: Pull }) {
  const people = [
    ...(pull.approved_by ?? []).map((l) => ({ login: l, state: "approved" as const })),
    ...(pull.changes_by ?? []).map((l) => ({ login: l, state: "changes" as const })),
    ...pull.review_requests.map((l) => ({ login: l, state: "pending" as const })),
  ];
  if (!people.length) return null;
  const shown = people.slice(0, 4);
  return (
    <span className="ml-auto flex min-w-0 items-center -space-x-1">
      {shown.map((p) => <Avatar key={p.login} {...p} />)}
      {people.length > shown.length && <span className="pl-2 text-xs text-slate-500 dark:text-muted">+{people.length - shown.length}</span>}
    </span>
  );
}

// ThreadRow links to the latest thread and, with more than one, opens a menu with all of them.
function ThreadRow({ threads }: { threads: Thread[] }) {
  const id = useId();
  const menu = useRef<HTMLDivElement>(null);
  const sorted = [...threads].sort((a, b) => linkedAt(b).localeCompare(linkedAt(a)));
  const t = sorted[0];
  const place = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    const r = e.currentTarget.getBoundingClientRect();
    const el = menu.current;
    if (!el) return;
    el.style.top = `${r.bottom + 6}px`;
    el.style.left = `${Math.max(12, Math.min(r.left, window.innerWidth - 300))}px`;
  };
  const link = "inline-flex min-w-0 items-center gap-1.5 text-slate-600 no-underline dark:text-text-secondary";
  return (
    <div className="flex items-center gap-1.5 text-xs" onClick={(e) => e.stopPropagation()}>
      <Link to="/threads/$threadId" params={{ threadId: t.id }} title={t.title} draggable={false} className={`${link} rounded-md border border-slate-200 px-1.5 py-0.5 hover:bg-slate-100 dark:border-white/10 dark:hover:bg-white/5`}>
        <ChatCircle aria-hidden="true" className="size-3 shrink-0" />
        <span className="min-w-0 truncate">{t.model || t.provider}</span>
        <ThreadDot status={t.status} />
      </Link>
      {sorted.length > 1 && (<>
        <button type="button" popoverTarget={id} onClick={place} aria-label={`Ver los ${sorted.length} hilos`} className="rounded-md px-1.5 py-0.5 font-medium text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-white/5">+{sorted.length - 1}</button>
        <div ref={menu} id={id} popover="auto" role="menu" className="fixed m-0 w-72 rounded-xl border border-slate-200 bg-white p-1 text-slate-900 shadow-xl dark:border-border-strong dark:bg-surface dark:text-white">
          {sorted.map((x) => (
            <Link key={x.id} to="/threads/$threadId" params={{ threadId: x.id }} role="menuitem" className="flex items-center gap-2 rounded-md px-2.5 py-2 text-sm text-inherit no-underline hover:bg-slate-100 dark:hover:bg-raised">
              <ThreadDot status={x.status} />
              <span className="min-w-0 flex-1"><span className="block truncate">{x.model || x.provider}</span><span className="block truncate text-xs text-slate-500 dark:text-muted">{x.title || "Hilo"}</span></span>
              <span className="shrink-0 text-xs text-slate-500 dark:text-muted">{ago(linkedAt(x))}</span>
            </Link>
          ))}
        </div>
      </>)}
    </div>
  );
}

// TaskCard has the height its content needs. Every row is separated by the same gap, and the footer divider uses the card's border color.
function TaskCard({ c, onOpen, onDrag, onMenu }: { c: Card; onOpen: () => void; onDrag: () => void; onMenu: (x: number, y: number) => void }) {
  const s = slot(c);
  const st = status(c);
  const where = c.branch || c.repo;
  const working = c.blockers.length === 0 ? c.threads.filter((t) => t.status === "working").sort((a, b) => (b.last_activity_at ?? b.updated_at).localeCompare(a.last_activity_at ?? a.updated_at))[0] : undefined;
  const activity = useThreadActivity(working?.id);
  const merge = c.column === "ready_to_merge" && !!c.pull;
  const repo = c.pull?.repo ?? c.repo;
  return (
    <div draggable onDragStart={onDrag} onClick={onOpen} onContextMenu={(e) => { e.preventDefault(); onMenu(e.clientX, e.clientY); }}
      className={`flex shrink-0 cursor-grab flex-col overflow-hidden rounded-lg border border-slate-300 text-sm shadow-sm dark:border-white/10 dark:shadow-black/40 ${c.column === "needs_you" ? "bg-red-50 dark:bg-red-950/30" : "bg-white dark:bg-raised"}`}>
      <div className="flex flex-col gap-2 p-2.5">
        <div className="flex items-start gap-2">
          <span className="mt-0.5 grid size-4 shrink-0 place-items-center" title={s.label} aria-label={s.label}>{s.icon}</span>
          <span className="line-clamp-2 min-w-0 font-semibold leading-tight">{c.title}</span>
        </div>
        {where && <p className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted"><GitBranch aria-hidden="true" className="size-3.5 shrink-0" /><span className="truncate">{where}</span></p>}
        {(c.issue_number || c.pr_number) && (
          <div className="flex items-center gap-1.5 text-xs" onClick={(e) => e.stopPropagation()}>
            {c.issue_number && <RefLink repo={repo} kind="issues" number={c.issue_number} />}
            {c.pr_number && <RefLink repo={repo} kind="pulls" number={c.pr_number} />}
            {c.pr_number && c.pull && <Reviewers pull={c.pull} />}
          </div>
        )}
        {c.threads.length > 0 && <ThreadRow threads={c.threads} />}
      </div>
      <div className={`flex shrink-0 items-center gap-2 border-t border-slate-300 px-2.5 text-xs dark:border-white/10 ${merge ? "py-2.5" : "py-2"}`}>
        <span key={working ? activity ?? st.text : st.text} className={`notif-in inline-flex min-w-0 flex-1 items-center gap-1.5 font-medium ${st.cls}`} title={working ? activity ?? st.text : st.text}>{st.busy && !working && <CircleNotch aria-hidden="true" className="size-3 shrink-0 animate-spin" />}<span className="truncate">{working ? activity ?? st.text : st.text}</span></span>
        <span className="shrink-0 text-slate-500 dark:text-muted">{ago(lastActivity(c))}</span>
        {merge && c.pull && <MergeButton c={c} pull={c.pull} />}
      </div>
    </div>
  );
}

// ThreadDot is the v1 colored dot of a thread's state.
function ThreadDot({ status }: { status: string }) {
  const d = status === "error" ? { label: "Error", color: "bg-red-500", pulse: false }
    : status === "waiting" ? { label: "Te necesita", color: "bg-amber-500", pulse: true }
    : status === "working" ? { label: "Trabajando", color: "bg-blue-500", pulse: true }
    : { label: "Inactivo", color: "bg-slate-400", pulse: false };
  return <span role="img" title={d.label} aria-label={d.label} className={`size-2 shrink-0 rounded-full ${d.color} ${d.pulse ? "motion-safe:animate-pulse" : ""}`} />;
}

const COLUMN_TONE: Record<Column, string> = {
  inbox: "bg-slate-100 text-slate-700 dark:bg-raised dark:text-text-secondary",
  ready: "bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-300",
  working: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
  needs_you: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
  in_review: "bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-300",
  ready_to_merge: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-300",
  done: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300",
  cancelled: "bg-slate-100 text-slate-500 dark:bg-raised dark:text-muted",
};

// TaskDetail is the task card. The board shows it as a wide modal; a thread shows
// it as a narrow side card (layout "side") with the same content and actions.
export function TaskDetail({ id, launch: initialLaunch, onClose, layout = "modal" }: { id: string; launch?: QuickLaunch; onClose: () => void; layout?: "modal" | "side" }) {
  const side = layout === "side";
  const qc = useQueryClient();
  const dictating = useDictationInProgress();
  const navigate = useNavigate();
  const card = useQuery({ queryKey: ["card", id], queryFn: () => api.card(id) });
  const plans = useQuery({ queryKey: ["plans", id], queryFn: () => api.plans(id) });
  const planDoc = useQuery({ queryKey: ["plans", id, "doc"], queryFn: () => api.plan(id) });
  const deps = useQuery({ queryKey: ["dependencies", id], queryFn: () => api.dependencies(id) });
  const board = useQuery({ queryKey: ["board", ""], queryFn: () => api.board() });
  const repos = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list() });
  const [draft, setDraft] = useState<{ title: string; description: string; issue: string; pr: string; repo: string }>();
  const planSection = useRef<HTMLDetailsElement>(null);
  const launchPopoverId = useId();
  const launchTrigger = useRef<HTMLButtonElement>(null);
  const launchPopover = useRef<HTMLDivElement>(null);
  const launchAnchor = useRef<HTMLElement | null>(null);
  const menuId = useId();
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const placeLaunchPopover = () => {
    const trigger = launchAnchor.current ?? launchTrigger.current;
    const popover = launchPopover.current;
    if (!trigger || !popover) return;
    const rect = trigger.getBoundingClientRect();
    // The anchor is not laid out yet (the detail is still opening): retry next frame.
    if (rect.width === 0 && rect.height === 0) { requestAnimationFrame(() => { if (popover.matches(":popover-open")) placeLaunchPopover(); }); return; }
    const { right, top, bottom } = rect;
    const width = Math.min(368, window.innerWidth - 24);
    const below = Math.max(0, window.innerHeight - bottom - 20);
    const above = Math.max(0, top - 20);
    const placeAbove = below < 260 && above > below;
    popover.style.width = `${width}px`;
    popover.style.left = `${Math.max(12, Math.min(right - width, window.innerWidth - width - 12))}px`;
    popover.style.top = placeAbove ? "auto" : `${bottom + 8}px`;
    popover.style.bottom = placeAbove ? `${window.innerHeight - top + 8}px` : "auto";
    popover.style.maxHeight = `${placeAbove ? above : below}px`;
  };
  useEffect(() => {
    const reposition = () => {
      if (launchPopover.current?.matches(":popover-open")) placeLaunchPopover();
    };
    window.addEventListener("resize", reposition);
    window.addEventListener("scroll", reposition, true);
    return () => {
      window.removeEventListener("resize", reposition);
      window.removeEventListener("scroll", reposition, true);
    };
  }, []);
  const openPlan = () => {
    const details = planSection.current;
    if (!details) return;
    details.open = true;
    details.querySelector("summary")?.focus({ preventScroll: true });
    details.scrollIntoView({ block: "nearest" });
  };
  const [editing, setEditing] = useState(false);
  // mode is what the launch popover opens: a thread to define the plan or one to implement.
  const [mode, setMode] = useState<"define" | "implement">("define");
  const c = card.data;
  useEffect(() => { if (c && !draft) setDraft({ title: c.title, description: c.description, issue: c.issue_number?.toString() ?? "", pr: c.pr_number?.toString() ?? "", repo: c.repository_id ?? "" }); }, [c]); // eslint-disable-line react-hooks/exhaustive-deps
  const refresh = () => { qc.invalidateQueries({ queryKey: ["card", id] }); qc.invalidateQueries({ queryKey: ["board"] }); };
  const save = useMutation({
    mutationFn: () => api.tasks.update(id, { title: draft!.title, description: draft!.description, issue_number: draft!.issue ? Number(draft!.issue) : null, pr_number: draft!.pr ? Number(draft!.pr) : null, repository_id: draft!.repo || null }),
    onSuccess: () => { refresh(); setEditing(false); },
  });
  const stage = useMutation({ mutationFn: (s: Stage) => api.tasks.update(id, { stage: s }), onSuccess: refresh });
  const dep = useMutation({
    mutationFn: ({ other, add }: { other: string; add: boolean }) => (add ? api.addDependency(id, other) : api.removeDependency(id, other)).then(() => undefined),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["dependencies", id] }); refresh(); },
  });
  const archive = useMutation({ mutationFn: () => api.tasks.archive(id), onSuccess: () => { refresh(); onClose(); } });
  const form = useLaunchForm({ section: "task", project: c?.project_id, borderless: true });
  // Sending opens the thread in the background; with go it also moves to it.
  const sent = (r: { thread_id: string }, go: boolean) => {
    if (go) return navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } });
    launchPopover.current?.hidePopover();
    refresh();
    qc.invalidateQueries({ queryKey: ["threads"] });
  };
  const launch = useMutation({
    mutationFn: (_go: boolean) => { form.remember(); return api.launch(id, form.value); },
    onSuccess: (r, go) => sent(r, go),
  });
  const define = useMutation({
    mutationFn: (_go: boolean) => {
      form.remember();
      const text = planMessage({ id, title: c!.title, description: c!.description });
      return api.createThread({ ...form.value, project_id: c!.project_id, task_id: id, title: `Definir: ${c!.title}`, text });
    },
    onSuccess: (r, go) => sent(r, go),
  });
  // A quick action from the board opens the detail with the launch popover ready.
  const autoLaunched = useRef(false);
  useEffect(() => {
    if (!initialLaunch || autoLaunched.current || !c || !draft || !planDoc.data || !launchPopover.current) return;
    autoLaunched.current = true;
    setMode(initialLaunch);
    const popover = launchPopover.current;
    // Anchor to the matching action button of the detail once it is laid out,
    // and again when the dialog's opening animation ends.
    requestAnimationFrame(() => {
      launchAnchor.current = document.querySelector<HTMLElement>(`[data-launch="${initialLaunch}"]`);
      popover.showPopover();
      placeLaunchPopover();
      const dialog = popover.closest("dialog") ?? popover.parentElement;
      const again = () => { if (popover.matches(":popover-open")) placeLaunchPopover(); };
      dialog?.addEventListener("animationend", again, { once: true });
      dialog?.addEventListener("transitionend", again, { once: true });
      setTimeout(again, 300);
    });
  }); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!side) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !document.querySelector(":popover-open")) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [side, onClose]);
  // frame is the modal on the board and a plain card in a thread.
  const frame = (title: string, children: React.ReactNode) => side ? (
    <div role="region" aria-label={title} className="space-y-4">
      <div className="flex items-start justify-between gap-2">
        <h2 className="min-w-0 wrap-break-word text-lg font-semibold leading-tight">{title}</h2>
        <button type="button" aria-label="Cerrar" onClick={onClose} className="-mr-1 grid size-8 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><X weight="regular" aria-hidden="true" className="size-4" /></button>
      </div>
      {children}
    </div>
  ) : <Modal title={title} onClose={onClose} className="!max-w-5xl sm:!p-6 [&>div:first-child>h2]:text-2xl [&>div:first-child>h2]:leading-tight">{children}</Modal>;
  if (!c || !draft) return frame("Tarea", <ErrorText error={card.error} />);
  const dirty = draft.title !== c.title || draft.description !== c.description || draft.issue !== (c.issue_number?.toString() ?? "") || draft.pr !== (c.pr_number?.toString() ?? "") || draft.repo !== (c.repository_id ?? "");
  const others = (board.data?.cards ?? []).filter((o) => o.id !== id && (!c.project_id || o.project_id === c.project_id));
  const latest = [...(plans.data ?? [])].sort((a, b) => b.version - a.version)[0];
  // The latest plan has to be accepted before the task is implemented.
  const planPending = !!latest && !latest.accepted_at;
  const planAccepted = !!planDoc.data?.accepted_at;
  const hasPlanContent = !!planDoc.data?.body_md.trim();
  const reviewPlan = hasPlanContent && !planAccepted;
  const launchLabel = mode === "implement" ? "Implementar" : "Definir plan";
  const launchAction = mode === "implement" ? launch : define;
  const launchPending = launch.isPending || define.isPending;
  const launchBlocked = mode === "implement" && (c.blockers.length > 0 || planPending);
  const environment = form.nodes.find((node) => node.value === form.value.node)?.label ?? "Este nodo";
  const projectRepos = (repos.data ?? []).filter((r) => !c.project_id || r.project_id === c.project_id);
  const githubRepo = c.repo && /^[\w.-]+\/[\w.-]+$/.test(c.repo) ? `https://github.com/${c.repo}` : undefined;
  const pullNumber = c.pull?.number ?? c.pr_number;
  const pullRoute = githubRoute(c.pull?.url) ?? githubRoute(githubRepo && pullNumber ? `${githubRepo}/pull/${pullNumber}` : undefined);
  const openLaunch = (anchor: HTMLElement | null, next: "define" | "implement") => {
    setMode(next);
    launch.reset();
    define.reset();
    launchAnchor.current = anchor;
    menu.current?.hidePopover();
    launchPopover.current?.showPopover();
    placeLaunchPopover();
  };
  const openMenu = () => {
    const trigger = menuTrigger.current, popover = menu.current;
    if (!trigger || !popover) return;
    const { right, bottom } = trigger.getBoundingClientRect();
    popover.style.top = `${bottom + 6}px`;
    popover.style.left = `${Math.max(12, right - 224)}px`;
  };
  const working = c.threads.find((t) => t.status === "working");
  const waiting = c.threads.find((t) => t.status === "waiting");
  const failed = c.threads.find((t) => t.status === "error");
  const merged = c.pr_state === "merged";
  const blockerTitles = c.blockers.map((b) => others.find((o) => o.id === b)?.title ?? b);
  // next is the single step the task needs now: what happens, why and the one action that moves it on.
  type Next = { tone: string; title: string; detail?: React.ReactNode; action?: React.ReactNode };
  const tones = {
    info: "border-blue-200 bg-blue-50 dark:border-blue-900 dark:bg-blue-950/40",
    warn: "border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950/40",
    ok: "border-emerald-200 bg-emerald-50 dark:border-emerald-900 dark:bg-emerald-950/40",
    bad: "border-red-200 bg-red-50 dark:border-red-900 dark:bg-red-950/40",
    idle: "border-slate-200 bg-slate-50 dark:border-border dark:bg-raised/40",
  };
  const threadLink = (t: { id: string }, label: string) => (
    <Link to="/threads/$threadId" params={{ threadId: t.id }} className="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-700 dark:bg-accent dark:hover:bg-accent-hover"><Chats className="size-4" />{label}</Link>
  );
  const launchButton = (label: string, next: "define" | "implement", disabled = false, variant: "default" | "ghost" = "default") => (
    <Button data-launch={next} variant={variant} onClick={(e) => openLaunch(e.currentTarget, next)} disabled={disabled || !planDoc.data || launchPending}>{next === "define" ? <ClipboardText className="size-4" /> : <Rocket className="size-4" />}{label}</Button>
  );
  const prLink = pullRoute && <Link to="/github/$owner/$repo/$kind/$number" params={pullRoute} className="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-700 dark:bg-accent dark:hover:bg-accent-hover"><GitPullRequest className="size-4" />Abrir PR</Link>;
  // The pull request decides the column; stage only counts without one.
  const doneByHand = c.column === "done" && c.stage === "done" && !pullNumber;
  const next: Next =
    c.column === "cancelled" ? { tone: tones.idle, title: "Tarea cancelada", detail: "No se trabajará en ella salvo que la reabras.", action: <Button variant="ghost" onClick={() => stage.mutate("ready")}>Reabrir</Button> }
    : doneByHand ? { tone: tones.ok, title: "Completada", detail: "Marcada como hecha sin pull request.", action: <Button variant="ghost" onClick={() => stage.mutate("ready")}>Reabrir</Button> }
    : merged ? { tone: tones.ok, title: "Completada", detail: `La PR #${pullNumber} está fusionada.`, action: prLink }
    : waiting ? { tone: tones.warn, title: "Un agente necesita tu respuesta", detail: waiting.title, action: threadLink(waiting, "Responder") }
    : working ? { tone: tones.info, title: "Un agente está trabajando", detail: working.title, action: threadLink(working, "Ver hilo") }
    : failed && !c.pull ? { tone: tones.bad, title: "El hilo falló", detail: "Revisa el error o lanza otro hilo.", action: threadLink(failed, "Ver hilo") }
    : c.pull ? { tone: c.pull.ci === "FAILURE" || c.pull.ci === "ERROR" || c.pull.mergeable === "CONFLICTING" ? tones.bad : c.pull.draft ? tones.info : tones.ok, title: `PR #${pullNumber}: ${slot(c).label}`, detail: c.pull.draft ? "La PR sigue en borrador hasta que la marques como lista." : undefined, action: prLink }
    : reviewPlan ? { tone: tones.warn, title: `Revisa y acepta el plan${planDoc.data?.revision ? ` V${planDoc.data.revision}` : ""}`, detail: "Hasta aceptarlo no se puede implementar.", action: <Button onClick={openPlan}><ClipboardText className="size-4" />Revisar plan</Button> }
    : !planAccepted ? { tone: tones.idle, title: "Define el plan o implementa directamente", detail: c.blockers.length ? `El plan es opcional. Para implementar, termina antes: ${blockerTitles.join(", ")}.` : c.description.trim() ? "Se recomienda un plan para tareas complejas; si es sencilla, mándala a implementar." : "Añade una descripción para que el agente sepa qué hacer.", action: <div className="flex flex-wrap gap-2">{launchButton("Implementar directamente", "implement", c.blockers.length > 0, "ghost")}{launchButton("Definir plan", "define")}</div> }
    : c.blockers.length ? { tone: tones.warn, title: `Bloqueada por ${c.blockers.length === 1 ? "una dependencia" : `${c.blockers.length} dependencias`}`, detail: `Termina antes: ${blockerTitles.join(", ")}.`, action: launchButton("Implementar", "implement", true) }
    : { tone: tones.ok, title: "Lista para implementar", detail: "El plan está aceptado y no hay bloqueos.", action: launchButton("Implementar", "implement") };
  // steps shows where the task is in its path: description, plan, implementation and pull request.
  const steps: { label: string; state: "done" | "now" | "todo" }[] = [
    { label: "Descripción", state: c.description.trim() ? "done" : "now" },
    { label: hasPlanContent || planAccepted ? "Plan" : "Plan (opcional)", state: planAccepted || c.pull || working ? "done" : hasPlanContent ? "now" : c.description.trim() ? "now" : "todo" },
    { label: "Implementación", state: c.pull ? "done" : planAccepted || working ? "now" : "todo" },
    { label: "PR", state: merged ? "done" : c.pull ? "now" : "todo" },
  ];
  const chip = "inline-flex max-w-full items-center gap-1.5 rounded-full border border-slate-200 px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100 dark:border-border dark:text-text-secondary dark:hover:bg-raised";
  const heading = "mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-muted";
  const row = "flex min-w-0 items-center gap-2 rounded-lg px-2 py-2 text-sm hover:bg-slate-100 dark:hover:bg-raised";
  const menuItem = "flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-slate-100 disabled:opacity-50 dark:hover:bg-raised";
  return (
    frame(c.title, <>
      <div className="flex flex-wrap items-center gap-2">
        <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${COLUMN_TONE[c.column]}`}>{COLUMN_LABELS[c.column]}</span>
        {c.repo && (githubRepo ? <a className={chip} href={githubRepo} target="_blank" rel="noreferrer"><GithubLogo className="size-3.5 shrink-0" /><span className="truncate">{c.repo}</span></a> : <span className={chip}>{c.repo}</span>)}
        {c.issue_number && (githubRepo ? <a className={chip} href={`${githubRepo}/issues/${c.issue_number}`} target="_blank" rel="noreferrer">Issue #{c.issue_number}</a> : <span className={chip}>Issue #{c.issue_number}</span>)}
        {pullNumber && (pullRoute ? <Link className={chip} to="/github/$owner/$repo/$kind/$number" params={pullRoute}><GitPullRequest className="size-3.5" />PR #{pullNumber}{(c.pull?.draft || c.pr_state === "draft") && " · borrador"}</Link> : <span className={chip}>PR #{pullNumber}</span>)}
        {c.branch && <span className={`${chip} font-mono`} title={c.branch}><GitBranch className="size-3.5 shrink-0" /><span className="truncate">{c.branch}</span></span>}
        <button ref={menuTrigger} type="button" popoverTarget={menuId} onClick={openMenu} aria-label="Más acciones" className={`${iconButton} ml-auto`}><DotsThree weight="bold" className="size-5" /></button>
        <div ref={menu} id={menuId} popover="auto" role="menu" className="fixed m-0 w-56 rounded-xl border border-slate-200 bg-white p-1 text-slate-900 shadow-xl dark:border-border dark:bg-surface dark:text-white">
          <button type="button" role="menuitem" className={menuItem} onClick={() => { setEditing(true); menu.current?.hidePopover(); }}><PencilSimple className="size-4" />Editar tarea</button>
          <button type="button" role="menuitem" className={menuItem} disabled={!planDoc.data || launchPending} onClick={() => openLaunch(menuTrigger.current, planAccepted || !hasPlanContent ? "implement" : "define")}><Rocket className="size-4" />Abrir otro hilo</button>
          {c.stage !== "ready" && c.column === "inbox" && <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("ready")}>Marcar como lista</button>}
          {c.stage === "ready" && c.column === "ready" && <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("inbox")}>Volver a inbox</button>}
          {!pullNumber && c.column !== "done" && c.column !== "cancelled" && <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("done")}>Marcar como hecha</button>}
          {c.stage === "cancelled" || doneByHand ? <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("ready")}>Reabrir</button> : <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("cancelled")}>Cancelar tarea</button>}
          <div className="my-1 border-t border-slate-200 dark:border-border" />
          <button type="button" role="menuitem" className={`${menuItem} text-red-700 dark:text-red-400`} onClick={() => { menu.current?.hidePopover(); askConfirm("¿Archivar la tarea?", "Archivar").then((ok) => ok && archive.mutate()); }}>Archivar</button>
        </div>
      </div>

      <ol aria-label="Progreso de la tarea" className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
        {steps.map((s, i) => (
          <li key={s.label} className="flex items-center gap-2">
            {i > 0 && <span aria-hidden="true" className="h-px w-4 bg-slate-300 dark:bg-border-strong sm:w-8" />}
            <span aria-current={s.state === "now" ? "step" : undefined} className={`inline-flex items-center gap-1.5 font-medium ${s.state === "done" ? "text-emerald-700 dark:text-emerald-400" : s.state === "now" ? "text-slate-900 dark:text-white" : "text-slate-400 dark:text-muted"}`}>
              <span className={`grid size-5 place-items-center rounded-full border text-[10px] ${s.state === "done" ? "border-emerald-600 bg-emerald-600 text-white" : s.state === "now" ? "border-slate-900 dark:border-white" : "border-slate-300 dark:border-border-strong"}`}>
                {s.state === "done" ? <Check weight="bold" className="size-3" /> : i + 1}
              </span>
              {s.label}
            </span>
          </li>
        ))}
      </ol>

      <section aria-label="Siguiente paso" className={`flex flex-wrap items-center gap-3 rounded-xl border p-4 ${next.tone}`}>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold">{next.title}</p>
          {next.detail && <p className="mt-0.5 text-sm text-slate-600 dark:text-muted">{next.detail}</p>}
        </div>
        {next.action}
        <ErrorText error={stage.error ?? archive.error} />
      </section>

      <div ref={launchPopover} id={launchPopoverId} popover="auto" role="dialog" aria-label="Abrir hilo"
        onKeyDown={(event) => {
          if (event.key !== "Escape") return;
          event.preventDefault();
          event.stopPropagation();
          launchPopover.current?.hidePopover();
          launchAnchor.current?.focus();
        }}
        className="fixed m-0 overflow-auto rounded-xl border border-slate-200 bg-white p-4 text-slate-900 shadow-xl dark:border-border dark:bg-surface dark:text-white">
        <div className="mb-3 flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">{launchLabel}</h3>
          <button type="button" className="grid size-8 shrink-0 place-items-center bg-transparent text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white" popoverTarget={launchPopoverId} popoverTargetAction="hide" aria-label="Cerrar configuración">
            <X weight="regular" aria-hidden="true" className="size-4" />
          </button>
        </div>
        {form.nodes.length <= 1 && <p className="mb-3 text-xs text-slate-600 dark:text-muted">Entorno <span className="ml-1 font-medium">{environment}</span></p>}
        {form.fields}
        {mode === "implement" && !hasPlanContent && !planAccepted && <p className="mb-3 text-xs text-slate-600 dark:text-muted">Se implementará sin plan, a partir de la descripción.</p>}
        {launchBlocked && <p className="mt-3 text-sm text-slate-600 dark:text-muted">{c.blockers.length ? "Resuelve las dependencias para implementar." : "Acepta el plan para implementar."}</p>}
        <ErrorText error={launchAction.error ?? form.error} />
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => launchAction.mutate(true)} disabled={!form.ready || launchPending || launchBlocked}>Enviar e ir</Button>
          <Button onClick={() => launchAction.mutate(false)} disabled={!form.ready || launchPending || launchBlocked}>
            {launchPending ? "Enviando…" : "Enviar"}
          </Button>
        </div>
      </div>

      <div className={`grid items-start gap-6 ${side ? "" : "md:grid-cols-[minmax(0,1fr)_minmax(0,16rem)]"}`}>
        <div className="min-w-0 space-y-6">
          <section>
            <div className="flex items-center">
              <h3 className={heading}>{editing ? "Editar tarea" : "Descripción"}</h3>
              {!editing && <button type="button" onClick={() => setEditing(true)} className="mb-2 ml-auto inline-flex items-center gap-1 text-xs font-medium text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><PencilSimple className="size-3.5" />Editar</button>}
            </div>
            {editing ? (
              <div className="space-y-3 rounded-xl border border-slate-200 p-4 dark:border-border">
                <Label text="Título"><Input autoFocus value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} className="!text-base font-semibold" aria-label="Título" /></Label>
                <DictationField label="Descripción" rows={6} value={draft.description} onValue={(description) => setDraft((current) => current && { ...current, description })} aria-label="Descripción" placeholder="Qué cambiar y cómo comprobarlo" />
                <div className="grid gap-2 sm:grid-cols-3">
                  <Label text="Repositorio"><Select value={draft.repo} onChange={(v) => setDraft({ ...draft, repo: v })} options={[{ value: "", label: projectRepos.length === 1 ? "El del proyecto" : "—" }, ...projectRepos.map((r) => ({ value: r.id, label: repoName(r.remote_url) }))]} /></Label>
                  <Label text="Issue"><Input inputMode="numeric" value={draft.issue} onChange={(e) => setDraft({ ...draft, issue: e.target.value.replace(/\D/g, "") })} /></Label>
                  <Label text="Pull request"><Input inputMode="numeric" value={draft.pr} onChange={(e) => setDraft({ ...draft, pr: e.target.value.replace(/\D/g, "") })} placeholder="Se detecta por la rama" /></Label>
                </div>
                <div className="flex gap-2">
                  <Button onClick={() => save.mutate()} disabled={!dirty || dictating || save.isPending}>Guardar cambios</Button>
                  <Button variant="ghost" onClick={() => { setDraft({ title: c.title, description: c.description, issue: c.issue_number?.toString() ?? "", pr: c.pr_number?.toString() ?? "", repo: c.repository_id ?? "" }); setEditing(false); }}>Descartar</Button>
                </div>
                <ErrorText error={save.error} />
              </div>
            ) : c.description.trim() ? <Md>{c.description}</Md> : (
              <button type="button" onClick={() => setEditing(true)} className="w-full rounded-xl border border-dashed border-slate-300 p-4 text-left text-sm text-slate-500 hover:bg-slate-50 dark:border-border-strong dark:text-muted dark:hover:bg-raised">Sin descripción. Añade qué cambiar y cómo comprobarlo.</button>
            )}
          </section>

          <details ref={planSection} open={reviewPlan} hidden={!!planDoc.data && !hasPlanContent && !planAccepted} className="group overflow-hidden rounded-xl border border-slate-200 dark:border-border">
            <summary className="flex cursor-pointer list-none flex-wrap items-center gap-2 px-4 py-3 text-sm font-semibold hover:bg-slate-50 dark:hover:bg-raised">
              <span className="inline-flex items-center gap-2">
                <ClipboardText className="size-4" />Plan
                {!!planDoc.data?.revision && <span className="text-xs font-medium tabular-nums text-slate-600 dark:text-muted">V{planDoc.data.revision}</span>}
              </span>
              <span className="ml-auto flex flex-wrap items-center gap-2 text-xs font-medium text-slate-600 dark:text-muted">
                {planAccepted ? (
                  <span className="inline-flex items-center gap-1 text-emerald-700 dark:text-emerald-400"><Check weight="thin" aria-hidden="true" className="size-3.5 shrink-0" />Aceptado</span>
                ) : hasPlanContent ? (
                  <span className="text-amber-700 dark:text-amber-400">Pendiente de aceptar</span>
                ) : planDoc.data ? "Sin plan" : planDoc.isError ? "Estado no disponible" : "Cargando…"}
                <CaretDown className="size-4 transition-transform group-open:rotate-180" />
              </span>
            </summary>
            <PlanDoc taskId={id} showStatus={false} reader onAccepted={() => { if (planSection.current) planSection.current.open = false; }} />
          </details>
        </div>

        <aside className={`min-w-0 space-y-6 ${side ? "" : "md:border-l md:border-slate-200 md:pl-6 md:dark:border-border"}`}>
          <section>
            <h3 className={heading}><LinkSimple className="size-4" />Dependencias</h3>
            {!deps.data?.length && <p className="px-2 text-sm text-slate-500 dark:text-muted">No depende de otras tareas.</p>}
            {(deps.data ?? []).map((d) => {
              const o = others.find((x) => x.id === d.depends_on_id);
              const open = c.blockers.includes(d.depends_on_id);
              return (
                <div key={d.depends_on_id} className="flex items-center gap-2 px-2 py-1 text-sm">
                  <span aria-label={open ? "Pendiente" : "Terminada"} className={`size-2 shrink-0 rounded-full ${open ? "bg-amber-500" : "bg-emerald-500"}`} />
                  <Link to="/tasks" search={{ task: d.depends_on_id } as never}
                    className={`min-w-0 flex-1 wrap-break-word hover:underline ${open ? "" : "text-slate-500 line-through dark:text-muted"}`}>
                    {o?.title ?? d.depends_on_id}
                  </Link>
                  <button type="button" className={iconButton} aria-label="Quitar dependencia" onClick={() => dep.mutate({ other: d.depends_on_id, add: false })}><X weight="regular" className="size-4" /></button>
                </div>
              );
            })}
            <Select plain className="mt-2" value="" disabled={dep.isPending || !deps.data || !board.data} onChange={(v) => v && dep.mutate({ other: v, add: true })} placeholder="Añadir dependencia…"
              options={others.filter((o) => !deps.data?.some((d) => d.depends_on_id === o.id)).map((o) => ({ value: o.id, label: o.title }))} />
            <ErrorText error={dep.error ?? deps.error ?? board.error} />
          </section>

          <section>
            <h3 className={heading}><Chats className="size-4" />Hilos</h3>
            {!c.threads.length && <p className="px-2 text-sm text-slate-500 dark:text-muted">Aún no hay hilos. Se crean al definir el plan o implementar.</p>}
            {c.threads.map((t) => (
              <Link key={t.id} to="/threads/$threadId" params={{ threadId: t.id }} className={`${row} gap-3`}>
                <ThreadDot status={t.status} />
                <span className="min-w-0 flex-1 truncate" title={t.title}>{t.title || "Hilo"}</span>
                <span role="img" aria-label={t.provider} title={t.provider} className="shrink-0">
                  <ModelLogo driver={t.provider} compact />
                </span>
                <CaretRight aria-hidden="true" className="size-3 shrink-0 text-slate-500" />
              </Link>
            ))}
          </section>
        </aside>
      </div>
    </>)
  );
}

