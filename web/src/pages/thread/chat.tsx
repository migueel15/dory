import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowRight, ArrowSquareOut, ArrowUp, ArrowsOut, Browser, Eye, EyeSlash, Key, Brain, CaretDown, CaretRight, Check, Copy, FileText, ListChecks, Question, Quotes, Stop, Wrench, X } from "@phosphor-icons/react";
import { api, type T3Attachment, type T3ThreadView, type T3ViewMessage } from "../../api/client";
import { copyText, ErrorText, Md } from "../../components/ui";
import { PendingRequest, type QuestionControl } from "../../components/pending-request";
import { SelectionMenu } from "../../components/selection-menu";
import { RefLink, ThreadMediaContext } from "../../components/refs";
import { ProviderIcon } from "../launch-form";
import { PdfThumb } from "../../components/pdf-thumb";
import { QUOTE_RE, quoteLabel, TokenChip, type Token, type Tokens } from "../../components/composer-tokens";
import { defaultUrlTransform } from "react-markdown";
import { Queue, iconBtn } from "./queue";
import { Composer } from "./composer";
import { SubagentBar } from "./subagent-bar";
import { ChangesCard } from "./changes";

// Insert adds text to the composer; with quote, a quote object goes before it.
export type Insert = { text: string; seq: number; quote?: { label: string; body: string } };
export function Chat({ v, queryKey, onStop, notice, beforeSend, insert }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void; notice?: React.ReactNode; beforeSend?: () => Promise<unknown>; insert?: Insert }) {
  const qc = useQueryClient();
  const threads = useQuery({ queryKey: ["t3-threads"], queryFn: api.t3Threads, refetchInterval: 60_000 });
  const everything = threads.data?.flatMap((x) => x.threads);
  const self = everything?.find((x) => x.id === v.id);
  const parent = self?.parent_id ? everything?.find((x) => x.id === self.parent_id) : undefined;
  const end = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const follow = () => {
    pinned.current = true;
    end.current?.scrollIntoView({ block: "end", behavior: "instant" });
  };
  const sending = async () => { await beforeSend?.(); follow(); };
  // Text sent to the composer: from the host view (insert) or from a selection.
  const [ins, setIns] = useState<Insert>();
  const push = (text: string, quote?: Insert["quote"]) => setIns((p) => ({ text, quote, seq: (p?.seq ?? 0) + 1 }));
  useEffect(() => { if (insert) push(insert.text, insert.quote); }, [insert?.seq]); // eslint-disable-line react-hooks/exhaustive-deps
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
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    pinned.current = distance < 40;
    setAway(distance > 200);
    if (el.scrollTop < 300) loadOlder();
  };
  const recent = new Set(v.messages.map((m) => m.id));
  // Only messages that arrive after opening the thread animate in; the loaded history stays still.
  const initial = useRef<Set<string> | null>(null);
  if (!initial.current) initial.current = new Set(recent);
  const seen = new Set<string>();
  // Pages overlap the latest turns: the live copy of an entry wins.
  const all = [...older.filter((m) => !recent.has(m.id) && !seen.has(m.id) && seen.add(m.id)), ...v.messages];
  const last = v.messages.at(-1);
  useEffect(() => {
    if (pinned.current) end.current?.scrollIntoView({ block: "end", behavior: "instant" });
  }, [v.messages, v.pending, v.queue, v.active_run_id]);
  const pendingSecret = [...all].reverse().find((m) => m.role === "secret" && m.status === "pending");
  const working = !!v.active_run_id && !last?.streaming && (last?.role === "user" || last?.role === "assistant" || !last);
  return (
    <ThreadMediaContext.Provider value={v.id}>
    <section className="relative flex min-h-0 flex-1 flex-col">
      <SelectionMenu box={scroller} scope="[data-message]" label="Usar la selección en el mensaje" actions={[
        { label: "Preguntar", icon: <Question className="size-3.5" />, run: (t) => push("Pregunta: ", { label: quoteLabel(t), body: t }) },
        { label: "Citar", icon: <Quotes className="size-3.5" />, run: (t) => push("", { label: quoteLabel(t), body: t }) },
      ]} />
      <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto py-3"><div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
        {v.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
        {nextCursor && (
          <div className="flex justify-center">
            <button type="button" onClick={loadOlder} disabled={loadingOlder} className="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
              {loadingOlder ? <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp weight="regular" className="size-3.5" />}
              {loadingOlder ? "Cargando mensajes anteriores…" : "Cargar mensajes anteriores"}
            </button>
          </div>
        )}
        {groups(all).map((m, gi, all) => Array.isArray(m) ? <WorkBlock key={m[0].id} steps={m} live={!!v.active_run_id && gi === all.length - 1} /> : m.role === "plan" ? <PlanCard key={m.id} m={m} /> : m.role === "handoff" ? <Handoff key={m.id} m={m} /> : m.role === "secret" ? <SecretLine key={m.id} m={m} /> : m.role === "changes" && m.files ? <ChangesCard key={m.id} m={m} /> : m.role === "html" && m.html ? <HtmlRender key={m.id} threadId={v.id} page={m.html} /> : m.role === "interrupt" ? (
          <div key={m.id} role="separator" className="flex items-center gap-3 text-[11px] font-medium uppercase tracking-wide text-slate-400">
            <span className="h-px flex-1 bg-slate-200 dark:bg-border" /><Stop weight="fill" className="size-3" />{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
          </div>
        ) : m.role === "user" ? (
          <div key={m.id} data-message className={`group flex flex-col items-end ${initial.current?.has(m.id) ? "" : "msg-in"}`}>
            {!!m.attachments?.length && <SentAttachments threadId={v.id} list={m.attachments} />}
            {(() => {
              // Tokens sent from the composer show inline as objects (tok: links the Md renders).
              const shown = tokenize(m);
              return <>
                {shown && <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent"><Md urlTransform={keepToken} components={tokenLinks}>{shown}</Md></div>}
              </>;
            })()}
            <MessageMeta m={m} />
          </div>
        ) : (
          <div key={m.id} data-message className={m.role === "system" ? "text-xs text-slate-500 dark:text-muted" : "group max-w-full pr-6"}>
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
          <button type="button" onClick={follow} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
            className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
            <ArrowDown weight="bold" className="size-3.5" />Ir al final
          </button>
        )}
      </div>
      <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
        {!v.pending && pendingSecret && (
          // A secret request slides out like a question, with its own input.
          <div className="notif-in mx-3 -mb-6 rounded-t-3xl border border-b-0 border-slate-200 bg-white px-4 pb-9 pt-3 shadow-md dark:border-border-strong dark:bg-surface">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-muted"><Key className="size-4" />El agente pide un secreto</p>
            <SecretRequest key={pendingSecret.id} threadId={v.id} m={pendingSecret} onDone={() => qc.invalidateQueries({ queryKey })} />
          </div>
        )}
        {v.pending && (
          // The question slides out from behind the composer, sharing its look.
          <div className="notif-in mx-3 -mb-6 max-h-[55dvh] overflow-y-auto rounded-t-3xl border border-b-0 border-slate-200 bg-white px-4 pb-9 pt-3 shadow-md dark:border-border-strong dark:bg-surface">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-muted"><Question className="size-4" />{v.pending.kind === "user_input" ? "El agente te pregunta" : "El agente pide permiso"}</p>
            <PendingRequest key={v.pending.id} threadId={v.id} pending={v.pending} onDone={() => qc.invalidateQueries({ queryKey })} onControl={onControl} />
          </div>
        )}
        {notice}
        {/* A subagent is driven by its parent and takes no messages: a bar
            with its run and a way back replaces the composer, as in T3 Code. */}
        <div className="relative z-10">{self?.relation === "subagent" ? <SubagentBar v={v} t={self} parent={parent} />
          : <Composer v={v} queryKey={queryKey} onStop={onStop} question={question} beforeSend={sending} insert={ins} />}</div>
      </div>
    </section>
    </ThreadMediaContext.Provider>
  );
}

export const fullDate = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
export const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

// MessageMeta shows, on hover, when a message was sent and a button to copy it.
export function MessageMeta({ m }: { m: T3ViewMessage }) {
  const [copied, setCopied] = useState(false);
  const at = m.created_at ? new Date(m.created_at) : undefined;
  const valid = at && !Number.isNaN(at.getTime());
  const copy = async () => {
    if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); }
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
export function PlanCard({ m }: { m: T3ViewMessage }) {
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
  const copy = async () => { if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); } };
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

export const isStep = (m: T3ViewMessage) => m.role === "reasoning" || m.role === "tool";

// groups folds each run of consecutive reasoning and tool steps into one block.
export function groups(ms: T3ViewMessage[]): (T3ViewMessage | T3ViewMessage[])[] {
  const out: (T3ViewMessage | T3ViewMessage[])[] = [];
  for (const m of ms) {
    const prev = out.at(-1);
    if (!isStep(m)) out.push(m);
    else if (Array.isArray(prev)) prev.push(m);
    else out.push([m]);
  }
  return out;
}

export const stepRunning = (m: T3ViewMessage) => m.streaming || (m.role === "tool" && !["completed", "failed", "error", "declined", "interrupted", "cancelled"].includes(m.status ?? ""));
export const stepFailed = (m: T3ViewMessage) => m.status === "failed" || m.status === "error";
export const stepLabel = (m: T3ViewMessage) => m.role === "reasoning" ? (m.streaming ? "Pensando…" : "Razonamiento") : m.title || "Herramienta";

export function StepIcon({ m }: { m: T3ViewMessage }) {
  if (stepFailed(m)) return <X weight="regular" className="size-3.5 shrink-0 text-red-500" />;
  if (stepRunning(m)) return <span className="size-3 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" />;
  return m.role === "reasoning" ? <Brain className="size-3.5 shrink-0" /> : <Wrench className="size-3.5 shrink-0" />;
}

// WorkBlock is one label that follows the model's current step; unfolded it
// lists every thought and tool of that stretch, each foldable on its own.
export function WorkBlock({ steps, live }: { steps: T3ViewMessage[]; live: boolean }) {
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
export function Step({ m }: { m: T3ViewMessage }) {
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

export const Dots = () => (
  <span className="inline-flex items-center gap-1 py-1" aria-label="Trabajando">
    {[0, 150, 300].map((d) => <span key={d} className="size-1.5 rounded-full bg-slate-400 motion-safe:animate-bounce" style={{ animationDelay: `${d}ms` }} />)}
  </span>
);

export type Shown = { src: string; name: string; mime: string };
export const isPdf = (f: { mime: string; name: string }) => f.mime === "application/pdf" || /\.pdf$/i.test(f.name);

// AttachmentPreview shows an image, PDF or text full size; anything else offers a download.
export function AttachmentPreview({ src, name, mime, onClose }: Shown & { onClose: () => void }) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [onClose]);
  const inline = mime === "application/pdf" || mime.startsWith("text/") || /\.html?$/i.test(name) || /\.(pdf|txt|md|log|json)$/i.test(name);
  return createPortal(
    <div role="dialog" aria-label={name} className="notif-in fixed inset-0 z-[120] flex flex-col bg-black/80 p-3 sm:p-6" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="mb-2 flex items-center gap-2 text-sm text-white">
        <span className="min-w-0 flex-1 truncate">{name}</span>
        <a href={src} download={name} className="rounded-md px-2 py-1 hover:bg-white/10">Descargar</a>
        <button type="button" onClick={onClose} aria-label="Cerrar" className="grid size-8 place-items-center rounded-md hover:bg-white/10"><X weight="regular" className="size-5" /></button>
      </div>
      <div className="flex min-h-0 flex-1 items-center justify-center" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
        {mime.startsWith("image/") ? <img src={src} alt={name} className="max-h-full max-w-full rounded-md object-contain shadow-2xl" />
          : inline ? <iframe src={src} title={name} sandbox={mime === "text/html" ? "allow-scripts allow-popups allow-popups-to-escape-sandbox" : undefined} className="size-full rounded-md bg-white" />
          : <p className="text-sm text-white/80">No hay vista previa para este tipo de fichero.</p>}
      </div>
    </div>,
    document.body,
  );
}

// SentAttachments shows the files sent with a user message; a click opens the preview.
export function SentAttachments({ threadId, list }: { threadId: string; list: T3Attachment[] }) {
  const [viewing, setViewing] = useState<Shown | null>(null);
  const url = (a: T3Attachment) => `/api/t3/threads/${encodeURIComponent(threadId)}/attachments/${encodeURIComponent(a.id)}?${new URLSearchParams({ name: a.name, mime: a.mime_type })}`;
  return (
    <div className="mb-1 flex max-w-[85%] flex-wrap justify-end gap-1.5">
      {list.map((a) => {
        const shown = { src: url(a), name: a.name, mime: a.mime_type };
        if (isPdf({ mime: a.mime_type, name: a.name })) return (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name} aria-label={`Ver ${a.name}`}
            className="w-36 cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 bg-white text-left dark:border-border dark:bg-surface">
            <PdfThumb src={shown.src} width={144} className="h-40 w-36 border-b border-slate-200 dark:border-border" />
            <span className="flex items-center gap-1.5 px-2 py-1.5 text-xs"><FileText className="size-3.5 shrink-0 text-red-600" /><span className="min-w-0 truncate">{a.name}</span></span>
          </button>
        );
        return a.type === "image" ? (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name} aria-label={`Ver ${a.name}`}
            className="cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 bg-slate-50 dark:border-border dark:bg-raised">
            <img src={shown.src} alt={a.name} loading="lazy" className="block max-h-48 max-w-[16rem] object-cover" />
          </button>
        ) : (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name}
            className="flex max-w-[16rem] items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 py-2 text-left text-sm hover:bg-slate-50 dark:border-border dark:bg-surface dark:hover:bg-raised">
            <FileText className="size-5 shrink-0 text-slate-500" /><span className="min-w-0 truncate">{a.name}</span>
          </button>
        );
      })}
      {viewing && <AttachmentPreview {...viewing} onClose={() => setViewing(null)} />}
    </div>
  );
}

// Handoff marks where the thread passed its context to another model.
export function Handoff({ m }: { m: T3ViewMessage }) {
  const chip = (model?: string, provider?: string) => (
    <span className="inline-flex max-w-[12rem] items-center gap-1.5 rounded-full border border-slate-200 bg-white px-2 py-0.5 normal-case tracking-normal text-slate-600 dark:border-border dark:bg-surface dark:text-text-secondary">
      <ProviderIcon driver={provider ?? ""} model={model} /><span className="truncate font-mono">{model || "—"}</span>
    </span>
  );
  return (
    <div role="separator" aria-label={`Cambio de modelo: de ${m.from || "otro modelo"} a ${m.to || "otro modelo"}`} title="El hilo pasó su contexto a otro modelo"
      className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide text-slate-400">
      <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
      {m.from && chip(m.from, m.from_provider)}
      <ArrowRight weight="bold" className="size-3.5 shrink-0" />
      {chip(m.to, m.to_provider)}
      <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
    </div>
  );
}

// HtmlRender shows a page the agent rendered with T3's html_render tool, as T3
// Code does: inline, sandboxed (its scripts run in an opaque origin), with a full view.
export function HtmlRender({ threadId, page }: { threadId: string; page: NonNullable<T3ViewMessage["html"]> }) {
  const [full, setFull] = useState(false);
  const name = `${page.title || "Página"}.html`;
  const src = `/api/t3/threads/${encodeURIComponent(threadId)}/attachments/${encodeURIComponent(page.attachment_id)}?${new URLSearchParams({ name, mime: "text/html" })}`;
  // The page reports its real height (the proxy adds a script with ?fit=1); the
  // agent's own height only holds until then. No inner scroll: the frame grows to fit.
  const frame = useRef<HTMLIFrameElement>(null);
  const [measured, setMeasured] = useState(0);
  useEffect(() => {
    const on = (e: MessageEvent) => {
      const h = (e.data as { doryFrameHeight?: unknown } | null)?.doryFrameHeight;
      if (e.source === frame.current?.contentWindow && typeof h === "number" && h > 0) setMeasured(Math.min(4000, Math.ceil(h)));
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, []);
  const height = measured || Math.min(1200, Math.max(80, Math.round(page.height || 400)));
  return (
    <figure className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-border dark:bg-surface">
      <figcaption className="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs dark:border-border">
        <Browser className="size-4 text-slate-400" /><span className="min-w-0 flex-1 truncate font-medium">{page.title || "Página"}</span>
        <button type="button" onClick={() => setFull(true)} title="Ver a pantalla completa" aria-label="Ver a pantalla completa" className={iconBtn}><ArrowsOut className="size-4" /></button>
        <a href={src} target="_blank" rel="noreferrer" title="Abrir en otra pestaña" aria-label="Abrir en otra pestaña" className={iconBtn}><ArrowSquareOut className="size-4" /></a>
      </figcaption>
      <iframe ref={frame} src={`${src}&fit=1`} title={page.title || "Página"} sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" loading="lazy" scrolling="no" style={{ height }} className="block w-full overflow-hidden bg-white" />
      {full && <AttachmentPreview src={src} name={name} mime="text/html" onClose={() => setFull(false)} />}
    </figure>
  );
}

export const secretStates: Record<string, string> = { pending: "pendiente", saved: "guardado", declined: "rechazado", cancelled: "cancelado" };

// SecretLine marks a secret request in the conversation, without its value.
export function SecretLine({ m }: { m: T3ViewMessage }) {
  const tone = m.status === "saved" ? "text-green-700 dark:text-green-400" : m.status === "pending" ? "text-amber-600 dark:text-amber-400" : "text-slate-400";
  return (
    <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-muted" title={m.text}>
      <Key className={`size-4 shrink-0 ${tone}`} />
      <span className="min-w-0 truncate">Secreto <span className="font-medium text-slate-700 dark:text-text-secondary">{m.title}</span></span>
      <span className={tone}>· {secretStates[m.status ?? ""] ?? m.status}</span>
    </div>
  );
}

// SecretRequest answers request_secret: the value goes to T3, which hands the
// agent a one-use reference; it never enters the chat or Dory's storage.
export function SecretRequest({ threadId, m, onDone }: { threadId: string; m: T3ViewMessage; onDone: () => void }) {
  const [value, setValue] = useState("");
  const [shown, setShown] = useState(false);
  const answer = useMutation({
    mutationFn: (decline: boolean) => api.t3AnswerSecret(threadId, m.id, decline ? { decline: true } : { secret: value }),
    onSuccess: () => { setValue(""); onDone(); },
  });
  const btn = "inline-flex h-8 items-center rounded-full px-4 text-sm font-medium transition disabled:opacity-50";
  return (
    <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (value.trim()) answer.mutate(false); }}>
      <div>
        <p className="text-sm font-medium text-slate-900 dark:text-white">{m.title}</p>
        {m.text && <p className="mt-0.5 text-sm text-slate-500 dark:text-muted">{m.text}</p>}
      </div>
      <div className="flex items-center gap-1 rounded-xl border border-slate-300 bg-white pr-1 focus-within:border-slate-500 dark:border-border-strong dark:bg-surface">
        <input autoFocus type={shown ? "text" : "password"} value={value} onChange={(e) => setValue(e.target.value)} placeholder={m.detail || "Pega el valor"}
          autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false} aria-label={m.title}
          className="min-h-10 w-full bg-transparent px-3 font-mono text-sm !outline-none placeholder:font-sans placeholder:text-slate-400" />
        <button type="button" onClick={() => setShown(!shown)} title={shown ? "Ocultar" : "Mostrar"} aria-label={shown ? "Ocultar" : "Mostrar"} className={iconBtn}>
          {shown ? <EyeSlash className="size-4" /> : <Eye className="size-4" />}
        </button>
      </div>
      {answer.error && <ErrorText error={answer.error} />}
      <div className="flex justify-end gap-2">
        <button type="button" disabled={answer.isPending} onClick={() => answer.mutate(true)} className={`${btn} text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised`}>Rechazar</button>
        <button type="submit" disabled={answer.isPending || !value.trim()} className={`${btn} bg-slate-900 text-white hover:bg-slate-700 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200`}>{answer.isPending ? "Enviando…" : "Guardar"}</button>
      </div>
    </form>
  );
}


// tokenize marks the tokens of a sent message as tok:kind:value links: its
// @files and $skills (from T3's context records), a leading /command and #PRs.
export const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
export function tokenize(m: T3ViewMessage): string {
  const link = (kind: string, value: string) => `[${kind}](tok:${kind}:${encodeURIComponent(value)})`;
  // Quotes sent from the composer come back as one object each.
  let t = m.text.replace(QUOTE_RE, (_, label: string, body: string) =>
    `\n\n${link("quote", JSON.stringify({ label, body: body.split("\n").map((l) => l.replace(/^> ?/, "")).join("\n").trim() }))}\n\n`);
  for (const p of m.mentions ?? []) t = t.split(`@${p}`).join(link("path", p));
  for (const k of m.skills ?? []) t = t.replace(new RegExp(`(^|\\s)\\$${escapeRe(k)}(?=\\s|$)`, "g"), (_, pre: string) => pre + link("skill", k));
  t = t.replace(/^\/([\w:.-]+)(?=\s|$)/, (_, c: string) => link("command", c));
  t = t.replace(/(^|\s)#(\d+)(?=\s|$|[.,;:])/g, (_, pre: string, n: string) => pre + link("ref", n));
  return t;
}
export const keepToken = (url: string) => (url.startsWith("tok:") || url.startsWith("file://") ? url : defaultUrlTransform(url));
export const tokenLinks = {
  a: ({ href, children, node: _n, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { node?: unknown }) => {
    if (!href?.startsWith("tok:")) return <RefLink href={href} {...rest}>{children}</RefLink>;
    const [, kind, raw] = href.split(":");
    const value = decodeURIComponent(raw ?? "");
    if (kind === "quote") {
      const q = (() => { try { return JSON.parse(value) as { label: string; body: string }; } catch { return { label: value, body: value }; } })();
      return <TokenChip kind="quote" label={q.label} title={q.body} />;
    }
    const label = kind === "path" ? value.replace(/\/+$/, "").split("/").pop() || value : value;
    return <TokenChip kind={kind as Token["kind"]} label={label} title={kind === "ref" ? `#${value}` : value} />;
  },
};
