import { DictationField } from "../composer/dictation-field";
import { PlanDoc } from "../../components/plan-doc";
import { useDictationInProgress } from "../composer/local-dictation";
import { useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check, CaretDown, CaretRight, PencilSimple, Chats, GithubLogo, LinkSimple, ClipboardText, DotsThree, GitBranch, GitPullRequest, Rocket, X } from "@phosphor-icons/react";
import { api, COLUMN_LABELS, repoName, type Column, type Pull, type Stage } from "../../api/client";
import { Button, ErrorText, iconButton, Input, Label, Md, Modal, Select } from "../../components/ui";
import { useLaunchForm } from "../composer/launch-picker";
import { ModelLogo } from "../../components/model-logo";
import { askConfirm } from "../../components/confirm";
import { type QuickLaunch } from "./task-quick-menu";
import { githubRoute } from "../../components/refs";
import { planMessage } from "../../lib/auto-message";
import { qk } from "../../api/queries/keys";
import { useBoard, useCard, usePlan, useRepositories } from "../../api/queries/hooks";
import { slot } from "./card-status";
import { ThreadDot } from "./task-card";

export const COLUMN_TONE: Record<Column, string> = {
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
  const card = useCard(id);
  const plans = useQuery({ queryKey: qk.plans(id), queryFn: () => api.plans(id) });
  const planDoc = usePlan(id);
  const deps = useQuery({ queryKey: qk.dependencies(id), queryFn: () => api.dependencies(id) });
  const board = useBoard();
  const repos = useRepositories();
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
  const refresh = () => { qc.invalidateQueries({ queryKey: qk.card(id) }); qc.invalidateQueries({ queryKey: qk.board() }); };
  const save = useMutation({
    mutationFn: () => api.tasks.update(id, { title: draft!.title, description: draft!.description, issue_number: draft!.issue ? Number(draft!.issue) : null, pr_number: draft!.pr ? Number(draft!.pr) : null, repository_id: draft!.repo || null }),
    onSuccess: () => { refresh(); setEditing(false); },
  });
  const stage = useMutation({ mutationFn: (s: Stage) => api.tasks.update(id, { stage: s }), onSuccess: refresh });
  const dep = useMutation({
    mutationFn: ({ other, add }: { other: string; add: boolean }) => (add ? api.addDependency(id, other) : api.removeDependency(id, other)).then(() => undefined),
    onSuccess: () => { qc.invalidateQueries({ queryKey: qk.dependencies(id) }); refresh(); },
  });
  const archive = useMutation({ mutationFn: () => api.tasks.archive(id), onSuccess: () => { refresh(); onClose(); } });
  const form = useLaunchForm({ section: "task", project: c?.project_id, borderless: true });
  // Sending opens the thread in the background; with go it also moves to it.
  const sent = (r: { thread_id: string }, go: boolean) => {
    if (go) return navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } });
    launchPopover.current?.hidePopover();
    refresh();
    qc.invalidateQueries({ queryKey: qk.threads() });
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
  // frame is the modal on the board and a plain card in a thread.
  const frame = (title: string, children: React.ReactNode) => side ? (
    <div role="region" aria-label={title} className="space-y-4">
      <div className="flex items-start justify-between gap-2">
        <h2 className="min-w-0 wrap-break-word text-lg font-semibold leading-tight">{title}</h2>
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

      {side ? (() => {
        const at = Math.max(0, steps.findIndex((s) => s.state !== "done"));
        const current = steps.every((s) => s.state === "done") ? steps.length - 1 : at;
        return (
          <div aria-label="Progreso de la tarea" className="space-y-1.5">
            <ol className="grid grid-cols-4 gap-1">
              {steps.map((s) => (
                <li key={s.label} title={s.label} aria-label={`${s.label}: ${s.state === "done" ? "hecho" : s.state === "now" ? "en curso" : "pendiente"}`}
                  className={`h-1.5 rounded-full ${s.state === "done" ? "bg-emerald-600" : s.state === "now" ? "bg-slate-900 dark:bg-white" : "bg-slate-200 dark:bg-border-strong"}`} />
              ))}
            </ol>
            <p className="text-xs text-slate-500 dark:text-muted">Paso {current + 1} de {steps.length} · <span className="font-medium text-slate-900 dark:text-white">{steps[current].label}</span></p>
          </div>
        );
      })() : (
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
      )}

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
                <ThreadDot t={t} />
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


