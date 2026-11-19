import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { hasWindowClose } from "../lib/window-close";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Circle, CircleNotch, GitDiff, Kanban, WarningCircle, XCircle } from "@phosphor-icons/react";
import { api } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, pill, uuid } from "../components/ui";
import { Messages, ModelChangeDivider, ToEnd, useFollow } from "./chat";
import { Chip, Composer, Queue } from "./composer";
import { QuoteSelection } from "../components/quote-selection";
import { ProviderIcon } from "../components/launch-picker";
import { Diff, type Suggestion } from "./github-diff";
import { setDiffBase, useDiffBase } from "../lib/diff-base";
import { addDraftQuote, removeDraftQuotes, useDraftQuotes } from "../lib/draft-quotes";
import { ThreadInfo } from "../components/thread-info";
import { useThreadMenu } from "../components/thread-menu";
import { useThreadsFold } from "../lib/threads-fold";
import { SidePanel, SidePanelToggle, useSidePanel, type SidePanelState } from "../components/side-panel";

// ThreadView is a whole thread page: header, chat and, on demand, its changes.
export function ThreadView() {
  const { threadId } = useParams({ strict: false }) as { threadId: string };
  const panel = useSidePanel(threadId);
  const local = useThread(threadId).data?.local;
  return (
    <div className="relative flex h-[calc(100dvh-4rem-1px)] min-h-0 sm:h-[calc(100dvh-3rem-2px)]">
      {/* The panel toggle is pinned to the page corner, so opening the panel never moves it. */}
      <div className={`absolute right-1 top-0 z-30 sm:-top-2 ${hasWindowClose ? "sm:right-8" : "sm:-right-1"}`}><SidePanelToggle panel={panel} /></div>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Header threadId={threadId} panel={panel} />
        <ThreadChat threadId={threadId} />
      </div>
      <SidePanel panel={panel} available={local ? ["diff"] : []} render={(k) => k === "diff" && <Changes threadId={threadId} />} />
    </div>
  );
}

function useThread(threadId?: string) {
  useThreadLive(threadId);
  return useQuery({ queryKey: ["thread", threadId], queryFn: () => api.thread(threadId!), enabled: !!threadId, staleTime: 5_000 });
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
  useEffect(() => { if (t && !t.read_at && t.status !== "working") act.mutate({ action: "read" }); }, [t?.id, t?.status]); // eslint-disable-line react-hooks/exhaustive-deps
  const fold = useThreadsFold();
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
      {t.task_id && <Link to="/tasks" search={{ task: t.task_id } as never} className={`${pill} !px-2`} title="Ver la tarea"><Kanban className="size-4" /></Link>}
      <ThreadInfo t={t} popup={panel.open} diff={panel.open && panel.active === "diff"} toggleDiff={() => (panel.open && panel.active === "diff" ? panel.toggle() : panel.show("diff"))} />
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
  if (view.error) return <div className="p-4"><ErrorText error={view.error} /></div>;
  return (
    <section className="relative flex min-h-0 flex-1 flex-col">
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

function Changes({ threadId }: { threadId: string }) {
  const base = useDiffBase(threadId);
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
        {diff.data.base && <Chip below title="Contra qué se comparan los cambios" icon={<GitDiff className="size-4 shrink-0" />} value={diff.data.base} onChange={(v) => setDiffBase(threadId, v)}
          options={[{ value: "HEAD", label: "Sin commitear" }, ...[...new Set([...(diff.data.branches ?? []), ...(diff.data.base !== "HEAD" ? [diff.data.base] : [])])].map((b) => ({ value: b, label: `Contra ${b}` }))]} />}
        <span ref={setTools} className="flex items-center gap-1.5" />
        <span className="ml-auto min-w-0 truncate font-mono">{diff.data.branch}</span>
      </div>}
      <ErrorText error={diff.error} />
      {diff.data && (diff.data.diff ? <Diff text={diff.data.diff} fill toolsHost={tools} onSuggest={comment} steerable={false} /> : <p className="p-4 text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>)}
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
