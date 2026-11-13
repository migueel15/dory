import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { ArrowRight, ArrowUp, ChatsCircle, DotsThree, GitFork, MagnifyingGlass, PencilSimpleLine, PushPin, SidebarSimple } from "@phosphor-icons/react";
import { api, type Item, type T3Thread } from "../api/client";
import { isSnoozed, isUnread, isWorking, threadMenu, useThreadAction } from "../api/thread-actions";
import { ContextMenu } from "../components/context-menu";
import { ErrorText } from "../components/ui";
import { useLaunchForm } from "./launch-form";
import { useProject } from "../project";
import { ThreadStatus } from "../components/thread-status";

type Row = T3Thread & { instance_id: string; env: string; item?: Item; project?: string; job?: string; known: boolean };
type Tab = "active" | "attention" | "running" | "items" | "jobs" | "snoozed" | "settled" | "archived";
const tabs: { id: Tab; label: string }[] = [
  { id: "active", label: "Activos" }, { id: "attention", label: "Te necesitan" }, { id: "running", label: "Trabajando" },
  { id: "items", label: "Con tarea" }, { id: "jobs", label: "Lanzados" }, { id: "snoozed", label: "Pospuestos" },
  { id: "settled", label: "Asentados" }, { id: "archived", label: "Archivados" },
];
const isTab = (v: unknown): v is Tab => tabs.some((t) => t.id === v);

// threadIdFrom accepts a T3 Code thread link or a bare id.
export const threadIdFrom = (value: string) => {
  const v = value.trim();
  const m = v.match(/\/threads\/([^/?#]+)/);
  if (m) return decodeURIComponent(m[1]);
  return /^[\w-]{8,}$/.test(v) ? v : "";
};

const ago = (iso: string) => span((Date.now() - Date.parse(iso)) / 1000);
// span writes a number of seconds the short way: "ahora", "5 min", "3 h", "2 d".
const span = (secs: number) => {
  const s = Math.max(0, secs);
  if (!Number.isFinite(s)) return "";
  if (s < 60) return "ahora";
  if (s < 3600) return `${Math.floor(s / 60)} min`;
  if (s < 86400) return `${Math.floor(s / 3600)} h`;
  return `${Math.floor(s / 86400)} d`;
};

const roleLabel: Record<string, string> = { refine: "Definir item", triage: "Analizar item", review: "Revisión", replan: "Revisar plan" };
const issueLabel: Record<string, string> = { resolve_conflicts: "Resolver conflictos", review: "Revisar PR", define_issue: "Definir tareas" };

// inTab says whether a thread belongs to a filter. Subagents are not listed
// on their own in "Activos": they hang from their parent.
function inTab(r: Row, tab: Tab) {
  const live = !r.archived_at && !r.settled_at && !isSnoozed(r);
  switch (tab) {
    case "active": return live && r.relation !== "subagent";
    case "attention": return r.attention && !r.archived_at;
    case "running": return isWorking(r.status) && !r.archived_at;
    case "items": return !!r.item && !r.archived_at;
    case "jobs": return !!r.job && !r.archived_at;
    case "snoozed": return isSnoozed(r) && !r.archived_at;
    case "settled": return !!r.settled_at && !r.archived_at;
    case "archived": return !!r.archived_at;
  }
}

// useThreadRows lists every T3 Code thread Dory knows, from all instances,
// with the item, project and Dory job each one belongs to. Threads linked to
// an item that no instance reports any more still show, from the item.
export function useThreadRows() {
  const t3 = useQuery({ queryKey: ["t3-threads"], queryFn: api.t3Threads, refetchInterval: 60_000 });
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 60_000 });
  const issues = useQuery({ queryKey: ["issue-threads"], queryFn: api.issueThreads, refetchInterval: 60_000 });
  const items = useQuery({ queryKey: ["items", "all"], queryFn: () => api.items.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list(), staleTime: 5 * 60_000 });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const rows = useMemo(() => {
    const itemById = new Map((items.data ?? []).map((i) => [i.id, i]));
    const projectName = new Map((projects.data ?? []).map((p) => [p.id, p.name]));
    const envName = new Map((envs.data ?? []).map((e) => [e.id, e.name]));
    const itemOf = new Map<string, { item: Item; role: string; instance: string }>();
    for (const l of links.data ?? []) {
      const it = itemById.get(l.item_id);
      if (it && !itemOf.has(l.thread_id)) itemOf.set(l.thread_id, { item: it, role: l.role, instance: l.instance_id });
    }
    const jobOf = new Map<string, string>();
    for (const j of issues.data ?? []) jobOf.set(j.thread_id, `${issueLabel[j.kind] ?? "Issue"} · ${j.repo}#${j.number}`);
    const out = new Map<string, Row>();
    for (const inst of t3.data ?? []) {
      for (const t of inst.threads) {
        const link = itemOf.get(t.id);
        const item = link?.item;
        out.set(t.id, {
          ...t, title: t.title || item?.title || t.id, instance_id: inst.instance_id, env: envName.get(inst.environment_id) ?? "", item, known: true,
          project: item?.project_id ? projectName.get(item.project_id) : undefined,
          job: jobOf.get(t.id) ?? (link ? roleLabel[link.role] ?? "Implementar item" : undefined),
        });
      }
    }
    for (const [tid, { item, role, instance }] of itemOf) {
      if (out.has(tid) || item.archived_at || item.status === "done" || item.status === "cancelled") continue;
      out.set(tid, {
        id: tid, title: item.title, status: "", attention: false, model: "", effort: "", updated_at: item.updated_at, project_id: "", url: "", t3_environment_id: "",
        instance_id: instance, env: "", item, known: false, project: item.project_id ? projectName.get(item.project_id) : undefined, job: roleLabel[role] ?? "Implementar item",
      });
    }
    return [...out.values()].sort((a, b) => Number(b.attention) - Number(a.attention) || Number(isWorking(b.status)) - Number(isWorking(a.status)) || b.updated_at.localeCompare(a.updated_at));
  }, [t3.data, links.data, issues.data, items.data, envs.data, projects.data]);
  return { rows, loading: t3.isPending, error: t3.error };
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
      <aside aria-label="Hilos" className={`min-h-0 flex-col overflow-hidden transition-[width] duration-300 ease-out lg:flex lg:shrink-0 ${folded ? "lg:w-0" : "lg:w-80"} hidden`}>
        <div className="flex min-h-0 w-80 flex-1 flex-col"><ThreadList active={threadId} onFold={() => fold(true)} /></div>
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

// The filter lives in ?f= so other pages can link to it (attention, jobs).
function useTab(): [Tab, (t: Tab) => void] {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { f?: string };
  const tab = isTab(search.f) ? search.f : "active";
  return [tab, (t) => navigate({ to: ".", search: (old: Record<string, unknown>) => ({ ...old, f: t === "active" ? undefined : t }), replace: true })];
}

// ThreadList is the side list: new thread, search (or a pasted T3 link),
// filters, pinned threads and the rest, with subagents under their parent.
function ThreadList({ active, onFold }: { active?: string; onFold?: () => void }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  // Hovering a thread loads its conversation, so opening it is instant.
  const prefetch = (id: string) => { void qc.prefetchQuery({ queryKey: ["t3-view", id], queryFn: () => api.t3View(id) }); };
  const { rows, loading, error } = useThreadRows();
  const act = useThreadAction();
  const [query, setQuery] = useState("");
  const [tab, setTab] = useTab();
  const [renaming, setRenaming] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number; row: Row }>();
  const pasted = threadIdFrom(query);
  const q = query.trim().toLowerCase();
  // Subagents stay out of the list: each thread shows its own in its details.
  const listed = useMemo(() => rows.filter((r) => r.relation !== "subagent"), [rows]);
  // A search looks through every thread, whatever the filter.
  const matches = (r: Row) => pasted === r.id || [r.title, r.item?.title, r.project, r.model, r.env, r.job, r.branch, r.id].some((x) => x?.toLowerCase().includes(q));
  const top = q ? listed.filter((r) => !r.archived_at || tab === "archived").filter(matches) : listed.filter((r) => inTab(r, tab));
  const pinned = tab === "active" && !q ? top.filter((r) => r.pinned_at).sort((a, b) => (a.pin_order_key ?? a.pinned_at ?? "").localeCompare(b.pin_order_key ?? b.pinned_at ?? "")) : [];
  const rest = pinned.length ? top.filter((r) => !r.pinned_at) : top;
  const count = (t: Tab) => listed.filter((r) => inTab(r, t)).length;
  const open = (id: string) => navigate({ to: "/threads/$threadId", params: { threadId: id } });
  const line = (r: Row): React.ReactNode => {
    return (
      <li key={r.id}>
        <div className={`group relative flex items-center rounded-lg transition-colors ${r.id === active ? "bg-slate-200 dark:bg-raised" : "hover:bg-slate-100 dark:hover:bg-raised/60"}`}
          onContextMenu={(e) => { if (!r.known) return; e.preventDefault(); setMenu({ x: e.clientX, y: e.clientY, row: r }); }}>
          {renaming === r.id ? (
            <form className="min-w-0 flex-1 px-2 py-1" onSubmit={(e) => { e.preventDefault(); const t = new FormData(e.currentTarget).get("t")?.toString().trim(); setRenaming(""); if (t && t !== r.title) act.mutate({ id: r.id, action: "rename", title: t }); }}>
              <input name="t" autoFocus defaultValue={r.title} aria-label="Nuevo título" onBlur={(e) => e.currentTarget.form?.requestSubmit()} onKeyDown={(e) => { if (e.key === "Escape") setRenaming(""); }}
                className="w-full rounded border border-slate-300 bg-white px-1.5 py-0.5 text-sm dark:border-border-strong dark:bg-surface" />
            </form>
          ) : (
            <Link to="/threads/$threadId" params={{ threadId: r.id }} aria-current={r.id === active ? "page" : undefined} onMouseEnter={() => prefetch(r.id)} onFocus={() => prefetch(r.id)}
              onDoubleClick={(e) => { if (!r.known) return; e.preventDefault(); setRenaming(r.id); }}
              className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5">
              <span className="w-4 shrink-0">{r.status ? <ThreadStatus status={r.status} attention={r.attention} /> : <ChatsCircle className="size-4 text-slate-300 dark:text-muted" />}</span>
              <span className="min-w-0 flex-1">
                <span className={`flex items-center gap-1 truncate text-sm ${isUnread(r) ? "font-semibold" : ""}`}>
                  {r.relation === "fork" && <GitFork className="size-3 shrink-0 text-slate-400" aria-label="Bifurcación" />}
                  <span className="truncate">{r.title}</span>
                </span>
                {(r.item || r.project || r.env || r.job) && (
                  <span className="block truncate text-[11px] text-slate-500 dark:text-muted">
                    {[r.job, r.item?.title, r.project, r.env].filter(Boolean).join(" · ")}
                  </span>
                )}
              </span>
              {isUnread(r) && <span className="size-2 shrink-0 rounded-full bg-blue-500" aria-label="Sin leer" />}
              {r.pinned_at && <PushPin weight="fill" className="size-3 shrink-0 text-slate-400" aria-label="Fijado" />}
              <span className="shrink-0 text-[11px] tabular-nums text-slate-400 group-hover:invisible">{isSnoozed(r) ? `vuelve en ${span((Date.parse(r.snoozed_until!) - Date.now()) / 1000)}` : ago(r.updated_at)}</span>
            </Link>
          )}
          {r.known && renaming !== r.id && (
            <button type="button" aria-label={`Acciones de ${r.title}`} title="Acciones"
              onClick={(e) => { const b = e.currentTarget.getBoundingClientRect(); setMenu({ x: b.left, y: b.bottom, row: r }); }}
              className="absolute right-1 grid size-6 place-items-center rounded text-slate-500 opacity-0 hover:bg-slate-200 focus:opacity-100 group-hover:opacity-100 dark:hover:bg-border"><DotsThree weight="bold" className="size-4" /></button>
          )}
        </div>
      </li>
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex items-center gap-1">
        <Link to="/threads" className="inline-flex flex-1 items-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium hover:bg-slate-200 dark:hover:bg-raised"><PencilSimpleLine className="size-4" />Nuevo hilo</Link>
        {onFold && <button type="button" onClick={onFold} title="Ocultar la lista" aria-label="Ocultar la lista de hilos" className="hidden size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 lg:grid dark:text-muted dark:hover:bg-raised dark:hover:text-white"><SidebarSimple className="size-4" /></button>}
      </div>
      <form onSubmit={(e) => { e.preventDefault(); const id = top[0]?.id ?? pasted; if (id) open(id); }}
        className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-2 focus-within:border-slate-400 dark:border-border dark:bg-surface dark:focus-within:border-muted">
        <MagnifyingGlass className="size-4 shrink-0 text-slate-400" />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar en todos o pegar enlace de T3"
          aria-label="Buscar hilos" className="min-h-9 w-full bg-transparent text-sm !outline-none placeholder:text-slate-400" />
      </form>
      <div role="tablist" className="flex flex-wrap gap-1">
        {tabs.map((t) => {
          const n = count(t.id);
          if (!n && t.id !== "active" && t.id !== tab) return null;
          return (
            <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} onClick={() => setTab(t.id)}
              className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors ${tab === t.id ? "bg-slate-900 text-white dark:bg-white dark:text-slate-900" : "text-slate-600 hover:bg-slate-200 dark:text-text-secondary dark:hover:bg-raised"}`}>
              {t.label}<span className="tabular-nums opacity-60">{n}</span>
            </button>
          );
        })}
      </div>
      {pasted && !rows.some((r) => r.id === pasted) && (
        <button type="button" onClick={() => open(pasted)} className="flex items-center gap-2 rounded-lg border border-dashed border-slate-300 px-2 py-2 text-left text-xs hover:bg-slate-100 dark:border-border-strong dark:hover:bg-raised">
          <ChatsCircle className="size-4 text-slate-400" /><span className="min-w-0 flex-1 truncate">Abrir <span className="font-mono">{pasted}</span></span><ArrowRight className="size-3.5" />
        </button>
      )}
      <nav aria-label="Lista de hilos" className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1">
        <ErrorText error={error} />
        {loading ? <p className="py-6 text-center text-xs text-slate-400">Cargando hilos…</p> : top.length === 0 ? (
          <p className="py-6 text-center text-xs text-slate-500 dark:text-muted">{q ? "Ningún hilo coincide." : "No hay hilos aquí."}</p>
        ) : (
          <>
            {pinned.length > 0 && <>
              <p className="px-2 pb-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Fijados</p>
              <ul className="mb-2 space-y-0.5">{pinned.map((r) => line(r))}</ul>
              <p className="px-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Recientes</p>
            </>}
            <ul className="space-y-0.5">{rest.map((r) => line(r))}</ul>
          </>
        )}
      </nav>
      {menu && <ContextMenu x={menu.x} y={menu.y} label={`Acciones de ${menu.row.title}`} submenuLabel="Posponer hasta" onClose={() => setMenu(undefined)}
        actions={threadMenu(menu.row, (a) => act.mutate({ id: menu.row.id, ...a }), { onRename: () => setRenaming(menu.row.id) })} />}
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
