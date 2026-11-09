import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowRight, ArrowUp, ChatsCircle, MagnifyingGlass, PencilSimpleLine, SidebarSimple } from "@phosphor-icons/react";
import { api, type Item } from "../api/client";
import { ErrorText } from "../components/ui";
import { useLaunchForm } from "./launch-form";
import { useProject } from "../project";
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

// useThreadRows lists the T3 Code threads Dory knows: live ones and those
// linked to items, the ones needing the user first, then the working ones.
function useThreadRows() {
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
  return { rows, loading: t3.isPending };
}

// Threads is the threads section, laid out like a chat app: the thread list on
// the left (foldable on desktop) and, in the middle, the welcome composer or
// the open thread.
export function Threads() {
  const { threadId } = useParams({ strict: false }) as { threadId?: string };
  const [folded, setFolded] = useState(() => { try { return localStorage.getItem("dory.threads-list") === "folded"; } catch { return false; } });
  const fold = (v: boolean) => { setFolded(v); try { localStorage.setItem("dory.threads-list", v ? "folded" : "open"); } catch { /* per-browser only */ } };
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] min-h-0 gap-3 sm:h-[calc(100dvh-3rem-2px)]">
      <aside aria-label="Hilos" className={`min-h-0 flex-col overflow-hidden transition-[width] duration-300 ease-out lg:flex lg:shrink-0 ${folded ? "lg:w-0" : "lg:w-72"} hidden`}>
        <div className="flex min-h-0 w-72 flex-1 flex-col"><ThreadList active={threadId} onFold={() => fold(true)} /></div>
      </aside>
      <div className={`relative flex min-h-0 min-w-0 flex-1 flex-col ${folded ? "lg:pl-10" : ""}`}>
        {folded && (
          <button type="button" onClick={() => fold(false)} title="Mostrar la lista de hilos" aria-label="Mostrar la lista de hilos"
            className="absolute left-0 top-0 z-20 hidden size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 lg:grid dark:text-muted dark:hover:bg-raised dark:hover:text-white"><SidebarSimple className="size-4" /></button>
        )}
        <Outlet />
      </div>
    </div>
  );
}

// ThreadList is the side list: new thread, search (or a pasted T3 link), filters and threads.
function ThreadList({ active, onFold }: { active?: string; onFold?: () => void }) {
  const navigate = useNavigate();
  const { rows, loading } = useThreadRows();
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<Tab>("all");
  const pasted = threadIdFrom(query);
  const q = query.trim().toLowerCase();
  const shown = rows.filter((r) =>
    (tab === "all" || (tab === "attention" && r.attention) || (tab === "running" && running(r.status)) || (tab === "items" && !!r.item)) &&
    (!q || pasted === r.id || [r.title, r.item?.title, r.project, r.model, r.env, r.id].some((x) => x?.toLowerCase().includes(q))));
  const count = (t: Tab) => t === "all" ? rows.length : rows.filter((r) => t === "attention" ? r.attention : t === "running" ? running(r.status) : !!r.item).length;
  const open = (id: string) => navigate({ to: "/threads/$threadId", params: { threadId: id } });
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex items-center gap-1">
        <Link to="/threads" className="inline-flex flex-1 items-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium hover:bg-slate-200 dark:hover:bg-raised"><PencilSimpleLine className="size-4" />Nuevo hilo</Link>
        {onFold && <button type="button" onClick={onFold} title="Ocultar la lista" aria-label="Ocultar la lista de hilos" className="hidden size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 lg:grid dark:text-muted dark:hover:bg-raised dark:hover:text-white"><SidebarSimple className="size-4" /></button>}
      </div>
      <form onSubmit={(e) => { e.preventDefault(); const id = shown[0]?.id ?? pasted; if (id) open(id); }}
        className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-2 focus-within:border-slate-400 dark:border-border dark:bg-surface dark:focus-within:border-muted">
        <MagnifyingGlass className="size-4 shrink-0 text-slate-400" />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar o pegar enlace de T3"
          aria-label="Buscar hilos" className="min-h-9 w-full bg-transparent text-sm !outline-none placeholder:text-slate-400" />
      </form>
      <div role="tablist" className="flex flex-wrap gap-1">
        {tabs.map((t) => (
          <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} onClick={() => setTab(t.id)}
            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors ${tab === t.id ? "bg-slate-900 text-white dark:bg-white dark:text-slate-900" : "text-slate-600 hover:bg-slate-200 dark:text-text-secondary dark:hover:bg-raised"}`}>
            {t.label}<span className="tabular-nums opacity-60">{count(t.id)}</span>
          </button>
        ))}
      </div>
      {pasted && !rows.some((r) => r.id === pasted) && (
        <button type="button" onClick={() => open(pasted)} className="flex items-center gap-2 rounded-lg border border-dashed border-slate-300 px-2 py-2 text-left text-xs hover:bg-slate-100 dark:border-border-strong dark:hover:bg-raised">
          <ChatsCircle className="size-4 text-slate-400" /><span className="min-w-0 flex-1 truncate">Abrir <span className="font-mono">{pasted}</span></span><ArrowRight className="size-3.5" />
        </button>
      )}
      <nav aria-label="Lista de hilos" className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1">
        {loading ? <p className="py-6 text-center text-xs text-slate-400">Cargando hilos…</p> : shown.length === 0 ? (
          <p className="py-6 text-center text-xs text-slate-500 dark:text-muted">{q ? "Ningún hilo coincide." : "No hay hilos aquí."}</p>
        ) : (
          <ul className="space-y-0.5">
            {shown.map((r) => (
              <li key={r.id}>
                <Link to="/threads/$threadId" params={{ threadId: r.id }} aria-current={r.id === active ? "page" : undefined}
                  className={`flex items-center gap-2 rounded-lg px-2 py-1.5 transition-colors ${r.id === active ? "bg-slate-200 dark:bg-raised" : "hover:bg-slate-100 dark:hover:bg-raised/60"}`}>
                  <span className="w-4 shrink-0">{r.status ? <ThreadStatus status={r.status} attention={r.attention} /> : <ChatsCircle className="size-4 text-slate-300 dark:text-muted" />}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{r.title}</span>
                    {(r.item || r.project || r.env) && <span className="block truncate text-[11px] text-slate-500 dark:text-muted">{[r.item?.title, r.project, r.env].filter(Boolean).join(" · ")}</span>}
                  </span>
                  {r.updated && <span className="shrink-0 text-[11px] tabular-nums text-slate-400">{ago(r.updated)}</span>}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </nav>
    </div>
  );
}

// ThreadsHome is the welcome screen: a composer in the middle with the
// environment, T3 project and model; sending creates a free thread (no item,
// one can be linked later) and opens it. On phones the list follows below.
export function ThreadsHome() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  // The project filter preselects that project's T3 project (and machines that have it).
  const project = useProject();
  const form = useLaunchForm({ section: "quick", doryProject: project || undefined });
  const [text, setText] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 240)}px`;
  }, [text]);
  const create = useMutation({
    mutationFn: () => api.t3CreateThread({ ...form.value, text: text.trim() }),
    onSuccess: ({ thread_id }) => { form.remember(); qc.invalidateQueries({ queryKey: ["t3-status"] }); navigate({ to: "/threads/$threadId", params: { threadId: thread_id } }); },
  });
  const ready = form.ready && !!text.trim() && !create.isPending;
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center gap-6 px-3 py-8 sm:px-6">
        <h1 className="text-center text-2xl font-semibold tracking-tight">¿En qué trabajamos?</h1>
        <form onSubmit={(e) => { e.preventDefault(); if (ready) create.mutate(); }}
          className="rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
          <textarea ref={area} autoFocus rows={2} value={text} onChange={(e) => setText(e.target.value)} aria-label="Primer mensaje"
            placeholder="Pregunta o pide algo; se crea un hilo nuevo sin tarea"
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); if (ready) create.mutate(); } }}
            className="block max-h-60 w-full resize-none bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
          <div className="flex items-center gap-1 px-2 pb-2">
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5 text-slate-600 dark:text-text-secondary">{form.chips}</div>
            <button type="submit" disabled={!ready} title="Crear el hilo y enviar (Enter)" aria-label="Enviar"
              className="grid size-8 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition-opacity disabled:opacity-30 dark:bg-white dark:text-slate-900"><ArrowUp weight="bold" className="size-4" /></button>
          </div>
        </form>
        <ErrorText error={form.error ?? create.error} />
      </div>
      <div className="border-t border-slate-200 px-1 pt-3 lg:hidden dark:border-border"><ThreadList /></div>
    </div>
  );
}
