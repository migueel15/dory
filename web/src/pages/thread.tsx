import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, CaretDown, Check, Lightning, PencilSimple, Question, Trash } from "@phosphor-icons/react";
import { api, type T3QueuedRun, type T3ThreadView } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, Input, Md, Notice, PageHeader, Select, StatusPill, pill } from "../components/ui";
import { OpenInT3 } from "../components/thread-link";
import { PendingRequest } from "../components/pending-request";
import { ProviderIcon } from "./launch-form";
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
    <div className="flex h-[calc(100dvh-7rem)] min-h-0 flex-col gap-3">
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
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [v.messages.length, last?.text.length, !!v.pending]);
  const working = !!v.active_run_id && !last?.streaming;
  return (
    <section className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto py-3"><div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
        {v.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
        {v.messages.map((m) => m.role === "user" ? (
          <div key={m.id} className="flex justify-end">
            <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-raised [&_*]:!text-inherit"><Md>{m.text}</Md></div>
          </div>
        ) : (
          <div key={m.id} className={m.role === "system" ? "text-xs text-slate-500 dark:text-muted" : "max-w-full pr-6"}>
            {m.text ? <Md>{m.text}</Md> : null}
            {m.streaming && <Dots />}
          </div>
        ))}
        {working && <Dots />}
        <div ref={end} />
      </div></div>
      <div className="mx-auto w-full max-w-3xl space-y-2 px-3 pt-2 sm:px-6">
        {v.pending && (
          <div className="rounded-2xl border border-amber-300 bg-amber-50 p-3 shadow-sm dark:border-amber-500/40 dark:bg-amber-500/10">
            <p className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-300"><Question weight="fill" className="size-4" />El agente te pregunta</p>
            <PendingRequest key={v.pending.id} threadId={v.id} pending={v.pending} onDone={() => qc.invalidateQueries({ queryKey })} />
          </div>
        )}
        <Queue v={v} queryKey={queryKey} />
        <Composer v={v} queryKey={queryKey} />
      </div>
    </section>
  );
}

const Dots = () => (
  <span className="inline-flex items-center gap-1 py-1" aria-label="Trabajando">
    {[0, 150, 300].map((d) => <span key={d} className="size-1.5 rounded-full bg-slate-400 motion-safe:animate-bounce" style={{ animationDelay: `${d}ms` }} />)}
  </span>
);

function Queue({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey });
  const act = useMutation({ mutationFn: (fn: () => Promise<void>) => fn(), onSuccess: done });
  if (!v.queue.length) return null;
  const q = v.queue;
  return (
    <div className="space-y-1 rounded-2xl border border-slate-200 bg-white p-2 dark:border-border dark:bg-surface">
      <p className="text-xs font-medium text-slate-500 dark:text-muted">En cola ({q.length}) · salen al terminar el turno</p>
      {q.map((r, i) => (
        <QueuedRow key={r.run_id} r={r} canSteer={!!v.active_run_id}
          onUp={i > 0 ? () => act.mutate(() => api.t3QueueReorder(v.id, r.run_id, q[i - 1].run_id)) : undefined}
          onDown={i < q.length - 1 ? () => act.mutate(() => api.t3QueueReorder(v.id, r.run_id, q[i + 2]?.run_id ?? "")) : undefined}
          onSteer={() => act.mutate(() => api.t3QueueSteer(v.id, r.run_id))}
          onCancel={() => act.mutate(() => api.t3QueueCancel(v.id, r.run_id))}
          onEdit={(text) => act.mutate(() => api.t3QueueEdit(v.id, r.run_id, text))} />
      ))}
      <ErrorText error={act.error} />
    </div>
  );
}

const iconBtn = "grid size-7 place-items-center rounded text-slate-500 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

function QueuedRow({ r, canSteer, onUp, onDown, onSteer, onCancel, onEdit }: { r: T3QueuedRun; canSteer: boolean; onUp?: () => void; onDown?: () => void; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  return (
    <div className="flex items-start gap-1 rounded-md bg-slate-50 p-1.5 text-sm dark:bg-surface">
      {draft === null ? <p className="min-w-0 flex-1 whitespace-pre-wrap break-words">{r.text}</p> : (
        <form className="flex flex-1 gap-1" onSubmit={(e) => { e.preventDefault(); onEdit(draft); setDraft(null); }}>
          <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setDraft(null); }} />
          <Button>Guardar</Button>
        </form>
      )}
      <button type="button" className={iconBtn} title="Subir" disabled={!onUp} onClick={onUp}><ArrowUp /></button>
      <button type="button" className={iconBtn} title="Bajar" disabled={!onDown} onClick={onDown}><ArrowDown /></button>
      <button type="button" className={iconBtn} title="Editar" onClick={() => setDraft(r.text)}><PencilSimple /></button>
      <button type="button" className={iconBtn} title="Steer: inyectar en el turno en curso" disabled={!canSteer} onClick={onSteer}><Lightning /></button>
      <button type="button" className={iconBtn} title="Quitar de la cola" onClick={onCancel}><Trash /></button>
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

function Composer({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const [text, setText] = useState("");
  const [steer, setSteer] = useState(false);
  const [choice, setChoice] = useState("");
  const [effort, setEffort] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const selected = choice || `${v.provider_id}|${v.model}`;
  const [providerId, slug] = selected.split("|");
  const model = catalog.data?.providers.find((p) => p.instance_id === providerId)?.models.find((m) => m.slug === slug);
  const effortValue = effort || (choice ? model?.effort?.default ?? "" : v.effort);
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
      text, mode: steer ? "steer" : "auto",
      ...(changed ? { provider_id: providerId, model: slug, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined } : {}),
    }),
    onSuccess: () => { setText(""); setSteer(false); qc.invalidateQueries({ queryKey }); },
  });
  const settings = useMutation({ mutationFn: (b: { interaction_mode?: string; runtime_mode?: string }) => api.t3Settings(v.id, b), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const busy = !!v.active_run_id;
  const models: MenuOption[] = (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} model={`${m.slug} ${m.name}`} /> })));
  if (!models.some((m) => m.value === selected)) models.unshift({ value: selected, label: v.model || "Modelo del hilo", icon: <ProviderIcon driver={v.provider_id} model={v.model} /> });
  const canSend = !!text.trim() && !send.isPending;
  const submit = () => { if (canSend) send.mutate(); };
  const action = busy ? (steer ? "Steer: se inyecta en el turno en curso" : "Se encolará al terminar el turno") : "";

  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(); }}
      className="rounded-3xl border border-slate-200 bg-white shadow-md transition-shadow focus-within:border-slate-400 focus-within:shadow-lg dark:border-border-strong dark:bg-surface dark:focus-within:border-muted">
      <textarea ref={area} rows={1} value={text} onChange={(e) => setText(e.target.value)} aria-label="Mensaje"
        placeholder={busy ? "El hilo está trabajando: tu mensaje se encolará…" : "Escribe al hilo…"}
        onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); } }}
        className="block max-h-60 w-full resize-none bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
      <div className="flex items-center gap-0.5 px-2 pb-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
          <Chip title="Modelo" value={selected} options={models} onChange={(x) => { setChoice(x); setEffort(""); }} />
          {model?.effort && <Chip title="Esfuerzo" value={effortValue} options={model.effort.values.map((x) => ({ value: x, label: x }))} onChange={setEffort} />}
          <Chip title="Modo" value={v.interaction_mode || "default"} options={interactionModes} onChange={(x) => settings.mutate({ interaction_mode: x })} />
          <Chip title="Permisos" value={v.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x })} />
          {busy && (
            <button type="button" aria-pressed={steer} onClick={() => setSteer(!steer)} title="Inyecta el mensaje en el turno en curso para corregir el rumbo"
              className={`inline-flex h-8 items-center gap-1 rounded-full px-2.5 text-xs font-medium transition-colors ${steer ? "bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300" : "text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"}`}>
              <Lightning weight={steer ? "fill" : "regular"} className="size-3.5" />Steer
            </button>
          )}
        </div>
        {action && <span className="hidden truncate px-2 text-xs text-slate-400 sm:inline">{action}</span>}
        <button type="submit" disabled={!canSend} title={busy ? (steer ? "Steer" : "Encolar") : "Enviar (Enter)"} aria-label="Enviar"
          className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
          {send.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp weight="bold" className="size-4" />}
        </button>
      </div>
      {(send.error || settings.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? settings.error} /></div>}
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
