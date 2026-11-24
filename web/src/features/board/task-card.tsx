import { useId, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { CircleNotch, GitBranch, GitMerge, GitPullRequest, CircleDashed } from "@phosphor-icons/react";
import { api, type Card, type Pull, type Thread } from "../../api/client";
import { useThreadActivity } from "../../api/activity";
import { useArm } from "../github/github-ui";
import { EnvIcon } from "../../components/env-icon";
import { qk } from "../../api/queries/keys";
import { useMesh, useNode } from "../../api/queries/hooks";
import { PAD, GAP, MORE_H, ago, lastActivity, status } from "./card-layout";
import { slot } from "./card-status";

// ColumnCards shows the cards that fit in the visible height. It measures the real height of each card in a hidden copy,
// so cards with less content leave room for more, and the rest go behind a "+N más" button.
export function ColumnCards({ list, all, onExpand, card }: { list: Card[]; all: boolean; onExpand: () => void; card: (c: Card) => React.ReactNode }) {
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
export function MergeButton({ c, pull }: { c: Card; pull: Pull }) {
  const qc = useQueryClient();
  const merge = useMutation({
    mutationFn: () => api.githubAct(pull.repo, "pulls", pull.number, { type: "merge", method: "SQUASH" }),
    onSettled: () => qc.invalidateQueries({ queryKey: qk.board() }),
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
export const linkedAt = (t: Thread) => t.task_linked_at ?? t.created_at;

// Avatar is a GitHub user's picture with its review state: green approved, amber pending, red changes requested.
export function Avatar({ login, state }: { login: string; state: "approved" | "pending" | "changes" }) {
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
export function RefLink({ repo, kind, number }: { repo?: string; kind: "issues" | "pulls"; number: number }) {
  const Icon = kind === "pulls" ? GitPullRequest : CircleDashed;
  const label = kind === "pulls" ? `Pull request #${number}` : `Issue #${number}`;
  const cls = "inline-flex shrink-0 items-center gap-2 text-slate-600 no-underline dark:text-text-secondary";
  const m = repo?.match(/^([\w.-]+)\/([\w.-]+)$/);
  const body = <><Icon aria-hidden="true" className="size-4 shrink-0" />#{number}</>;
  if (!m) return <span className={cls} title={label}>{body}</span>;
  return <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: m[1], repo: m[2], kind, number: String(number) }} draggable={false} title={label} aria-label={label} className={`${cls} hover:text-slate-900 dark:hover:text-white`}>{body}</Link>;
}

// Reviewers shows who reviewed the pull request and who still has to.
export function Reviewers({ pull }: { pull: Pull }) {
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

// ThreadEnv draws the environment a thread runs in, which may be a peer of this node.
export function ThreadEnv({ nodeId, className }: { nodeId: string; className: string }) {
  const node = useNode({ staleTime: 60_000 });
  const mesh = useMesh({ staleTime: 60_000 });
  const self = node.data?.node;
  const peer = mesh.data?.peers.find((p) => p.id === nodeId);
  const env = self && nodeId === self.id ? self : peer?.info.node;
  return <EnvIcon icon={env?.icon} os={env?.os} name={env?.name ?? peer?.name ?? `entorno ${nodeId.slice(0, 8)}`} className={className} />;
}

// ThreadRow links to the latest thread and, with more than one, opens a menu with all of them.
export function ThreadRow({ threads }: { threads: Thread[] }) {
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
  const link = "inline-flex min-w-0 items-center text-slate-600 no-underline dark:text-text-secondary";
  return (
    <div className="flex items-center gap-1.5 text-xs" onClick={(e) => e.stopPropagation()}>
      <Link to="/threads/$threadId" params={{ threadId: t.id }} title={t.title} draggable={false} className={`${link} gap-2 rounded-md border border-slate-200 px-1.5 py-0.5 hover:bg-slate-100 dark:border-white/10 dark:hover:bg-white/5`}>
        <ThreadEnv nodeId={t.node_id} className="size-4 shrink-0" />
        <span className="min-w-0 truncate">{t.model || t.provider}</span>
        <ThreadDot t={t} />
      </Link>
      {sorted.length > 1 && (<>
        <button type="button" popoverTarget={id} onClick={place} aria-label={`Ver los ${sorted.length} hilos`} className="rounded-md px-1.5 py-0.5 font-medium text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-white/5">+{sorted.length - 1}</button>
        <div ref={menu} id={id} popover="auto" role="menu" className="fixed m-0 w-72 rounded-xl border border-slate-200 bg-white p-1 text-slate-900 shadow-xl dark:border-border-strong dark:bg-surface dark:text-white">
          {sorted.map((x) => (
            <Link key={x.id} to="/threads/$threadId" params={{ threadId: x.id }} role="menuitem" className="flex items-center gap-2 rounded-md px-2.5 py-2 text-sm text-inherit no-underline hover:bg-slate-100 dark:hover:bg-raised">
              <ThreadDot t={x} />
              <ThreadEnv nodeId={x.node_id} className="size-4 shrink-0" />
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
export function TaskCard({ c, onOpen, onDrag, onMenu }: { c: Card; onOpen: () => void; onDrag: () => void; onMenu: (x: number, y: number) => void }) {
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
        {where && <p className="flex items-center gap-2 text-xs text-slate-500 dark:text-muted"><GitBranch aria-hidden="true" className="size-4 shrink-0" /><span className="truncate">{where}</span></p>}
        {(c.issue_number || c.pr_number) && (
          <div className="flex items-center gap-3 text-xs" onClick={(e) => e.stopPropagation()}>
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
export function ThreadDot({ t }: { t: Thread }) {
  const status = t.status;
  const d = t.state_stale ? { label: "Sin conexión", color: "bg-slate-400", pulse: false }
    : status === "error" ? { label: "Error", color: "bg-red-500", pulse: false }
    : status === "waiting" ? { label: "Te necesita", color: "bg-amber-500", pulse: true }
    : status === "working" ? { label: "Trabajando", color: "bg-blue-500", pulse: true }
    : t.unread ? { label: "Terminado, sin leer", color: "bg-emerald-500", pulse: false }
    : { label: "Inactivo", color: "bg-slate-400", pulse: false };
  return <span role="img" title={d.label} aria-label={d.label} className={`size-2 shrink-0 rounded-full ${d.color} ${d.pulse ? "motion-safe:animate-pulse" : ""}`} />;
}

