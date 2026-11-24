import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent, type RefObject } from "react";
import { useCommand } from "../components/shortcuts";
import { hasWindowClose } from "../lib/window-close";
import { PlanDoc } from "../components/plan-doc";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { Circle, CircleNotch, ClockCounterClockwise, GitDiff, Kanban, WarningCircle, XCircle } from "@phosphor-icons/react";
import { api, type Message } from "../api/client";
import { setTurnFilter, useTurnFilter } from "../components/turn-changes";
import { turnDiff } from "../lib/turn-changes";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, uuid } from "../components/ui";
import { changeTurns, Messages, ModelChangeDivider, ToEnd, useFollow } from "./chat";
import { Chip, Composer, Queue } from "./composer";
import { QuoteSelection } from "../components/quote-selection";
import { ProviderIcon } from "../components/launch-picker";
import { Diff, type Suggestion } from "./github-diff";
import { setDiffBase, useDiffBase } from "../lib/diff-base";
import { addDraftQuote, removeDraftQuotes, useDraftQuotes } from "../lib/draft-quotes";
import { headerButton, ThreadInfo } from "../components/thread-info";
import { ParentLink } from "../components/subagents";
import { TaskDetail } from "./tasks";
import { useThreadMenu } from "../components/thread-menu";
import { useThreadsFold } from "../lib/threads-fold";
import { BrowserPanel } from "../components/browser-panel";
import { DevicePanel, FloatingWindow } from "../components/device-panel";
import { kindOf, SidePanel, SidePanelToggle, useSidePanel, type SidePanelState } from "../components/side-panel";

// ThreadView is a whole thread page: header, chat and, on demand, its changes
// and the plan of its task.
export function ThreadView() {
  const { threadId } = useParams({ strict: false }) as { threadId: string };
  const panel = useSidePanel(threadId);
  useCommand("thread-panel", panel.toggle);
  const thread = useThread(threadId).data;
  const local = thread?.local;
  const taskId = thread?.task_id ?? null;
  // Until the thread loads its task is unknown, not missing.
  useOpenPlanOnAgentEdit(thread ? taskId : undefined, panel);
  // The agent opens or closes a device tab; the user watches what it does.
  const latest = useRef(panel);
  latest.current = panel;
  useEffect(() => {
    const on = (e: Event) => {
      const d = (e as CustomEvent<{ thread: string; action: "open" | "close"; serial: string; floating?: boolean }>).detail;
      if (d.thread !== threadId) return;
      const p = latest.current;
      const tab = p.tabs.find((k) => kindOf(k) === "device" && p.meta[k]?.serial === d.serial);
      if (d.action === "close") { if (tab) p.close(tab); return; }
      if (tab) { p.setMeta(tab, { floating: !!d.floating }); p.focus(tab); } else p.show("device", { serial: d.serial, floating: !!d.floating });
    };
    window.addEventListener("dory-device", on);
    return () => window.removeEventListener("dory-device", on);
  }, [threadId]);
  useEffect(() => {
    if (!window.doryDesktop?.browser) return;
    const on = (event: Event) => {
      if ((event as CustomEvent<string>).detail === threadId) latest.current.show("browser");
    };
    window.addEventListener("dory-browser-open", on);
    return () => window.removeEventListener("dory-browser-open", on);
  }, [threadId]);
  const deviceView = (k: string, floating = false) => <DevicePanel key={k} node={thread?.node_id ?? "local"} serial={panel.meta[k]?.serial ?? null} floating={floating} onFloat={(v) => panel.setMeta(k, { floating: v })}
    onSerial={(serial, name) => panel.setMeta(k, serial ? { serial, ...(panel.meta[k]?.name ? {} : { name }) } : { serial: undefined })} />;
  return (
    <div className="relative flex h-[calc(100dvh_-_4rem_-_1px_-_var(--tabbar))] min-h-0 sm:h-[calc(100dvh-3rem-2px)]">
      {/* The panel toggle is pinned to the page corner, so opening the panel never moves it. */}
      <div className={`absolute right-1 top-0 z-30 sm:-top-2 ${hasWindowClose ? "sm:right-8" : "sm:-right-1"}`}><SidePanelToggle panel={panel} /></div>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Header threadId={threadId} panel={panel} />
        <ThreadChat threadId={threadId} />
      </div>
      <SidePanel panel={panel} available={[...(local ? ["diff" as const] : []), ...(taskId ? ["plan" as const] : []), "device" as const, ...(window.doryDesktop?.browser ? ["browser" as const] : [])]}
        render={(k) => (k === "diff" ? <Changes threadId={threadId} messages={thread?.messages} /> : k === "browser" ? <BrowserPanel threadId={threadId} remote={local === false} /> : kindOf(k) === "device" ? (panel.open ? (panel.meta[k]?.floating
          ? <div className="grid h-full place-items-center p-6 text-center text-sm text-slate-500 dark:text-muted"><div className="space-y-2"><p>El dispositivo flota sobre el chat.</p><Button variant="ghost" onClick={() => panel.setMeta(k, { floating: false })}>Abrir en el panel lateral</Button></div></div>
          : deviceView(k)) : null) : taskId && <PlanDoc taskId={taskId} />)} />
      {panel.tabs.filter((k) => kindOf(k) === "device" && panel.meta[k]?.floating).map((k) => <FloatingWindow key={k} id={`${threadId}:${k}`}>{deviceView(k, true)}</FloatingWindow>)}
    </div>
  );
}

// useOpenPlanOnAgentEdit brings the plan tab to the front when the agent
// changes the plan of the thread's task, so the user watches it being written.
// taskId is undefined while the thread loads, so opening a thread is not taken
// for linking a task to it: the panel then stays as the user left it.
function useOpenPlanOnAgentEdit(taskId: string | null | undefined, panel: SidePanelState) {
  const plan = useQuery({ queryKey: ["plans", taskId, "doc"], queryFn: () => api.plan(taskId!), enabled: !!taskId });
  const seen = useRef<number | undefined>(undefined);
  const previousTask = useRef(taskId);
  const newlyLinked = useRef(false);
  const revision = plan.data?.revision;
  useEffect(() => { newlyLinked.current = previousTask.current === null && !!taskId; previousTask.current = taskId; seen.current = undefined; }, [taskId]);
  useEffect(() => {
    if (revision === undefined) return;
    const before = seen.current;
    seen.current = revision;
    if (((before !== undefined && revision > before) || (before === undefined && newlyLinked.current && revision > 0)) && plan.data?.author === "agent" && !(panel.open && panel.active === "plan")) panel.show("plan");
    if (revision > 0) newlyLinked.current = false;
  }, [revision]); // eslint-disable-line react-hooks/exhaustive-deps
}

function useThread(threadId?: string) {
  useThreadLive(threadId);
  return useQuery({ queryKey: ["thread", threadId], queryFn: () => api.thread(threadId!), enabled: !!threadId, staleTime: 5_000 });
}

// TaskCard opens the thread's task as a side card in the place of the details
// card, with the same component the board uses. While the side panel is open it
// closes on any click outside, like the details card.
function TaskCard({ taskId, open, setOpen, popup }: { taskId: string; open: boolean; setOpen: (v: boolean) => void; popup: boolean }) {
  const wrap = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!popup || !open) return;
    const close = (e: Event) => { if (!wrap.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close, true);
    return () => document.removeEventListener("pointerdown", close, true);
  }, [popup, open, setOpen]);
  return (
    <div ref={wrap}>
      <button type="button" onClick={() => setOpen(!open)} title="Ver la tarea" aria-label="Ver la tarea" aria-expanded={open} className={headerButton(open)}><Kanban weight="regular" className="size-4" /></button>
      {open && (
        <div className="notif-in absolute right-1 top-11 z-30 max-h-[calc(100dvh-7rem)] w-[min(24rem,calc(100vw-1rem))] overflow-y-auto rounded-xl border border-slate-200 bg-white p-4 shadow-xl dark:border-border dark:bg-surface">
          <TaskDetail key={taskId} id={taskId} layout="side" onClose={() => setOpen(false)} />
        </div>
      )}
    </div>
  );
}

function Header({ threadId, panel }: { threadId: string; panel: SidePanelState }) {
  const qc = useQueryClient();
  const t = useThread(threadId).data;
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  const act = useMutation({
    mutationFn: (body: Parameters<typeof api.threadAction>[1]) => api.threadAction(threadId, body),
    onSuccess: (_, body) => {
      qc.invalidateQueries({ queryKey: ["thread", threadId] });
      qc.invalidateQueries({ queryKey: ["threads"] });
    },
  });
  const menu = useThreadMenu(t);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const projectName = t?.project_id ? projects.data?.find((p) => p.id === t.project_id)?.name : undefined;
  // Opening a thread marks it as read.
  useEffect(() => { if (t?.unread && t.status !== "working") act.mutate({ action: "read" }); }, [t?.id, t?.status, t?.unread, t?.last_activity_at]); // eslint-disable-line react-hooks/exhaustive-deps
  const fold = useThreadsFold();
  // Like the details card, each thread remembers whether its task card was left open.
  const taskKey = `dory-thread-task:${threadId}`;
  const readTask = () => { try { return localStorage.getItem(taskKey) === "1"; } catch { return false; } };
  const [taskOpen, setTaskOpenState] = useState(readTask);
  useEffect(() => setTaskOpenState(readTask()), [taskKey]); // eslint-disable-line react-hooks/exhaustive-deps
  const setTaskOpen = useCallback((v: boolean) => { setTaskOpenState(v); try { if (v) localStorage.setItem(taskKey, "1"); else localStorage.removeItem(taskKey); } catch { /* per-viewer only */ } }, [taskKey]);
  if (!t) return <div className="h-12 shrink-0" />;
  return (
    <div className={`relative z-20 flex shrink-0 flex-wrap items-center gap-2 pb-2 pl-1 pr-11 transition-[padding,margin] ${fold ? (fold.folded ? "sm:pl-10" : "sm:pl-2") : ""} duration-300 ease-out motion-reduce:transition-none sm:-mt-2 ${hasWindowClose ? "sm:pr-20" : ""} ${panel.open ? "lg:!pr-1" : ""} ${panel.open ? "lg:mr-0 sm:-mr-2" : "sm:-mr-2"}`}>
      {projectName && <span className="flex min-w-0 max-w-[40%] shrink items-center gap-2 text-slate-500 dark:text-muted"><span className="truncate">{projectName}</span><span aria-hidden="true">/</span></span>}
      {editing ? (
        <form className="min-w-0 flex-1" onSubmit={(e) => { e.preventDefault(); setEditing(false); if (title.trim()) act.mutate({ action: "rename", title: title.trim() }); }}>
          <input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} onBlur={() => setEditing(false)} onKeyDown={(e) => { if (e.key === "Escape") setEditing(false); }} className="w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-border-strong" />
        </form>
      ) : (
        <h1 className="min-w-0 flex-1 cursor-text truncate font-semibold" title={`${t.title} · doble clic para renombrar`} onDoubleClick={() => { setTitle(t.title); setEditing(true); }} onContextMenu={menu.open}>{t.title || "Hilo"}</h1>
      )}
      {t.parent_id ? <ParentLink parentId={t.parent_id} /> : t.origin_id && <ParentLink parentId={t.origin_id} origin />}
      {t.task_id && <TaskCard taskId={t.task_id} open={taskOpen} setOpen={setTaskOpen} popup={panel.open} />}
      <ThreadInfo t={t} popup={panel.open} hidden={!!t.task_id && taskOpen} onOpen={() => setTaskOpen(false)} diff={panel.open && panel.active === "diff"} toggleDiff={() => { if (panel.open && panel.active === "diff") panel.toggle(); else { setTurnFilter(threadId, null); panel.show("diff"); } }} />
      <ErrorText error={act.error ?? menu.error} />
      {menu.menu}
    </div>
  );
}

export function StatusDot({ status }: { status: string }) {
  if (status === "working") return <CircleNotch weight="regular" className="size-4 shrink-0 animate-spin text-blue-500" aria-label="Trabajando" />;
  if (status === "waiting") return <WarningCircle className="size-4 shrink-0 text-orange-500" weight="fill" aria-label="Te necesita" />;
  if (status === "error") return <XCircle className="size-4 shrink-0 text-red-500" weight="fill" aria-label="Error" />;
  return <Circle className="size-3 shrink-0 text-slate-400" weight="fill" aria-label="Inactivo" />;
}

// ThreadChat is the conversation and its composer; the PR page embeds it too.
export function ThreadChat({ threadId }: { threadId?: string }) {
  return threadId ? <ThreadConversation key={threadId} threadId={threadId} /> : null;
}

function ThreadConversation({ threadId }: { threadId: string }) {
  const view = useThread(threadId);
  const t = view.data;
  const last = t?.last_turn_model;
  const pending = t?.local && last && (
    last.provider !== t.provider ||
    ((last.model !== null || (last.reported_model && t.model)) && last.model !== t.model && last.reported_model !== t.model)
  ) ? { from: last, to: { provider: t.provider, model: t.model }, handoff: last.provider !== t.provider } : undefined;
  const quotes = useDraftQuotes(threadId);
  const composerInput = useRef<HTMLTextAreaElement>(null);
  const { scroller, onScroll, follow, away } = useFollow([t?.messages, t?.queue, t?.running, quotes.length]);
  // Opening another thread starts at its end, wherever the last one was left.
  useLayoutEffect(() => { follow(); }, [threadId, !!t]); // eslint-disable-line react-hooks/exhaustive-deps
  // A new quote, from the chat or from the diff, takes the focus to the box.
  const quoteCount = useRef(quotes.length);
  useEffect(() => {
    if (quotes.length > quoteCount.current) composerInput.current?.focus({ preventScroll: true });
    quoteCount.current = quotes.length;
  }, [quotes.length]);
  const quote = (text: string, reply: string) => addDraftQuote(threadId, { id: uuid(), text, reply });
  const removeQuotes = (ids: string[]) => removeDraftQuotes(threadId, ids);
  // Entering the chat with the pointer takes the keyboard to its box, so
  // dictation and Enter work without a click after the side panels.
  const takeFocus = (e: ReactMouseEvent<HTMLElement>) => {
    const box = composerInput.current;
    if (!box || document.querySelector('dialog[open],[role=dialog],[role=alertdialog],[role=menu],[role=listbox]')) return;
    const current = document.activeElement;
    if (current && current !== document.body && e.currentTarget.contains(current)) return;
    if (window.getSelection()?.toString()) return;
    void window.doryDesktop?.browser?.focusUI?.();
    box.focus({ preventScroll: true });
  };
  if (view.error) return <div className="p-4"><ErrorText error={view.error} /></div>;
  return (
    <section className="relative flex min-h-0 flex-1 flex-col" onMouseEnter={takeFocus}>
      {t?.local && <QuoteSelection onQuote={quote} />}
      {/* No divider under the header: the conversation fades out as it scrolls under it. */}
      <div aria-hidden="true" className="pointer-events-none absolute left-0 right-3 top-0 z-10 h-10 bg-gradient-to-b from-slate-50 to-transparent dark:from-canvas" />
      <div ref={scroller} data-scroller onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto pt-10 pb-3 [overflow-anchor:none] [scrollbar-gutter:stable]">
        <div className="mx-auto w-full max-w-[55.2rem] space-y-4 px-3 sm:px-6">
          {t && t.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
          {t && <Messages key={t.id} threadId={t.id} messages={t.messages} running={t.running} />}
          {t && <Queue t={t} />}
          {t && !t.local && <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">Este hilo corre en otro nodo.</p>}
        </div>
      </div>
      <ToEnd away={away} follow={follow} />
      {/* The scroller always reserves its 10px scrollbar, so the box leaves the same 10px to line up with the messages. */}
      <div className="pr-2.5">
        {pending && <div className="mx-auto w-full max-w-[55.2rem] px-3 sm:px-6"><ModelChangeDivider change={pending} pending /></div>}
        {t && <Composer t={t} onSent={follow} area={composerInput} quotes={quotes} onRemoveQuotes={removeQuotes} />}
      </div>
    </section>
  );
}

function Changes({ threadId, messages }: { threadId: string; messages?: Message[] }) {
  const base = useDiffBase(threadId);
  // The tab shows the whole thread against its base, or one turn the chat's box chose.
  const filter = useTurnFilter(threadId);
  const turns = useMemo(() => changeTurns(messages ?? []), [messages]);
  const turnIndex = turns.findIndex((x) => x.id === filter.turn);
  const turn = turnIndex >= 0 ? turns[turnIndex] : undefined;
  const turnText = useMemo(() => (turn ? turnDiff(turn.files) : ""), [turn]);
  const focus = useMemo(() => (filter.path ? { path: filter.path, seq: filter.seq } : undefined), [filter]);
  // The diff's buttons (files, view, full screen) go in this header, beside the choice of base.
  const [tools, setTools] = useState<HTMLElement | null>(null);
  // A comment on selected lines is added to the thread's message box as a
  // reference to that code; it goes with the next message the user sends.
  const comment = useCallback(async (s: Suggestion) => {
    addDraftQuote(threadId, { id: uuid(), text: s.snippet, reply: s.question, code: { path: s.path, start: s.start, end: s.end } });
  }, [threadId]);
  const diff = useQuery({ queryKey: ["thread-diff", threadId, base], queryFn: () => api.threadDiff(threadId, base), refetchInterval: 5000 });
  return (
    <>
      {diff.data?.branch && <div className="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs text-slate-500 dark:border-border">
        {turns.length > 0 && <Chip below title="Qué cambios se muestran" icon={<ClockCounterClockwise className="size-4 shrink-0" />} value={turn ? turn.id : ""} onChange={(v) => setTurnFilter(threadId, v || null)}
          options={[{ value: "", label: "Todo el hilo" }, ...turns.map((x, i) => ({ value: x.id, label: `Turno ${i + 1} · ${x.files.length} ${x.files.length === 1 ? "fichero" : "ficheros"}` }))]} />}
        {!turn && diff.data.base && <Chip below title="Contra qué se comparan los cambios" icon={<GitDiff className="size-4 shrink-0" />} value={diff.data.base} onChange={(v) => setDiffBase(threadId, v)}
          options={[{ value: "HEAD", label: "Sin commitear" }, ...[...new Set([...(diff.data.branches ?? []), ...(diff.data.base !== "HEAD" ? [diff.data.base] : [])])].map((b) => ({ value: b, label: `Contra ${b}` }))]} />}
        <span ref={setTools} className="flex items-center gap-1.5" />
        <span className="ml-auto min-w-0 truncate font-mono">{diff.data.branch}</span>
      </div>}
      <ErrorText error={diff.error} />
      {turn ? <Diff key={turn.id} text={turnText} focus={focus} fill toolsHost={tools} onSuggest={comment} steerable={false} />
        : diff.data && (diff.data.diff ? <Diff text={diff.data.diff} fill toolsHost={tools} onSuggest={comment} steerable={false} /> : <p className="p-4 text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>)}
    </>
  );
}

// ResizeHandle lets a side panel be dragged wider or narrower.
export function ResizeHandle({ panel, width, onResize, min = 320, rest = 420 }: { panel: RefObject<HTMLElement | null>; width: number; onResize: (w: number) => void; storageKey?: string | null; min?: number; rest?: number }) {
  return (
    <div role="separator" aria-orientation="vertical" tabIndex={0}
      className="w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-slate-300 dark:hover:bg-border"
      onPointerDown={(e) => {
        // Dragging must not select the conversation or the diff it crosses.
        e.preventDefault();
        window.getSelection()?.removeAllRanges();
        const body = document.body.style, prev = [body.userSelect, body.cursor];
        body.userSelect = "none";
        body.cursor = "col-resize";
        // The panel animates its width when it opens or closes; while dragging
        // it must follow the pointer at once, or widening lags behind the content.
        const el = panel.current, transition = el?.style.transition ?? "";
        if (el) el.style.transition = "none";
        const start = e.clientX, from = width;
        const right = panel.current?.getBoundingClientRect().right ?? window.innerWidth;
        const move = (ev: PointerEvent) => onResize(Math.max(min, Math.min(from + start - ev.clientX, right - rest)));
        const up = () => {
          [body.userSelect, body.cursor] = prev;
          if (el) el.style.transition = transition;
          window.removeEventListener("pointermove", move);
          window.removeEventListener("pointerup", up);
          window.removeEventListener("pointercancel", up);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up);
        window.addEventListener("pointercancel", up);
      }} />
  );
}

// ExternalView shows a conversation started in a harness, read-only, and
// continues it as a Dory thread that resumes the same session.
export function ExternalView() {
  const { node, provider, id } = useParams({ strict: false }) as { node: string; provider: string; id: string };
  const navigate = useNavigate();
  const qc = useQueryClient();
  const view = useQuery({ queryKey: ["external", node, provider, id], queryFn: () => api.externalSession(node, provider, id) });
  const adopt = useMutation({
    mutationFn: () => api.adoptSession(node, provider, id),
    onSuccess: (r) => { qc.invalidateQueries({ queryKey: ["threads"] }); qc.invalidateQueries({ queryKey: ["external"] }); navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }); },
  });
  const s = view.data?.session;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-slate-200 px-3 py-2 dark:border-border">
        <ProviderIcon driver={provider} />
        <h1 className="min-w-0 flex-1 truncate font-semibold">{s?.title ?? "Conversación"}</h1>
        {s?.dir && <span className="hidden truncate font-mono text-xs text-slate-500 sm:block">{s.dir}</span>}
        <Button onClick={() => adopt.mutate()} disabled={adopt.isPending}>Continuar en Dory</Button>
      </div>
      <div data-scroller className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[55.2rem] space-y-4 px-3 py-3 sm:px-6">
          <ErrorText error={view.error ?? adopt.error} />
          <p className="rounded-md bg-slate-100 px-3 py-2 text-xs text-slate-600 dark:bg-raised dark:text-muted">Conversación iniciada en {provider === "claude" ? "Claude Code" : "Codex"}. Se muestra en solo lectura; al continuarla en Dory, el agente retoma la misma sesión en su carpeta.</p>
          {view.data && <Messages threadId="" messages={view.data.messages} running={false} />}
        </div>
      </div>
    </div>
  );
}
