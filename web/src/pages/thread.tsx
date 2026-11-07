import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowUp, Brain, CaretDown, CaretRight, Check, Copy, FileText, Paperclip, Lightning, ListBullets, PencilSimple, Play, Question, Stop, Trash, Wrench, X } from "@phosphor-icons/react";
import { api, type T3Attachment, type T3QueuedRun, type T3ThreadView, type T3ViewMessage } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, Input, Md, errMsg, uuid, Notice, PageHeader, Select, StatusPill, pill } from "../components/ui";
import { OpenInT3 } from "../components/thread-link";
import { PendingRequest } from "../components/pending-request";
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

const interactionModes = [{ value: "default", label: "Chat" }, { value: "plan", label: "Plan" }];
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
  const [tab, setTab] = useState<"chat" | "changes">("chat");
  if (view.isPending) return <Notice loading />;
  if (view.error) return <ErrorText error={view.error} />;
  const v = view.data;
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] sm:h-[calc(100dvh-3rem-2px)] min-h-0 flex-col gap-3">
      <ContextBar v={v} />
      <div className="flex gap-1 lg:hidden" role="tablist">
        {(["chat", "changes"] as const).map((t) => (
          <button key={t} type="button" role="tab" aria-selected={tab === t} onClick={() => setTab(t)} className={`${pill} ${tab === t ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>{t === "chat" ? "Chat" : "Cambios"}</button>
        ))}
      </div>
      <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-2">
        <div className={`min-h-0 flex-col ${tab === "chat" ? "flex" : "hidden lg:flex"}`}><Chat v={v} queryKey={key} /></div>
        <div className={`min-h-0 overflow-auto ${tab === "changes" ? "block" : "hidden lg:block"}`}><Changes v={v} queryKey={key} /></div>
      </div>
    </div>
  );
}

function ContextBar({ v }: { v: T3ThreadView }) {
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
    </header>
  );
}

function Chat({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const end = useRef<HTMLDivElement>(null);
  const last = v.messages.at(-1);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [v.messages.length, last?.text.length, !!v.pending, v.queue.length]);
  const working = !!v.active_run_id && !last?.streaming && (last?.role === "user" || last?.role === "assistant" || !last);
  return (
    <section className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto py-3"><div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
        {v.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
        {groups(v.messages).map((m, gi, all) => Array.isArray(m) ? <WorkBlock key={m[0].id} steps={m} live={!!v.active_run_id && gi === all.length - 1} /> : m.role === "interrupt" ? (
          <div key={m.id} role="separator" className="flex items-center gap-3 text-[11px] font-medium uppercase tracking-wide text-slate-400">
            <span className="h-px flex-1 bg-slate-200 dark:bg-border" /><Stop weight="fill" className="size-3" />{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
          </div>
        ) : m.role === "user" ? (
          <div key={m.id} className="group flex flex-col items-end">
            <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-raised [&_*]:!text-inherit"><Md>{m.text}</Md></div>
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
      <div className="mx-auto w-full max-w-3xl space-y-2 px-3 pb-4 pt-2 sm:px-6">
        {v.pending && (
          <div className="rounded-2xl border border-amber-300 bg-amber-50 p-3 shadow-sm dark:border-amber-500/40 dark:bg-amber-500/10">
            <p className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-300"><Question weight="fill" className="size-4" />El agente te pregunta</p>
            <PendingRequest key={v.pending.id} threadId={v.id} pending={v.pending} onDone={() => qc.invalidateQueries({ queryKey })} />
          </div>
        )}
        <Composer v={v} queryKey={queryKey} />
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
        {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
    </div>
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
  if (stepFailed(m)) return <X className="size-3.5 shrink-0 text-red-500" />;
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
        {running ? <StepIcon m={{ ...last, streaming: true }} /> : <Check className="size-3.5 shrink-0" />}
        <span className="shrink-0 font-medium">{summary}</span>
        {preview && <span className="min-w-0 truncate font-mono opacity-70">{preview}</span>}
        {open ? <CaretDown className="size-3 shrink-0" /> : <CaretRight className="size-3 shrink-0" />}
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
        {body && (shown ? <CaretDown className="size-3 shrink-0" /> : <CaretRight className="size-3 shrink-0" />)}
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
        {current?.icon ?? icon}<span className="truncate">{current?.label ?? value}</span><CaretDown className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className="absolute bottom-full left-0 z-20 mb-2 max-h-72 min-w-[12rem] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
          {options.map((o, i) => (
            <li key={o.value}>
              {o.group && o.group !== options[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); setOpen(false); }}
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === value && <Check className="size-3.5" />}
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
        {effort && <span className="shrink-0 text-slate-400">· {effort}</span>}<CaretDown className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div role="dialog" aria-label="Modelo y esfuerzo" className="absolute bottom-full left-0 z-20 mb-2 flex max-h-80 w-[min(30rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <ul className="min-w-0 flex-1 overflow-y-auto p-1">
            {models.map((o, i) => (
              <li key={o.value}>
                {o.group && o.group !== models[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
                <button type="button" onClick={() => pickModel(o.value)}
                  className={`${row} ${o.value === focus ? "bg-slate-100 dark:bg-raised" : ""}`}>
                  {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === selected && <Check className="size-3.5" />}
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
                  <span className="min-w-0 flex-1 truncate">{x}</span>{active && <Check className="size-3.5" />}
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

function Composer({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
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
  const stop = useMutation({ mutationFn: () => api.t3Interrupt(v.id), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const held = !busy && v.queue.some((r) => r.held);
  // Continue picks up where the stopped turn left off: the held queue first, else a nudge.
  const resume = useMutation({
    mutationFn: () => held ? api.t3QueueResume(v.id) : api.t3Dispatch(v.id, { text: "Continúa por donde lo dejaste.", mode: "auto" }),
    onSuccess: () => qc.invalidateQueries({ queryKey }),
  });
  // Stopped with an empty box: the send button continues where it stopped.
  const resumable = !busy && (held || v.interrupted) && !text.trim() && !files.length;
  const canSend = (!!text.trim() || ready.length > 0 || resumable) && !uploading && !send.isPending && !resume.isPending;
  const submit = () => { if (!canSend) return; if (resumable) resume.mutate(); else send.mutate(); };

  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(); }}
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
      onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
      onPaste={(e) => {
        // Screenshots often come only as clipboard items, not in .files.
        const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
        if (fs.length) { e.preventDefault(); addFiles(fs); }
      }}
      className="rounded-3xl border border-slate-200 bg-white shadow-md transition-shadow focus-within:border-slate-400 focus-within:shadow-lg dark:border-border-strong dark:bg-surface dark:focus-within:border-muted">
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2 px-3 pt-3">
          {files.map((f) => (
            <div key={f.key} title={f.error ?? f.name} className={`relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
              {f.preview ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
              {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[9px] font-medium text-white">Error</span>}
              {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
              <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white hover:bg-slate-900"><X className="size-3" /></button>
            </div>
          ))}
        </div>
      )}
      <textarea ref={area} rows={1} value={text} onChange={(e) => setText(e.target.value)} aria-label="Mensaje"
        placeholder="Escribe al hilo…"
        onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); } }}
        className="block max-h-60 w-full resize-none bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
      <div className="flex items-center gap-0.5 px-2 pb-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
          <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
          <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
            className="grid size-8 place-items-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
            <Paperclip className="size-4" />
          </button>
          <ModelPicker models={models} selected={selected} effort={effortValue}
            effortsOf={(value) => { const [pid, sl] = value.split("|"); const m = catalog.data?.providers.find((p) => p.instance_id === pid)?.models.find((x) => x.slug === sl); return m?.effort ? { values: m.effort.values, initial: value === selected ? effortValue : savedEffort(sl, m.effort.values) || m.effort.default } : undefined; }}
            onChange={(value, eff) => { setChoice(value); setEffort(eff); const sl = value.split("|")[1]; if (eff) saveEffort(sl, eff); }} />
          <Chip title="Modo" value={v.interaction_mode || "default"} options={interactionModes} onChange={(x) => settings.mutate({ interaction_mode: x })} />
          <Chip title="Permisos" value={v.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x })} />
          {busy && (
            <button type="button" aria-pressed={steer} onClick={() => setSteer(!steer)} title="Inyecta el mensaje en el turno en curso para corregir el rumbo"
              className={`inline-flex h-8 items-center gap-1 rounded-full px-2.5 text-xs font-medium transition-colors ${steer ? "bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300" : "text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"}`}>
              <Lightning weight={steer ? "fill" : "regular"} className="size-3.5" />Steer
            </button>
          )}
        </div>
        {busy && !text.trim() ? (
          <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
            className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
            <Stop weight="fill" className="size-3.5" />
          </button>
        ) : <button type="submit" disabled={!canSend} title={resumable ? "Continuar por donde iba" : busy ? (steer ? "Steer" : "Encolar") : "Enviar (Enter)"} aria-label={resumable ? "Continuar" : "Enviar"}
          className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
          {send.isPending || resume.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : resumable ? <Play weight="fill" className="size-3.5" /> : busy && !steer ? <ListBullets weight="bold" className="size-4" /> : <ArrowUp weight="bold" className="size-4" />}
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
    <section className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Select className="w-64" value={source} onChange={setSource} options={[
          { value: "thread", label: "Cambios del hilo (worktree)" },
          ...prs.map((p) => ({ value: p.url, label: `PR #${p.number} · ${p.repository}` })),
        ]} />
        {v.worktree_path && <span className="truncate font-mono text-xs text-slate-500 dark:text-muted" title={v.worktree_path}>{v.worktree_path}</span>}
      </div>
      <ErrorText error={pr ? prDiff.error : local.error} />
      {text === undefined ? <Notice loading /> : text === "" ? <p className="text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>
        : <Diff key={source} text={text} onSuggest={suggest} />}
    </section>
  );
}
