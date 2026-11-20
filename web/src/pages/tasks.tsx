import { DictationField } from "../components/dictation-field";
import { PlanDoc } from "../components/plan-doc";
import { useDictationInProgress } from "../components/local-dictation";
import { useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowSquareOut, Check, CaretDown, CaretRight, PencilSimple, Chats, GithubLogo, LinkSimple, ChatCircle, CircleNotch, ClipboardText, DotsThree, GitBranch, GitMerge, GitPullRequest, Plus, Rocket, X } from "@phosphor-icons/react";
import { api, COLUMN_LABELS, repoName, type Card, type Column, type Stage } from "../api/client";
import { Button, Card as UiCard, ErrorText, iconButton, Input, Label, Md, Modal, Select } from "../components/ui";
import { useLaunchForm } from "../components/launch-picker";
import { useProject } from "../project";
import { ModelLogo } from "../components/model-logo";
import { askConfirm } from "../components/confirm";
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

// Tasks is the kanban, the main view (requirement RF-28).
export function Tasks() {
  const project = useProject();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { task?: string };
  const board = useQuery({ queryKey: ["board", project], queryFn: () => api.board(project || undefined), refetchInterval: 30_000 });
  const [adding, setAdding] = useState(false);
  const [fold, setFold] = useState<Record<string, boolean>>({ done: true, cancelled: true });
  const [drag, setDrag] = useState<string>();
  const move = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: Stage }) => api.tasks.update(id, { stage }),
    onSettled: () => qc.invalidateQueries({ queryKey: ["board"] }),
  });
  const cards = board.data?.cards ?? [];
  const open = (id?: string) => navigate({ to: "/tasks", search: (id ? { task: id } : {}) as never });
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <h2 className="font-semibold">Kanban <span className="hidden text-xs font-normal text-slate-500 dark:text-muted sm:inline">(clic para abrir · arrastra para cambiar de estado)</span></h2>
        <Button className="ml-auto" onClick={() => setAdding(true)}><Plus weight="regular" className="size-4" />Nueva tarea</Button>
      </div>
      <ErrorText error={board.error ?? move.error} />
      <div className="flex flex-col gap-3 pb-2 sm:flex-row sm:overflow-x-auto">
        {(board.data?.columns ?? COLUMNS).map((col) => {
          const list = cards.filter((c) => c.column === col);
          const folded = fold[col] ?? false;
          const shown = folded ? list.slice(0, 0) : col === "done" || col === "cancelled" ? list.slice(-10) : list;
          return (
            <section key={col} className={`w-full space-y-2 rounded-lg p-2 sm:shrink-0 ${folded ? "sm:w-40" : "min-h-24 sm:min-h-48 sm:w-64"} ${drag && MANUAL[col] ? "bg-blue-100 dark:bg-accent-subtle" : "bg-slate-100 dark:bg-raised"}`}
              onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }}
              onDrop={() => { if (drag && MANUAL[col]) move.mutate({ id: drag, stage: MANUAL[col]! }); setDrag(undefined); }}>
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-text-secondary">{COLUMN_LABELS[col]}</h3>
                <div className="flex items-center gap-1">
                  <span className={`rounded-full px-2 text-xs font-medium ${COLUMN_TONE[col]}`}>{list.length}</span>
                  <button type="button" aria-expanded={!folded} aria-label={folded ? `Mostrar ${COLUMN_LABELS[col]}` : `Ocultar ${COLUMN_LABELS[col]}`} onClick={() => setFold({ ...fold, [col]: !folded })} className="rounded px-1 text-xs text-slate-500 hover:bg-slate-200 dark:text-muted dark:hover:bg-hover">{folded ? "▸" : "▾"}</button>
                </div>
              </div>
              <div className="space-y-2">
                {shown.map((c) => <TaskCard key={c.id} c={c} onOpen={() => open(c.id)} onDrag={() => setDrag(c.id)} />)}
              </div>
            </section>
          );
        })}
      </div>
      {adding && <NewTask onClose={() => setAdding(false)} onCreated={(id) => { setAdding(false); open(id); }} />}
      {search.task && <TaskDetail key={search.task} id={search.task} onClose={() => open()} />}
    </div>
  );
}

const chip = "inline-flex max-w-full items-center gap-1.5 rounded-md border border-slate-200 px-2 py-0.5 text-xs text-slate-600 no-underline hover:bg-slate-100 dark:border-border dark:text-muted dark:hover:bg-white/5";

// TaskCard keeps the v1 card: title and state icon, then chips for the latest
// thread and the pull request.
function TaskCard({ c, onOpen, onDrag }: { c: Card; onOpen: () => void; onDrag: () => void }) {
  const s = slot(c);
  const t = c.threads.length ? c.threads.reduce((a, b) => ((b.last_activity_at ?? b.updated_at) >= (a.last_activity_at ?? a.updated_at) ? b : a)) : undefined;
  return (
    <div draggable onDragStart={onDrag} onClick={onOpen} className="cursor-grab">
      <UiCard>
        <div className="text-sm">
          <div className="flex items-start justify-between gap-2">
            <span className="min-w-0 font-medium">{c.title}</span>
            <span className="mt-0.5 grid size-4 shrink-0 place-items-center" title={s.label} aria-label={s.label}>{s.icon}</span>
          </div>
          {c.blockers.length > 0 && <p className="mt-1.5 rounded-md bg-slate-50 px-2 py-1.5 text-xs text-slate-600 dark:bg-raised dark:text-text-secondary"><span className="font-medium text-red-600 dark:text-red-400">Bloqueada: </span>depende de tareas sin terminar</p>}
          {(t || c.pr_number || c.issue_number || c.has_plan) && (
            <div className="mt-1 flex flex-wrap items-center gap-1" onClick={(e) => e.stopPropagation()}>
              {t && (
                <Link to="/threads/$threadId" params={{ threadId: t.id }} title={t.title} className={chip}>
                  <ChatCircle aria-hidden="true" className="size-3 shrink-0" />
                  <span className="min-w-0 truncate">{t.model || t.provider}</span>
                  {t.effort && <span className="shrink-0 opacity-60">· {t.effort}</span>}
                  <ThreadDot status={t.status} />
                </Link>
              )}
              {c.pr_number && (c.pull?.url
                ? <a href={c.pull.url} target="_blank" rel="noreferrer" title={`Pull request #${c.pr_number}`} className={chip}><GitPullRequest aria-hidden="true" className="size-3 shrink-0" /><span>#{c.pr_number}</span></a>
                : <span className={chip}><GitPullRequest aria-hidden="true" className="size-3 shrink-0" /><span>#{c.pr_number}</span></span>)}
              {c.issue_number && <span className={chip}>issue #{c.issue_number}</span>}
              {c.has_plan && <span className={chip} title="Tiene plan"><ClipboardText aria-hidden="true" className="size-3 shrink-0" />plan</span>}
            </div>
          )}
        </div>
      </UiCard>
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

function NewTask({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const current = useProject();
  const dictating = useDictationInProgress();
  const [project, setProject] = useState(current);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [issue, setIssue] = useState("");
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const create = useMutation({
    mutationFn: () => api.tasks.create({ project_id: project || null, title: title.trim(), description, issue_number: issue ? Number(issue) : null }),
    onSuccess: (t) => onCreated(t.id),
  });
  return (
    <Modal title="Nueva tarea" onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (!dictating && title.trim()) create.mutate(); }}>
        <Label text="Título"><Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} /></Label>
        <DictationField label="Descripción" rows={5} value={description} onValue={setDescription} placeholder="Qué cambiar y cómo comprobarlo" />
        <div className="grid grid-cols-2 gap-2">
          <Label text="Proyecto"><Select value={project} onChange={setProject} options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} /></Label>
          <Label text="Issue"><Input inputMode="numeric" value={issue} onChange={(e) => setIssue(e.target.value.replace(/\D/g, ""))} placeholder="Opcional" /></Label>
        </div>
        <ErrorText error={create.error} />
        <Button disabled={dictating || !title.trim() || create.isPending}>Crear</Button>
      </form>
    </Modal>
  );
}

function TaskDetail({ id, onClose }: { id: string; onClose: () => void }) {
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
    const { right, top, bottom } = trigger.getBoundingClientRect();
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
  const launch = useMutation({
    mutationFn: () => { form.remember(); return api.launch(id, form.value); },
    onSuccess: (r) => navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }),
  });
  const define = useMutation({
    mutationFn: () => {
      form.remember();
      const text = planMessage({ id, title: c!.title, description: c!.description });
      return api.createThread({ ...form.value, project_id: c!.project_id, task_id: id, title: `Definir: ${c!.title}`, text });
    },
    onSuccess: (r) => navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }),
  });
  if (!c || !draft) return <Modal title="Tarea" onClose={onClose}><ErrorText error={card.error} /></Modal>;
  const dirty = draft.title !== c.title || draft.description !== c.description || draft.issue !== (c.issue_number?.toString() ?? "") || draft.pr !== (c.pr_number?.toString() ?? "") || draft.repo !== (c.repository_id ?? "");
  const others = (board.data?.cards ?? []).filter((o) => o.id !== id && (!c.project_id || o.project_id === c.project_id));
  const latest = [...(plans.data ?? [])].sort((a, b) => b.version - a.version)[0];
  // The latest plan has to be accepted before the task is implemented.
  const planPending = !!latest && !latest.accepted_at;
  const planAccepted = !!planDoc.data?.accepted_at;
  const hasPlanContent = !!planDoc.data?.body_md.trim();
  const reviewPlan = hasPlanContent && !planAccepted;
  const launchLabel = planAccepted ? "Implementar" : "Definir plan";
  const launchAction = planAccepted ? launch : define;
  const launchPending = launch.isPending || define.isPending;
  const launchBlocked = planAccepted && (c.blockers.length > 0 || planPending);
  const environment = form.nodes.find((node) => node.value === form.value.node)?.label ?? "Este nodo";
  const projectRepos = (repos.data ?? []).filter((r) => !c.project_id || r.project_id === c.project_id);
  const githubRepo = c.repo && /^[\w.-]+\/[\w.-]+$/.test(c.repo) ? `https://github.com/${c.repo}` : undefined;
  const pullNumber = c.pull?.number ?? c.pr_number;
  const pullUrl = c.pull?.url ?? (githubRepo && pullNumber ? `${githubRepo}/pull/${pullNumber}` : undefined);
  const openLaunch = (anchor: HTMLElement | null) => {
    launchAction.reset();
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
  const launchButton = (label: string, disabled = false) => (
    <Button ref={launchTrigger} onClick={(e) => openLaunch(e.currentTarget)} disabled={disabled || !planDoc.data || launchPending}><Rocket className="size-4" />{label}</Button>
  );
  const prLink = pullUrl && <a href={pullUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-700 dark:bg-accent dark:hover:bg-accent-hover"><GitPullRequest className="size-4" />Abrir PR<ArrowSquareOut className="size-3.5" /></a>;
  const next: Next =
    c.column === "cancelled" ? { tone: tones.idle, title: "Tarea cancelada", detail: "No se trabajará en ella salvo que la reabras.", action: <Button variant="ghost" onClick={() => stage.mutate("inbox")}>Reabrir</Button> }
    : merged ? { tone: tones.ok, title: "Completada", detail: `La PR #${pullNumber} está fusionada.`, action: prLink }
    : waiting ? { tone: tones.warn, title: "Un agente necesita tu respuesta", detail: waiting.title, action: threadLink(waiting, "Responder") }
    : working ? { tone: tones.info, title: "Un agente está trabajando", detail: working.title, action: threadLink(working, "Ver hilo") }
    : failed && !c.pull ? { tone: tones.bad, title: "El hilo falló", detail: "Revisa el error o lanza otro hilo.", action: threadLink(failed, "Ver hilo") }
    : c.pull ? { tone: c.pull.ci === "FAILURE" || c.pull.ci === "ERROR" || c.pull.mergeable === "CONFLICTING" ? tones.bad : c.pull.draft ? tones.info : tones.ok, title: `PR #${pullNumber}: ${slot(c).label}`, detail: c.pull.draft ? "La PR sigue en borrador hasta que la marques como lista." : undefined, action: prLink }
    : reviewPlan ? { tone: tones.warn, title: `Revisa y acepta el plan${planDoc.data?.revision ? ` V${planDoc.data.revision}` : ""}`, detail: "Hasta aceptarlo no se puede implementar.", action: <Button onClick={openPlan}><ClipboardText className="size-4" />Revisar plan</Button> }
    : !planAccepted ? { tone: tones.idle, title: "Define el plan", detail: c.description.trim() ? "Un agente propondrá un plan a partir de la descripción." : "Añade una descripción para que el agente sepa qué hacer.", action: launchButton("Definir plan") }
    : c.blockers.length ? { tone: tones.warn, title: `Bloqueada por ${c.blockers.length === 1 ? "una dependencia" : `${c.blockers.length} dependencias`}`, detail: `Termina antes: ${blockerTitles.join(", ")}.`, action: launchButton("Implementar", true) }
    : { tone: tones.ok, title: "Lista para implementar", detail: "El plan está aceptado y no hay bloqueos.", action: launchButton("Implementar") };
  // steps shows where the task is in its path: description, plan, implementation and pull request.
  const steps: { label: string; state: "done" | "now" | "todo" }[] = [
    { label: "Descripción", state: c.description.trim() ? "done" : "now" },
    { label: "Plan", state: planAccepted ? "done" : hasPlanContent ? "now" : c.description.trim() ? "now" : "todo" },
    { label: "Implementación", state: c.pull ? "done" : planAccepted ? "now" : "todo" },
    { label: "PR", state: merged ? "done" : c.pull ? "now" : "todo" },
  ];
  const chip = "inline-flex max-w-full items-center gap-1.5 rounded-full border border-slate-200 px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100 dark:border-border dark:text-text-secondary dark:hover:bg-raised";
  const heading = "mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-muted";
  const row = "flex min-w-0 items-center gap-2 rounded-lg px-2 py-2 text-sm hover:bg-slate-100 dark:hover:bg-raised";
  const menuItem = "flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-slate-100 disabled:opacity-50 dark:hover:bg-raised";
  return (
    <Modal title={c.title} onClose={onClose} className="!max-w-5xl sm:!p-6 [&>div:first-child>h2]:text-2xl [&>div:first-child>h2]:leading-tight">
      <div className="flex flex-wrap items-center gap-2">
        <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${COLUMN_TONE[c.column]}`}>{COLUMN_LABELS[c.column]}</span>
        {c.repo && (githubRepo ? <a className={chip} href={githubRepo} target="_blank" rel="noreferrer"><GithubLogo className="size-3.5 shrink-0" /><span className="truncate">{c.repo}</span></a> : <span className={chip}>{c.repo}</span>)}
        {c.issue_number && (githubRepo ? <a className={chip} href={`${githubRepo}/issues/${c.issue_number}`} target="_blank" rel="noreferrer">Issue #{c.issue_number}</a> : <span className={chip}>Issue #{c.issue_number}</span>)}
        {pullNumber && (pullUrl ? <a className={chip} href={pullUrl} target="_blank" rel="noreferrer"><GitPullRequest className="size-3.5" />PR #{pullNumber}{(c.pull?.draft || c.pr_state === "draft") && " · borrador"}</a> : <span className={chip}>PR #{pullNumber}</span>)}
        {c.branch && <span className={`${chip} font-mono`} title={c.branch}><GitBranch className="size-3.5 shrink-0" /><span className="truncate">{c.branch}</span></span>}
        <button ref={menuTrigger} type="button" popoverTarget={menuId} onClick={openMenu} aria-label="Más acciones" className={`${iconButton} ml-auto`}><DotsThree weight="bold" className="size-5" /></button>
        <div ref={menu} id={menuId} popover="auto" role="menu" className="fixed m-0 w-56 rounded-xl border border-slate-200 bg-white p-1 text-slate-900 shadow-xl dark:border-border dark:bg-surface dark:text-white">
          <button type="button" role="menuitem" className={menuItem} onClick={() => { setEditing(true); menu.current?.hidePopover(); }}><PencilSimple className="size-4" />Editar tarea</button>
          <button type="button" role="menuitem" className={menuItem} disabled={!planDoc.data || launchPending} onClick={() => openLaunch(menuTrigger.current)}><Rocket className="size-4" />Abrir otro hilo</button>
          {c.stage !== "ready" && c.column === "inbox" && <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("ready")}>Marcar como lista</button>}
          {c.stage === "ready" && c.column === "ready" && <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("inbox")}>Volver a inbox</button>}
          {c.stage !== "cancelled" ? <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("cancelled")}>Cancelar tarea</button> : <button type="button" role="menuitem" className={menuItem} onClick={() => stage.mutate("inbox")}>Reabrir</button>}
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
        {launchBlocked && <p className="mt-3 text-sm text-slate-600 dark:text-muted">{c.blockers.length ? "Resuelve las dependencias para implementar." : "Acepta el plan para implementar."}</p>}
        <ErrorText error={launchAction.error ?? form.error} />
        <div className="mt-4 flex justify-end">
          <Button onClick={() => launchAction.mutate()} disabled={!form.ready || launchPending || launchBlocked}>
            {launchPending ? "Abriendo hilo…" : "Abrir hilo"}
          </Button>
        </div>
      </div>

      <div className="grid items-start gap-6 md:grid-cols-[minmax(0,1fr)_minmax(0,16rem)]">
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
            <PlanDoc taskId={id} showStatus={false} />
          </details>
        </div>

        <aside className="min-w-0 space-y-6 md:border-l md:border-slate-200 md:pl-6 md:dark:border-border">
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
    </Modal>
  );
}

