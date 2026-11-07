import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Brain, CaretDown, CaretRight, Check, Copy, FileText, ListChecks, GitDiff, SidebarSimple, Paperclip, Lightning, ListBullets, PencilSimple, Play, Question, Stop, Trash, Wrench, X } from "@phosphor-icons/react";
import { api, type T3Attachment, type T3QueuedRun, type T3ThreadView, type T3ViewMessage } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, Input, Md, errMsg, uuid, Notice, PageHeader, Select, StatusPill, pill } from "../components/ui";
import { OpenInT3 } from "../components/thread-link";
import { PendingRequest, type QuestionControl } from "../components/pending-request";
import { ProviderIcon } from "./launch-form";
import { saveEffort, savedEffort } from "../effort";
import { Diff, type Suggestion } from "./github-diff";

// threadIdFrom accepts a T3 thread link or a bare id.
const threadIdFrom = (raw: string) => raw.trim().replace(/[?#].*$/, "").replace(/\/+$/, "").split("/").pop() ?? "";

// ThreadOpen asks for a T3 Code thread link and opens its view.
export function ThreadOpen() {
  const navigate = useNavigate();
  const [value, setValue] = useState("");
  const id = threadIdFrom(value);
  return (
    <div className="mx-auto max-w-xl space-y-4">
      <PageHeader title="Abrir hilo" subtitle="Pega el enlace o el id de un hilo de T3 Code." />
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (id) navigate({ to: "/threads/$threadId", params: { threadId: id } }); }}>
        <Input autoFocus value={value} onChange={(e) => setValue(e.target.value)} placeholder="https://…/threads/…" />
        <Button disabled={!id}>Abrir</Button>
      </form>
    </div>
  );
}

const runtimeModes = [
  { value: "approval-required", label: "Pedir permiso" },
  { value: "auto-accept-edits", label: "Aceptar ediciones" },
  { value: "auto", label: "Automático" },
  { value: "full-access", label: "Acceso total" },
];
const statusLabel: Record<string, string> = { preparing: "preparando", starting: "arrancando", running: "trabajando", waiting: "esperando respuesta" };

// ThreadView shows one T3 Code thread: conversation and composer on the left,
// changed files on the right.
export function ThreadView() {
  const { threadId } = useParams({ from: "/threads/$threadId" });
  const key = ["t3-view", threadId];
  const view = useQuery({ queryKey: key, queryFn: () => api.t3View(threadId) });
  useThreadLive(threadId, key);
  const [panel, setPanel] = useState<"" | "changes">("");
  const aside = useRef<HTMLElement>(null);
  const [opened, setOpened] = useState(false);
  useEffect(() => { if (panel) setOpened(true); }, [panel]);
  const [width, setWidth] = useState(() => { try { return Number(localStorage.getItem("dory.thread-panel-width")) || 560; } catch { return 560; } });
  // stopping is the run the user just stopped: the view shows it stopped
  // until T3 confirms, and goes back if T3 keeps it running.
  const [stopping, setStopping] = useState("");
  const running = view.data?.active_run_id ?? "";
  useEffect(() => {
    if (!stopping) return;
    if (running !== stopping) { setStopping(""); return; }
    const t = setTimeout(() => setStopping(""), 15_000);
    return () => clearTimeout(t);
  }, [stopping, running]);
  if (view.isPending) return <Notice loading />;
  if (view.error) return <ErrorText error={view.error} />;
  const v: T3ThreadView = stopping && view.data.active_run_id === stopping
    ? { ...view.data, active_run_id: "", active_status: "", interrupted: true }
    : view.data;
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] sm:h-[calc(100dvh-3rem-2px)] min-h-0 flex-col gap-3">
      <ContextBar v={v}>
        <button type="button" aria-pressed={panel === "changes"} onClick={() => setPanel(panel === "changes" ? "" : "changes")} title="Panel de cambios"
          className={`${pill} ml-auto ${panel === "changes" ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>
          <GitDiff className="size-4" />Cambios<SidebarSimple className="size-4 -scale-x-100" />
        </button>
      </ContextBar>
      <div className="relative flex min-h-0 flex-1">
        <div className={`min-h-0 min-w-0 flex-1 flex-col ${panel ? "hidden lg:flex" : "flex"}`}><Chat v={v} queryKey={key} onStop={setStopping} /></div>
        {/* Always mounted (its content only once opened): on desktop its width
            slides between 0 and the panel width, with the content pinned right
            so it enters from the right edge and leaves the same way. */}
        {(
          <aside ref={aside} aria-hidden={!panel}
            className={`min-h-0 flex-col overflow-hidden lg:flex lg:shrink-0 lg:transition-[width] lg:duration-300 lg:ease-out ${panel ? "flex w-full lg:w-[var(--panel-w)]" : "hidden lg:w-0"}`}
            style={{ "--panel-w": `${width}px` } as React.CSSProperties}>
            <div className="ml-auto flex min-h-0 w-full flex-1 lg:w-[var(--panel-w)]">
              <ResizeHandle panel={aside} width={width} onResize={setWidth} />
              <div className="flex min-h-0 min-w-0 flex-1 flex-col pb-4 lg:pl-3">{opened && <Changes v={v} queryKey={key} />}</div>
            </div>
          </aside>
        )}
      </div>
    </div>
  );
}

// ResizeHandle drags the side panel's left edge. While dragging it only moves
// a CSS variable on the panel, once per frame, so React does not re-render the
// chat and the diff on every pointer move; the width is saved on release.
function ResizeHandle({ panel, width, onResize }: { panel: React.RefObject<HTMLElement | null>; width: number; onResize: (w: number) => void }) {
  const start = (e: React.PointerEvent) => {
    e.preventDefault();
    const x0 = e.clientX, w0 = width;
    let last = w0, frame = 0;
    const move = (ev: PointerEvent) => {
      last = Math.min(Math.max(320, w0 + x0 - ev.clientX), window.innerWidth - 420);
      if (!frame) frame = requestAnimationFrame(() => { frame = 0; panel.current?.style.setProperty("--panel-w", `${last}px`); });
    };
    const up = () => {
      cancelAnimationFrame(frame);
      panel.current?.style.removeProperty("transition");
      onResize(last);
      window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up);
      document.body.style.cursor = ""; document.body.style.userSelect = "";
      try { localStorage.setItem("dory.thread-panel-width", String(Math.round(last))); } catch { /* per-browser only */ }
    };
    document.body.style.cursor = "col-resize"; document.body.style.userSelect = "none";
    panel.current?.style.setProperty("transition", "none");
    window.addEventListener("pointermove", move); window.addEventListener("pointerup", up);
  };
  return (
    <div role="separator" aria-orientation="vertical" aria-label="Redimensionar panel" onPointerDown={start}
      className="group hidden w-2 shrink-0 cursor-col-resize items-stretch justify-center lg:flex">
      <span className="w-px bg-slate-200 transition-colors group-hover:w-0.5 group-hover:bg-slate-400 dark:bg-border dark:group-hover:bg-muted" />
    </div>
  );
}

function ContextBar({ v, children }: { v: T3ThreadView; children?: React.ReactNode }) {
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  const t = t3.data?.flatMap((x) => x.threads).find((x) => x.id === v.id);
  return (
    <header className="flex flex-wrap items-center gap-2">
      <h1 className="min-w-0 truncate text-lg font-semibold">{v.title || v.id}</h1>
      {t?.url && <OpenInT3 url={t.url} environmentId={t.t3_environment_id} threadId={v.id} className="mr-1" />}
      {v.branch && <span className="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs dark:bg-raised">{v.branch}</span>}
      {v.active_run_id ? <StatusPill tone="blue">{statusLabel[v.active_status] ?? v.active_status}</StatusPill> : <StatusPill>en reposo</StatusPill>}
      {v.pull_requests.map((pr) => (
        <a key={pr.url} href={pr.url} target="_blank" rel="noreferrer" className={pill}>PR #{pr.number} · {pr.repository}</a>
      ))}
      {v.items.map((it) => (
        <Link key={it.item_id} to="/items/$id/define" params={{ id: it.item_id }} className={pill}>Item {it.item_id.slice(0, 8)}</Link>
      ))}
      {v.pending && <StatusPill tone="amber">Espera tu respuesta</StatusPill>}
      {children}
    </header>
  );
}

function Chat({ v, queryKey, onStop }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void }) {
  const qc = useQueryClient();
  const end = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // While a question is open, text sent from the composer answers it.
  const [question, setQuestion] = useState<QuestionControl | null>(null);
  const onControl = (c: QuestionControl | null) => setQuestion((p) => (p && c && p.picked === c.picked && p.last === c.last && p.busy === c.busy ? (p.next = c.next, p) : c));
  const [away, setAway] = useState(false);
  // T3 sends only the latest turns: older pages load when the top comes near,
  // keeping what is on screen on the same pixel.
  const [older, setOlder] = useState<T3ViewMessage[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const anchor = useRef<number | null>(null);
  const nextCursor = cursor ?? v.history_cursor;
  const loadOlder = () => {
    if (!nextCursor || loadingOlder) return;
    setLoadingOlder(true);
    anchor.current = scroller.current ? scroller.current.scrollHeight - scroller.current.scrollTop : null;
    api.t3History(v.id, nextCursor).then(
      (p) => { setOlder((o) => [...p.messages, ...o]); setCursor(p.has_more ? p.next_cursor : ""); },
      () => { anchor.current = null; },
    ).finally(() => setLoadingOlder(false));
  };
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && anchor.current !== null) { el.scrollTop = el.scrollHeight - anchor.current; anchor.current = null; }
  }, [older]);
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    setAway(el.scrollHeight - el.scrollTop - el.clientHeight > 200);
    if (el.scrollTop < 300) loadOlder();
  };
  const recent = new Set(v.messages.map((m) => m.id));
  const seen = new Set<string>();
  // Pages overlap the latest turns: the live copy of an entry wins.
  const all = [...older.filter((m) => !recent.has(m.id) && !seen.has(m.id) && seen.add(m.id)), ...v.messages];
  const last = v.messages.at(-1);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [v.messages.length, last?.text.length, !!v.pending, v.queue.length]);
  const working = !!v.active_run_id && !last?.streaming && (last?.role === "user" || last?.role === "assistant" || !last);
  return (
    <section className="relative flex min-h-0 flex-1 flex-col">
      <div ref={scroller} onScroll={onScroll} className="thin-scroll min-h-0 flex-1 overflow-y-auto py-3"><div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
        {v.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
        {nextCursor && (
          <div className="flex justify-center">
            <button type="button" onClick={loadOlder} disabled={loadingOlder} className="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
              {loadingOlder ? <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp weight="regular" className="size-3.5" />}
              {loadingOlder ? "Cargando mensajes anteriores…" : "Cargar mensajes anteriores"}
            </button>
          </div>
        )}
        {groups(all).map((m, gi, all) => Array.isArray(m) ? <WorkBlock key={m[0].id} steps={m} live={!!v.active_run_id && gi === all.length - 1} /> : m.role === "plan" ? <PlanCard key={m.id} m={m} /> : m.role === "interrupt" ? (
          <div key={m.id} role="separator" className="flex items-center gap-3 text-[11px] font-medium uppercase tracking-wide text-slate-400">
            <span className="h-px flex-1 bg-slate-200 dark:bg-border" /><Stop weight="fill" className="size-3" />{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
          </div>
        ) : m.role === "user" ? (
          <div key={m.id} className="group flex flex-col items-end">
            <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent"><Md>{m.text}</Md></div>
            <MessageMeta m={m} />
          </div>
        ) : (
          <div key={m.id} className={m.role === "system" ? "text-xs text-slate-500 dark:text-muted" : "group max-w-full pr-6"}>
            {m.text ? <Md>{m.text}</Md> : null}
            {m.streaming ? <Dots /> : m.role === "assistant" && <MessageMeta m={m} />}
          </div>
        ))}
        {working && <Dots />}
        <Queue v={v} queryKey={queryKey} />
        <div ref={end} />
      </div></div>
      <div className="relative h-0">
        {/* Always mounted so it can slide up on show and down on hide. */}
        {(
          <button type="button" onClick={() => end.current?.scrollIntoView({ block: "end", behavior: "smooth" })} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
            className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
            <ArrowDown weight="bold" className="size-3.5" />Ir al final
          </button>
        )}
      </div>
      <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
        {v.pending && (
          // The question slides out from behind the composer, sharing its look.
          <div className="notif-in mx-3 -mb-6 max-h-[55dvh] overflow-y-auto rounded-t-3xl border border-b-0 border-slate-200 bg-white px-4 pb-9 pt-3 shadow-md dark:border-border-strong dark:bg-surface">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-muted"><Question className="size-4" />{v.pending.kind === "user_input" ? "El agente te pregunta" : "El agente pide permiso"}</p>
            <PendingRequest key={v.pending.id} threadId={v.id} pending={v.pending} onDone={() => qc.invalidateQueries({ queryKey })} onControl={onControl} />
          </div>
        )}
        <div className="relative z-10"><Composer v={v} queryKey={queryKey} onStop={onStop} question={question} /></div>
      </div>
    </section>
  );
}

const fullDate = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

// MessageMeta shows, on hover, when a message was sent and a button to copy it.
function MessageMeta({ m }: { m: T3ViewMessage }) {
  const [copied, setCopied] = useState(false);
  const at = m.created_at ? new Date(m.created_at) : undefined;
  const valid = at && !Number.isNaN(at.getTime());
  const copy = async () => {
    try { await navigator.clipboard.writeText(m.text); setCopied(true); setTimeout(() => setCopied(false), 1500); } catch { /* clipboard unavailable */ }
  };
  return (
    <div className="mt-1 flex items-center gap-1 text-[11px] text-slate-400 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-within:opacity-100">
      {valid && <time dateTime={m.created_at} title={fullDate.format(at)} className="cursor-default tabular-nums">{shortTime.format(at)}</time>}
      <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar mensaje"} aria-label="Copiar mensaje" className={iconBtn}>
        {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
    </div>
  );
}

// PlanCard shows a plan the model proposed as a readable markdown document,
// folded to a preview when long.
function PlanCard({ m }: { m: T3ViewMessage }) {
  const [open, setOpen] = useState(false);
  const card = useRef<HTMLElement>(null);
  const bottom = useRef<number | null>(null);
  // Folding keeps what follows the card on the same pixel: the chat scrolls
  // by exactly what the card shrank.
  useLayoutEffect(() => {
    const before = bottom.current;
    bottom.current = null;
    const el = card.current;
    if (before === null || !el) return;
    let box: HTMLElement | null = el.parentElement;
    while (box && !/(auto|scroll)/.test(getComputedStyle(box).overflowY)) box = box.parentElement;
    if (box) box.scrollTop += el.getBoundingClientRect().bottom - before;
  }, [open]);
  const toggle = () => { bottom.current = open ? card.current?.getBoundingClientRect().bottom ?? null : null; setOpen(!open); };
  const [copied, setCopied] = useState(false);
  const long = m.text.length > 1200;
  const title = m.text.match(/^#\s+(.+)$/m)?.[1];
  const copy = async () => { try { await navigator.clipboard.writeText(m.text); setCopied(true); setTimeout(() => setCopied(false), 1500); } catch { /* clipboard unavailable */ } };
  return (
    <article ref={card} className="group overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-border dark:bg-surface">
      <header className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-4 py-2.5 dark:border-border dark:bg-raised">
        <ListChecks className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-muted">Plan propuesto</span>
        {m.streaming && <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent text-slate-400" />}
        <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar plan en markdown"} aria-label="Copiar plan" className={`ml-auto ${iconBtn}`}>
          {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
        </button>
      </header>
      <div className={`relative px-5 py-4 ${long && !open ? "max-h-96 overflow-hidden" : ""}`}>
        <Md>{m.text || "…"}</Md>
        {long && !open && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-white dark:from-surface" />}
      </div>
      {long && (
        <button type="button" onClick={toggle} className="flex w-full items-center justify-center gap-1 border-t border-slate-200 py-2 text-xs font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900 dark:border-border dark:text-muted dark:hover:bg-raised dark:hover:text-white">
          {open ? <>Contraer <CaretDown weight="regular" className="size-3 rotate-180" /></> : <>Ver plan completo{title ? ` · ${title}` : ""} <CaretDown weight="regular" className="size-3" /></>}
        </button>
      )}
    </article>
  );
}

const isStep = (m: T3ViewMessage) => m.role === "reasoning" || m.role === "tool";

// groups folds each run of consecutive reasoning and tool steps into one block.
function groups(ms: T3ViewMessage[]): (T3ViewMessage | T3ViewMessage[])[] {
  const out: (T3ViewMessage | T3ViewMessage[])[] = [];
  for (const m of ms) {
    const prev = out.at(-1);
    if (!isStep(m)) out.push(m);
    else if (Array.isArray(prev)) prev.push(m);
    else out.push([m]);
  }
  return out;
}

const stepRunning = (m: T3ViewMessage) => m.streaming || (m.role === "tool" && !["completed", "failed", "error", "declined", "interrupted", "cancelled"].includes(m.status ?? ""));
const stepFailed = (m: T3ViewMessage) => m.status === "failed" || m.status === "error";
const stepLabel = (m: T3ViewMessage) => m.role === "reasoning" ? (m.streaming ? "Pensando…" : "Razonamiento") : m.title || "Herramienta";

function StepIcon({ m }: { m: T3ViewMessage }) {
  if (stepFailed(m)) return <X weight="regular" className="size-3.5 shrink-0 text-red-500" />;
  if (stepRunning(m)) return <span className="size-3 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" />;
  return m.role === "reasoning" ? <Brain className="size-3.5 shrink-0" /> : <Wrench className="size-3.5 shrink-0" />;
}

// WorkBlock is one label that follows the model's current step; unfolded it
// lists every thought and tool of that stretch, each foldable on its own.
function WorkBlock({ steps, live }: { steps: T3ViewMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false);
  const last = steps[steps.length - 1];
  const running = live || steps.some(stepRunning);
  const tools = steps.filter((m) => m.role === "tool").length;
  const summary = running ? stepLabel(last) : `${steps.length} pasos${tools ? ` · ${tools} herramientas` : ""}`;
  const preview = running && (last.role === "tool" ? last.detail : last.text.split("\n").filter(Boolean).at(-1));
  return (
    <div className="text-sm">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex max-w-full items-center gap-2 rounded-lg py-0.5 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
        {running ? <StepIcon m={{ ...last, streaming: true }} /> : <Check weight="regular" className="size-3.5 shrink-0" />}
        <span className="shrink-0 font-medium">{summary}</span>
        {preview && <span className="min-w-0 truncate font-mono opacity-70">{preview}</span>}
        {open ? <CaretDown weight="regular" className="size-3 shrink-0" /> : <CaretRight weight="regular" className="size-3 shrink-0" />}
      </button>
      {open && (
        <div className="mt-1 space-y-1 border-l-2 border-slate-200 pl-3 dark:border-border">
          {steps.map((m) => <Step key={m.id} m={m} />)}
        </div>
      )}
    </div>
  );
}

// Step is one thought or tool call inside a WorkBlock; it unfolds its text or input.
function Step({ m }: { m: T3ViewMessage }) {
  const body = m.role === "reasoning" ? m.text : m.detail ?? "";
  const [open, setOpen] = useState<boolean | null>(null);
  const shown = open ?? (m.role === "reasoning" && m.streaming);
  return (
    <div className="text-xs text-slate-500 dark:text-muted">
      <button type="button" disabled={!body} onClick={() => setOpen(!shown)} className="flex max-w-full items-center gap-2 py-0.5 hover:text-slate-900 disabled:hover:text-inherit dark:hover:text-white">
        <StepIcon m={m} />
        <span className="shrink-0 font-medium text-slate-700 dark:text-text-secondary">{stepLabel(m)}</span>
        {!shown && m.role === "tool" && body && <code className="min-w-0 truncate font-mono">{body}</code>}
        {body && (shown ? <CaretDown weight="regular" className="size-3 shrink-0" /> : <CaretRight weight="regular" className="size-3 shrink-0" />)}
      </button>
      {shown && body && (m.role === "reasoning"
        ? <div className="pb-1 pl-5 text-slate-500 dark:text-muted [&_*]:!text-inherit"><Md>{body}</Md></div>
        : <pre className="mb-1 ml-5 max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-100 p-2 font-mono text-[11px] dark:bg-raised">{body}</pre>)}
    </div>
  );
}

const Dots = () => (
  <span className="inline-flex items-center gap-1 py-1" aria-label="Trabajando">
    {[0, 150, 300].map((d) => <span key={d} className="size-1.5 rounded-full bg-slate-400 motion-safe:animate-bounce" style={{ animationDelay: `${d}ms` }} />)}
  </span>
);

// Queue shows the messages waiting for the active turn as outlined bubbles
// at the end of the conversation, each with its actions underneath.
function Queue({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey });
  const act = useMutation({ mutationFn: (fn: () => Promise<void>) => fn(), onSuccess: done });
  if (!v.queue.length) return null;
  const q = v.queue;
  return (
    <section aria-label="Mensajes en cola" className="space-y-3 border-t border-dashed border-slate-300 pt-3 dark:border-border">
      {q.map((r) => (
        <QueuedRow key={r.run_id} r={r} canSteer={!!v.active_run_id}
          onSteer={() => act.mutate(() => api.t3QueueSteer(v.id, r.run_id))}
          onCancel={() => act.mutate(() => api.t3QueueCancel(v.id, r.run_id))}
          onEdit={(text) => act.mutate(() => api.t3QueueEdit(v.id, r.run_id, text))} />
      ))}
      <ErrorText error={act.error} />
    </section>
  );
}

const iconBtn = "grid size-7 place-items-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

function QueuedRow({ r, canSteer, onSteer, onCancel, onEdit }: { r: T3QueuedRun; canSteer: boolean; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <p className="whitespace-pre-wrap break-words text-sm">{r.text}</p> : (
          <form className="flex gap-1" onSubmit={(e) => { e.preventDefault(); onEdit(draft); setDraft(null); }}>
            <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setDraft(null); }} />
            <Button>Guardar</Button>
          </form>
        )}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={iconBtn} title="Mandar ya (steer: inyectar en el turno en curso)" disabled={!canSteer} onClick={onSteer}><Lightning /></button>
        <button type="button" className={iconBtn} title="Editar" onClick={() => setDraft(r.text)}><PencilSimple /></button>
        <button type="button" className={iconBtn} title="Eliminar de la cola" onClick={onCancel}><Trash /></button>
      </div>
    </div>
  );
}

type MenuOption = { value: string; label: string; group?: string; icon?: React.ReactNode };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
function Chip({ value, options, onChange, title, icon }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);
  const current = options.find((o) => o.value === value);
  return (
    <div ref={box} className="relative">
      <button type="button" title={title} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[11rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon ?? icon}<span className="truncate">{current?.label ?? value}</span><CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className="absolute bottom-full left-0 z-20 mb-2 max-h-72 min-w-[12rem] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
          {options.map((o, i) => (
            <li key={o.value}>
              {o.group && o.group !== options[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); setOpen(false); }}
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === value && <Check weight="regular" className="size-3.5" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ModelPicker joins model and effort in one chip; its panel lists models on
// the left and, on the right, the efforts of the model last clicked. Picking a
// model preselects the effort last used with it.
function ModelPicker({ models, selected, effort, effortsOf, onChange }: { models: MenuOption[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void }) {
  const [open, setOpen] = useState(false);
  const [focus, setFocus] = useState(selected);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    setFocus(selected);
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open, selected]);
  const current = models.find((m) => m.value === selected);
  const efforts = effortsOf(focus);
  const pickModel = (value: string) => {
    const e = effortsOf(value);
    setFocus(value);
    onChange(value, e?.initial ?? "");
    if (!e) setOpen(false);
  };
  const row = "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div ref={box} className="relative">
      <button type="button" title="Modelo y esfuerzo" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[15rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon}<span className="truncate">{current?.label ?? selected}</span>
        {effort && <span className="shrink-0 text-slate-400">· {effort}</span>}<CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div role="dialog" aria-label="Modelo y esfuerzo" className="absolute bottom-full left-0 z-20 mb-2 flex max-h-80 w-[min(30rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <ul className="min-w-0 flex-1 overflow-y-auto p-1">
            {models.map((o, i) => (
              <li key={o.value}>
                {o.group && o.group !== models[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
                <button type="button" onClick={() => pickModel(o.value)}
                  className={`${row} ${o.value === focus ? "bg-slate-100 dark:bg-raised" : ""}`}>
                  {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === selected && <Check weight="regular" className="size-3.5" />}
                </button>
              </li>
            ))}
          </ul>
          <div className="w-32 shrink-0 overflow-y-auto border-l border-slate-200 p-1 dark:border-border">
            <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Esfuerzo</p>
            {efforts ? efforts.values.map((x) => {
              const active = focus === selected ? x === effort : x === efforts.initial;
              return (
                <button key={x} type="button" onClick={() => { onChange(focus, x); setOpen(false); }} className={`${row} ${active ? "font-medium" : ""}`}>
                  <span className="min-w-0 flex-1 truncate">{x}</span>{active && <Check weight="regular" className="size-3.5" />}
                </button>
              );
            }) : <p className="px-2 py-1.5 text-xs text-slate-400">Sin opciones</p>}
          </div>
        </div>
      )}
    </div>
  );
}

type Draft = { key: string; name: string; preview?: string; done?: T3Attachment; error?: string };

function Composer({ v, queryKey, onStop, question }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void; question: QuestionControl | null }) {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const [text, setText] = useState("");
  const [steer, setSteer] = useState(false);
  const [choice, setChoice] = useState("");
  const [effort, setEffort] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<Draft[]>([]);
  const uploading = files.some((f) => !f.done && !f.error);
  const ready = files.flatMap((f) => (f.done ? [f.done] : []));
  // Each file uploads as soon as it is added; the message carries the results.
  const addFiles = (list: FileList | File[]) => {
    for (const file of Array.from(list)) {
      const key = uuid(); // crypto.randomUUID needs HTTPS, the dev preview is plain HTTP
      const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
      const preview = named.type.startsWith("image/") ? URL.createObjectURL(named) : undefined;
      setFiles((fs) => [...fs, { key, name: named.name, preview }]);
      api.t3Upload(v.id, named).then(
        (done) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, done } : f))),
        (e: unknown) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, error: errMsg(e) || "No se pudo subir" } : f))),
      );
    }
  };
  const removeFile = (key: string) => setFiles((fs) => fs.filter((f) => { if (f.key === key && f.preview) URL.revokeObjectURL(f.preview); return f.key !== key; }));
  const selected = choice || `${v.provider_id}|${v.model}`;
  const [providerId, slug] = selected.split("|");
  const model = catalog.data?.providers.find((p) => p.instance_id === providerId)?.models.find((m) => m.slug === slug);
  const effortValue = effort || (choice ? (model?.effort ? savedEffort(model.slug, model.effort.values) || model.effort.default : "") : v.effort);
  const changed = !!choice || !!effort;

  // Grow the textarea with its content, up to about ten lines.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 240)}px`;
  }, [text]);

  const send = useMutation({
    mutationFn: () => api.t3Dispatch(v.id, {
      text, mode: steer ? "steer" : "auto", ...(ready.length ? { attachments: ready } : {}),
      ...(changed ? { provider_id: providerId, model: slug, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined } : {}),
    }),
    onSuccess: () => { if (changed && model?.effort && effortValue) saveEffort(model.slug, effortValue); setText(""); setSteer(false); files.forEach((f) => f.preview && URL.revokeObjectURL(f.preview)); setFiles([]); qc.invalidateQueries({ queryKey }); },
  });
  const settings = useMutation({ mutationFn: (b: { interaction_mode?: string; runtime_mode?: string }) => api.t3Settings(v.id, b), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const busy = !!v.active_run_id;
  const models: MenuOption[] = (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} model={`${m.slug} ${m.name}`} /> })));
  if (!models.some((m) => m.value === selected)) models.unshift({ value: selected, label: v.model || "Modelo del hilo", icon: <ProviderIcon driver={v.provider_id} model={v.model} /> });
  // Optimistic: the thread shows stopped at once; an error brings the run back.
  const stop = useMutation({
    mutationFn: () => api.t3Interrupt(v.id),
    onMutate: () => onStop(v.active_run_id),
    onError: () => onStop(""),
    onSettled: () => qc.invalidateQueries({ queryKey }),
  });
  const held = !busy && v.queue.some((r) => r.held);
  // Continue picks up where the stopped turn left off: the held queue first, else a nudge.
  const resume = useMutation({
    mutationFn: () => held ? api.t3QueueResume(v.id) : api.t3Dispatch(v.id, { text: "Continúa por donde lo dejaste.", mode: "auto" }),
    onSuccess: () => qc.invalidateQueries({ queryKey }),
  });
  // Stopped with an empty box: the send button continues where it stopped.
  const resumable = !busy && (held || v.interrupted) && !text.trim() && !files.length;
  const canSend = (!!text.trim() || ready.length > 0 || resumable) && !uploading && !send.isPending && !resume.isPending;
  const canNext = !!question && !question.busy && (!!text.trim() || question.picked);
  const submit = () => {
    // An open question takes the typed text as its free answer, or its checked options.
    if (question) { if (canNext) { question.next(text.trim()); setText(""); } return; }
    if (!canSend) return;
    if (resumable) resume.mutate(); else send.mutate();
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(); }}
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
      onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
      onPaste={(e) => {
        // Screenshots often come only as clipboard items, not in .files.
        const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
        if (fs.length) { e.preventDefault(); addFiles(fs); }
      }}
      className="rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2 px-3 pt-3">
          {files.map((f) => (
            <div key={f.key} title={f.error ?? f.name} className={`relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
              {f.preview ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
              {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[9px] font-medium text-white">Error</span>}
              {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
              <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white hover:bg-slate-900"><X weight="regular" className="size-3" /></button>
            </div>
          ))}
        </div>
      )}
      <textarea ref={area} rows={1} value={text} onChange={(e) => setText(e.target.value)} aria-label="Mensaje"
        placeholder={question ? "Escribe tu respuesta a la pregunta…" : "Escribe al hilo…"}
        onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); } }}
        className="block max-h-60 w-full resize-none bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
      <div className="flex items-center gap-0.5 px-2 pb-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
          <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
          <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
            className="grid size-8 place-items-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
            <Paperclip weight="regular" className="size-4" />
          </button>
          <ModelPicker models={models} selected={selected} effort={effortValue}
            effortsOf={(value) => { const [pid, sl] = value.split("|"); const m = catalog.data?.providers.find((p) => p.instance_id === pid)?.models.find((x) => x.slug === sl); return m?.effort ? { values: m.effort.values, initial: value === selected ? effortValue : savedEffort(sl, m.effort.values) || m.effort.default } : undefined; }}
            onChange={(value, eff) => { setChoice(value); setEffort(eff); const sl = value.split("|")[1]; if (eff) saveEffort(sl, eff); }} />
          <Chip title="Permisos" value={v.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x })} />
          {busy && (
            <button type="button" aria-pressed={steer} onClick={() => setSteer(!steer)} title="Inyecta el mensaje en el turno en curso para corregir el rumbo"
              className={`inline-flex h-8 items-center gap-1 rounded-full px-2.5 text-xs font-medium transition-colors ${steer ? "bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300" : "text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"}`}>
              <Lightning weight={steer ? "fill" : "regular"} className="size-3.5" />Steer
            </button>
          )}
        </div>
        {question ? (
          <>
            {busy && (
              <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
                className="grid size-9 shrink-0 place-items-center rounded-full border border-slate-200 text-slate-700 transition hover:bg-slate-100 disabled:opacity-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised">
                <Stop weight="fill" className="size-3.5" />
              </button>
            )}
            <button type="submit" disabled={!canNext} title={question.last ? "Responder (Enter)" : "Siguiente (Enter)"}
              className="ml-1 inline-flex h-9 shrink-0 items-center gap-1 rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
              {question.last ? "Responder" : "Siguiente"}<CaretRight weight="regular" className="size-3.5" />
            </button>
          </>
        ) : busy && !text.trim() ? (
          <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
            className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
            <Stop weight="fill" className="size-3.5" />
          </button>
        ) : <button type="submit" disabled={!canSend} title={resumable ? "Continuar por donde iba" : busy ? (steer ? "Steer" : "Encolar") : "Enviar (Enter)"} aria-label={resumable ? "Continuar" : "Enviar"}
          className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 p-0 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
          {send.isPending || resume.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : resumable ? <Play weight="fill" className="block size-3.5 translate-x-px" /> : busy && !steer ? <ListBullets weight="bold" className="block size-4" /> : <ArrowUp weight="bold" className="block size-4" />}
        </button>}
      </div>
      {(send.error || settings.error || stop.error || resume.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? settings.error ?? stop.error ?? resume.error} /></div>}
    </form>
  );
}

// prOf reads owner/repo and number from a pull request link.
const prOf = (url: string) => {
  const m = url.match(/github\.com\/([^/]+\/[^/]+)\/pull\/(\d+)/);
  return m ? { repo: m[1], number: Number(m[2]) } : undefined;
};

// Changes shows the thread's own changes or its pull request, and sends a
// selected range to the chat through the same queue or steer as the composer.
function Changes({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const prs = v.pull_requests.map((p) => ({ ...p, parsed: prOf(p.url) })).filter((p) => p.parsed);
  const [source, setSource] = useState("thread");
  const pr = prs.find((p) => p.url === source)?.parsed;
  const local = useQuery({ queryKey: ["t3-diff", v.id, v.messages.length], queryFn: () => api.t3Diff(v.id), enabled: !pr });
  const prDiff = useQuery({ queryKey: ["github-diff", pr?.repo, pr?.number], queryFn: () => api.githubDiff(pr!.repo, pr!.number), enabled: !!pr });
  const text = pr ? prDiff.data?.diff : local.data?.diff;
  const suggest = async (s: Suggestion) => {
    const where = s.start === s.end ? `línea ${s.start}` : `líneas ${s.start}–${s.end}`;
    const body = `Sugerencia sobre \`${s.path}\` (${where}):\n\n\`\`\`diff\n${s.snippet}\n\`\`\`\n\n${s.question}`;
    await api.t3Dispatch(v.id, { text: body, mode: s.steer ? "steer" : "auto" });
    qc.invalidateQueries({ queryKey });
  };
  return (
    <section className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <Select className="w-64" value={source} onChange={setSource} options={[
          { value: "thread", label: "Cambios del hilo (worktree)" },
          ...prs.map((p) => ({ value: p.url, label: `PR #${p.number} · ${p.repository}` })),
        ]} />
        {v.worktree_path && <span className="truncate font-mono text-xs text-slate-500 dark:text-muted" title={v.worktree_path}>{v.worktree_path}</span>}
      </div>
      <ErrorText error={pr ? prDiff.error : local.error} />
      {text === undefined ? <Notice loading /> : text === "" ? <p className="text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>
        : <div className="min-h-0 flex-1"><Diff key={source} text={text} onSuggest={suggest} fill /></div>}
    </section>
  );
}
