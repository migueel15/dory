import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowRight, ChatsCircle, Kanban, MagnifyingGlass, Plus } from "@phosphor-icons/react";
import { api, type Item } from "../api/client";
import { Button, ErrorText, Modal, PageHeader } from "../components/ui";
import { useLaunchForm } from "./launch-form";
import { ThreadStatus } from "../components/thread-status";

type Row = {
  id: string; title: string; status: string; attention: boolean; model: string; updated: string;
  env: string; item?: Item; project?: string;
};
type Tab = "all" | "attention" | "running" | "items";
const tabs: { id: Tab; label: string }[] = [
  { id: "all", label: "Todos" }, { id: "attention", label: "Te necesitan" }, { id: "running", label: "Trabajando" }, { id: "items", label: "Con tarea" },
];

// threadIdFrom accepts a T3 Code thread link or a bare id.
export const threadIdFrom = (value: string) => {
  const v = value.trim();
  const m = v.match(/\/threads\/([^/?#]+)/);
  if (m) return decodeURIComponent(m[1]);
  return /^[\w-]{8,}$/.test(v) ? v : "";
};

const ago = (iso: string) => {
  const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000);
  if (!Number.isFinite(s)) return "";
  if (s < 60) return "ahora";
  if (s < 3600) return `${Math.floor(s / 60)} min`;
  if (s < 86400) return `${Math.floor(s / 3600)} h`;
  return `${Math.floor(s / 86400)} d`;
};
const running = (s: string) => ["running", "working", "starting", "preparing"].includes(s.toLowerCase());

// Threads lists the T3 Code threads Dory knows (live ones and those linked to
// items), so a thread opens with one click; a pasted link or id still works.
export function Threads() {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<Tab>("all");
  const [creating, setCreating] = useState(false);
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 15_000 });
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 60_000 });
  const items = useQuery({ queryKey: ["items", "all"], queryFn: () => api.items.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list(), staleTime: 5 * 60_000 });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });

  const rows = useMemo(() => {
    const itemById = new Map((items.data ?? []).map((i) => [i.id, i]));
    const projectName = new Map((projects.data ?? []).map((p) => [p.id, p.name]));
    const envName = new Map((envs.data ?? []).map((e) => [e.id, e.name]));
    const itemOf = new Map<string, Item>();
    for (const l of links.data ?? []) {
      const it = itemById.get(l.item_id);
      if (it && !itemOf.has(l.thread_id)) itemOf.set(l.thread_id, it);
    }
    const out = new Map<string, Row>();
    for (const inst of t3.data ?? []) {
      for (const t of inst.threads) {
        const item = itemOf.get(t.id);
        out.set(t.id, {
          id: t.id, title: t.title || item?.title || t.id, status: t.status, attention: t.attention, model: t.model, updated: t.updated_at,
          env: envName.get(inst.environment_id) ?? "", item, project: item?.project_id ? projectName.get(item.project_id) : undefined,
        });
      }
    }
    // Threads settled long ago drop out of T3's live list; their items still lead to them.
    for (const [tid, item] of itemOf) {
      if (out.has(tid) || item.archived_at || item.status === "done" || item.status === "cancelled") continue;
      out.set(tid, { id: tid, title: item.title, status: "", attention: false, model: "", updated: item.updated_at, env: "", item, project: item.project_id ? projectName.get(item.project_id) : undefined });
    }
    return [...out.values()].sort((a, b) => Number(b.attention) - Number(a.attention) || Number(running(b.status)) - Number(running(a.status)) || b.updated.localeCompare(a.updated));
  }, [t3.data, links.data, items.data, envs.data, projects.data]);

  const pasted = threadIdFrom(query);
  const q = query.trim().toLowerCase();
  const shown = rows.filter((r) =>
    (tab === "all" || (tab === "attention" && r.attention) || (tab === "running" && running(r.status)) || (tab === "items" && !!r.item)) &&
    (!q || pasted === r.id || [r.title, r.item?.title, r.project, r.model, r.env, r.id].some((x) => x?.toLowerCase().includes(q))));
  const count = (t: Tab) => t === "all" ? rows.length : rows.filter((r) => t === "attention" ? r.attention : t === "running" ? running(r.status) : !!r.item).length;
  const open = (id: string) => navigate({ to: "/threads/$threadId", params: { threadId: id } });

  return (
    <div className="mx-auto max-w-4xl space-y-4">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1"><PageHeader title="Hilos" subtitle="Tus hilos de T3 Code y las tareas que trabajan en ellos." /></div>
        <Button onClick={() => setCreating(true)} className="inline-flex shrink-0 items-center gap-1.5"><Plus weight="bold" className="size-4" />Nuevo hilo</Button>
      </div>
      {creating && <NewThread onClose={() => setCreating(false)} onCreated={open} />}
      <form onSubmit={(e) => { e.preventDefault(); const id = shown[0]?.id ?? pasted; if (id) open(id); }}
        className="flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 shadow-sm focus-within:border-slate-400 dark:border-border dark:bg-surface dark:focus-within:border-muted">
        <MagnifyingGlass className="size-4 shrink-0 text-slate-400" />
        <input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar por título, tarea o proyecto… o pega un enlace de T3"
          aria-label="Buscar hilos" className="min-h-11 w-full bg-transparent text-sm !outline-none placeholder:text-slate-400" />
      </form>
      <div role="tablist" className="flex flex-wrap gap-1">
        {tabs.map((t) => (
          <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} onClick={() => setTab(t.id)}
            className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm transition-colors ${tab === t.id ? "bg-slate-900 text-white dark:bg-white dark:text-slate-900" : "text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"}`}>
            {t.label}<span className="text-xs tabular-nums opacity-60">{count(t.id)}</span>
          </button>
        ))}
      </div>
      {pasted && !rows.some((r) => r.id === pasted) && (
        <button type="button" onClick={() => open(pasted)} className="flex w-full items-center gap-3 rounded-xl border border-dashed border-slate-300 px-4 py-3 text-left text-sm hover:bg-slate-50 dark:border-border-strong dark:hover:bg-raised">
          <ChatsCircle className="size-5 text-slate-400" /><span className="flex-1">Abrir el hilo <span className="font-mono text-xs">{pasted}</span></span><ArrowRight className="size-4" />
        </button>
      )}
      {t3.isPending ? <p className="py-10 text-center text-sm text-slate-400">Cargando hilos…</p> : shown.length === 0 ? (
        <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">{q ? "Ningún hilo coincide." : "No hay hilos aquí."}</p>
      ) : (
        <ul className="divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200 bg-white dark:divide-border/60 dark:border-border dark:bg-surface">
          {shown.map((r) => (
            <li key={r.id}>
              <Link to="/threads/$threadId" params={{ threadId: r.id }} className="group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-slate-50 dark:hover:bg-raised">
                <span className="w-5 shrink-0">{r.status ? <ThreadStatus status={r.status} attention={r.attention} /> : <ChatsCircle className="size-4 text-slate-300 dark:text-muted" />}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{r.title}</span>
                  <span className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-slate-500 dark:text-muted">
                    {r.item && <span className="inline-flex min-w-0 items-center gap-1"><Kanban className="size-3.5 shrink-0" /><span className="max-w-[18rem] truncate">{r.item.title}</span></span>}
                    {r.project && <span>{r.project}</span>}
                    {r.env && <span>{r.env}</span>}
                    {r.model && <span className="font-mono">{r.model}</span>}
                  </span>
                </span>
                {r.updated && <span className="shrink-0 text-xs tabular-nums text-slate-400">{ago(r.updated)}</span>}
                <ArrowRight className="size-4 shrink-0 text-slate-300 transition-transform group-hover:translate-x-0.5 group-hover:text-slate-500" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// NewThread starts a free thread for a quick question, with no item; it can be
// linked to an item later. It opens the thread as soon as T3 creates it.
function NewThread({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const qc = useQueryClient();
  const form = useLaunchForm({ section: "quick" });
  const [text, setText] = useState("");
  const create = useMutation({
    mutationFn: () => api.t3CreateThread({ ...form.value, text: text.trim() }),
    onSuccess: ({ thread_id }) => { form.remember(); qc.invalidateQueries({ queryKey: ["t3-status"] }); onClose(); onCreated(thread_id); },
  });
  const ready = form.ready && !create.isPending;
  return (
    <Modal title="Nuevo hilo" onClose={onClose}>
      <p className="text-sm text-slate-500 dark:text-muted">Un chat sin tarea; podrás asociarle una más adelante. El primer mensaje es opcional: sin él se abre el chat vacío.</p>
      <textarea autoFocus value={text} onChange={(e) => setText(e.target.value)} rows={4} placeholder="Primer mensaje (opcional)" aria-label="Primer mensaje"
        onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && ready) { e.preventDefault(); create.mutate(); } }}
        className="block w-full resize-y rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm dark:border-border-strong dark:bg-surface" />
      {form.fields}
      <ErrorText error={form.error ?? create.error} />
      <Button disabled={!ready} onClick={() => create.mutate()}>{create.isPending ? "Creando…" : text.trim() ? "Crear y enviar" : "Abrir chat vacío"}</Button>
    </Modal>
  );
}
