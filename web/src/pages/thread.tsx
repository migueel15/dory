import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Lightning, PaperPlaneRight, PencilSimple, Trash } from "@phosphor-icons/react";
import { api, type T3QueuedRun, type T3ThreadView } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, Input, Md, Notice, PageHeader, Select, StatusPill, Textarea, pill } from "../components/ui";
import { ProviderIcon } from "./launch-form";

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
  if (view.isPending) return <Notice loading />;
  if (view.error) return <ErrorText error={view.error} />;
  const v = view.data;
  return (
    <div className="flex h-[calc(100dvh-7rem)] min-h-0 flex-col gap-3">
      <ContextBar v={v} />
      <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-2">
        <Chat v={v} queryKey={key} />
        <section className="hidden min-h-0 rounded-lg border border-dashed border-slate-300 p-4 text-sm text-slate-500 lg:block dark:border-border dark:text-muted">
          Los archivos cambiados llegarán en la siguiente entrega.
        </section>
      </div>
    </div>
  );
}

function ContextBar({ v }: { v: T3ThreadView }) {
  return (
    <header className="flex flex-wrap items-center gap-2">
      <h1 className="mr-2 min-w-0 truncate text-lg font-semibold">{v.title || v.id}</h1>
      {v.branch && <span className="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs dark:bg-raised">{v.branch}</span>}
      {v.active_run_id ? <StatusPill tone="blue">{statusLabel[v.active_status] ?? v.active_status}</StatusPill> : <StatusPill>en reposo</StatusPill>}
      {v.pull_requests.map((pr) => (
        <a key={pr.url} href={pr.url} target="_blank" rel="noreferrer" className={pill}>PR #{pr.number} · {pr.repository}</a>
      ))}
      {v.items.map((it) => (
        <Link key={it.item_id} to="/items/$id/define" params={{ id: it.item_id }} className={pill}>Item {it.item_id.slice(0, 8)}</Link>
      ))}
      {v.pending && <StatusPill tone="amber">Petición pendiente: respóndela en Atención</StatusPill>}
    </header>
  );
}

function Chat({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const end = useRef<HTMLDivElement>(null);
  const last = v.messages.at(-1);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [v.messages.length, last?.text.length]);
  return (
    <section className="flex min-h-0 flex-col rounded-lg border border-slate-200 dark:border-border">
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
        {v.messages.length === 0 && <p className="text-sm text-slate-500">Sin mensajes todavía.</p>}
        {v.messages.map((m) => (
          <div key={m.id} className={m.role === "user" ? "ml-8 rounded-lg bg-slate-100 p-2 dark:bg-raised" : "mr-4"}>
            <Md>{m.text || (m.streaming ? "…" : "")}</Md>
          </div>
        ))}
        <div ref={end} />
      </div>
      <Queue v={v} queryKey={queryKey} />
      <Composer v={v} queryKey={queryKey} />
    </section>
  );
}

function Queue({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey });
  const act = useMutation({ mutationFn: (fn: () => Promise<void>) => fn(), onSuccess: done });
  if (!v.queue.length) return null;
  const q = v.queue;
  return (
    <div className="space-y-1 border-t border-slate-200 p-2 dark:border-border">
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

function Composer({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const [text, setText] = useState("");
  const [steer, setSteer] = useState(false);
  const [choice, setChoice] = useState("");
  const [effort, setEffort] = useState("");
  const selected = choice || `${v.provider_id}|${v.model}`;
  const [providerId, slug] = selected.split("|");
  const model = catalog.data?.providers.find((p) => p.instance_id === providerId)?.models.find((m) => m.slug === slug);
  const effortValue = effort || (choice ? model?.effort?.default ?? "" : v.effort);
  const changed = !!choice || !!effort;

  const send = useMutation({
    mutationFn: () => api.t3Dispatch(v.id, {
      text, mode: steer ? "steer" : "auto",
      ...(changed ? { provider_id: providerId, model: slug, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined } : {}),
    }),
    onSuccess: () => { setText(""); setSteer(false); qc.invalidateQueries({ queryKey }); },
  });
  const settings = useMutation({ mutationFn: (b: { interaction_mode?: string; runtime_mode?: string }) => api.t3Settings(v.id, b), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const busy = !!v.active_run_id;
  const models = (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} /> })));
  if (!models.some((m) => m.value === selected)) models.unshift({ value: selected, label: v.model || "Modelo del hilo", group: "", icon: <ProviderIcon driver={v.provider_id} /> });

  return (
    <form className="space-y-2 border-t border-slate-200 p-2 dark:border-border" onSubmit={(e) => { e.preventDefault(); if (text.trim()) send.mutate(); }}>
      <Textarea bare rows={3} value={text} onChange={(e) => setText(e.target.value)} placeholder={busy ? "El hilo está trabajando: el mensaje se encola (o márcalo como steer)" : "Escribe al hilo…"}
        onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && text.trim()) { e.preventDefault(); send.mutate(); } }} />
      <div className="flex flex-wrap items-center gap-2">
        <Select className="w-48" value={selected} onChange={(x) => { setChoice(x); setEffort(""); }} options={models} />
        {model?.effort && <Select className="w-28" value={effortValue} onChange={setEffort} options={model.effort.values.map((x) => ({ value: x, label: x }))} />}
        <Select className="w-24" value={v.interaction_mode || "default"} onChange={(x) => settings.mutate({ interaction_mode: x })} options={interactionModes} />
        <Select className="w-40" value={v.runtime_mode} onChange={(x) => settings.mutate({ runtime_mode: x })} options={runtimeModes} />
        <label className="flex items-center gap-1 text-xs" title="Inyecta el mensaje en el turno en curso para corregir el rumbo">
          <input type="checkbox" checked={steer} onChange={(e) => setSteer(e.target.checked)} />Steer
        </label>
        <Button className="ml-auto" disabled={!text.trim() || send.isPending}>
          <PaperPlaneRight aria-hidden="true" className="size-4" />{busy ? (steer ? "Steer" : "Encolar") : "Enviar"}
        </Button>
      </div>
      <ErrorText error={send.error ?? settings.error} />
    </form>
  );
}
