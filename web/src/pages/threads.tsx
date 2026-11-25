import { Check,FunnelSimple,MagnifyingGlass,Plus,PushPin } from "@phosphor-icons/react";
import { Link,Outlet,useNavigate,useParams } from "@tanstack/react-router";
import { useMemo,useRef,useState,type CSSProperties,type ReactNode,type PointerEvent as ReactPointerEvent } from "react";
import { repoName,type Thread } from "../api/client";
import { useMesh,useNode,useRepositories } from "../api/queries/hooks";
import { useExternalSessions,useThreadList } from "../api/queries/threads";
import { EnvIcon } from "../components/env-icon";
import { useCommand } from "../components/shortcuts";
import { ErrorText } from "../components/ui";
import { ProviderIcon } from "../features/composer/launch-picker";
import { KeptWorktrees } from "../features/settings/kept-worktrees";
import { useThreadMenu } from "../features/thread/thread-menu";
import { RefChip,useThreadRefs,type GhRef } from "../features/thread/thread-refs";
import { storage,storageKeys } from "../lib/storage";
import { events,useEmitter } from "../lib/store";
import { ThreadsFold } from "../lib/threads-fold";
import { useProject } from "../project";
import { NewThread } from "./home";

// Filters of the single inbox of threads (requirement RF-19).
const FILTERS = { active: "Activos", needs: "Te necesitan", working: "Trabajando", archived: "Archivados", external: "En los agentes" } as const;
type Filter = keyof typeof FILTERS;

const LIST_W = 336;
const clampList = (w: number) => Math.round(Math.min(Math.max(w, 240), 560));

const snoozed = (t: Thread) => !!t.snoozed_until && new Date(t.snoozed_until) > new Date();

// Threads keeps the list of threads on the left and the open one on the right.
export function Threads() {
  const { threadId } = useParams({ strict: false }) as { threadId?: string };
  const project = useProject();
  const [filter, setFilter] = useState<Filter>("active");
  const [query, setQuery] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  // On desktop the thread list folds away while a thread is open; remembered in this browser.
  const [listFolded, setListFolded] = useState(() => { try { return storage.get(storageKeys.threadsFolded) === "1"; } catch { return false; } });
  const foldList = (v: boolean) => { setListFolded(v); try { storage.set(storageKeys.threadsFolded, v ? "1" : "0"); } catch { /* per-browser only */ } };
  const searchInput = useRef<HTMLInputElement>(null);
  useEmitter(events.focusThreadSearch, () => { foldList(false); requestAnimationFrame(() => searchInput.current?.focus()); });
  useEmitter(events.toggleThreadList, () => foldList(!listFolded));
  const folded = listFolded;
  useCommand("thread-list", () => foldList(!listFolded));
  // The list width is dragged from its right edge and kept per browser.
  const [listWidth, setListWidth] = useState(() => { try { return clampList(Number(storage.get(storageKeys.threadsWidth)) || LIST_W); } catch { return LIST_W; } });
  const [dragging, setDragging] = useState(false);
  const saveWidth = (w: number) => { setListWidth(w); try { storage.set(storageKeys.threadsWidth, String(w)); } catch { /* per-browser only */ } };
  const startDrag = (e: ReactPointerEvent) => {
    e.preventDefault();
    const x0 = e.clientX, w0 = listWidth;
    let w = w0;
    setDragging(true);
    const move = (ev: PointerEvent) => { w = clampList(w0 + ev.clientX - x0); setListWidth(w); };
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); document.body.style.cursor = ""; setDragging(false); saveWidth(w); };
    document.body.style.cursor = "col-resize";
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };
  const threads = useThreadList(filter, { refetchInterval: 15_000 });
  const nodes = useNode({ staleTime: 60_000 });
  // Conversations started in the harnesses of this node, outside Dory.
  const external = useExternalSessions("local", { enabled: filter === "external", refetchInterval: 30_000 });
  const local = nodes.data?.node.id;
  const mesh = useMesh({ staleTime: 30_000 });
  // The environment a thread runs in, to show its icon in the list.
  const envOf = (id: string) => {
    if (id === local && nodes.data) return { name: nodes.data.node.name, icon: nodes.data.node.icon, os: nodes.data.node.os };
    const p = mesh.data?.peers.find((x) => x.id === id);
    return p ? { name: p.name, icon: p.info.node?.icon, os: p.info.node?.os } : { name: "Otro nodo" };
  };
  // Pull requests and issues of each thread, through its task.
  const refsOf = useThreadRefs();
  // Each thread shows the repositories of its project as owner/repo.
  const repos = useRepositories({ staleTime: 60_000 });
  const repoOf = (projectID: string | null) => (repos.data ?? []).filter((r) => projectID && r.project_id === projectID && !r.archived_at).map((r) => repoName(r.remote_url)).join(", ");
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (threads.data ?? [])
      // Subagents live in their parent's details, not in the list.
      .filter((t) => !t.parent_id)
      .filter((t) => !project || t.project_id === project)
      .filter((t) => !q || t.title.toLowerCase().includes(q) || repoOf(t.project_id).toLowerCase().includes(q))
      .filter((t) => {
        switch (filter) {
          case "archived": return !!t.archived_at;
          case "needs": return !t.archived_at && (t.status === "waiting" || t.status === "error");
          case "working": return !t.archived_at && t.status === "working";
          default: return !t.archived_at && !t.settled_at && !snoozed(t);
        }
      })
      .sort((a, b) => (b.pinned_at ? 1 : 0) - (a.pinned_at ? 1 : 0) || (b.last_activity_at ?? b.created_at).localeCompare(a.last_activity_at ?? a.created_at));
  }, [threads.data, repos.data, filter, query, project]);
  return (
    <div className="relative flex h-[calc(100dvh_-_4rem_-_1px_-_var(--tabbar))] min-h-0 sm:h-[calc(100dvh-5.5rem-2px)]" style={{ "--list-w": `${listWidth}px` } as CSSProperties}>
      <aside inert={folded} className={`relative ${dragging ? "sm:!transition-none" : ""} min-h-0 w-full shrink-0 flex-col overflow-hidden border-slate-200 sm:-mt-2.5 sm:border-r dark:border-border sm:-ml-4 sm:items-end sm:transition-[width,border-color] sm:duration-300 sm:ease-out motion-reduce:transition-none ${folded ? "sm:w-4 sm:!border-r-0" : "sm:w-[var(--list-w)]"} ${threadId ? "hidden sm:flex" : "flex"}`}>
        {!folded && <div role="separator" aria-orientation="vertical" aria-label="Redimensionar lista de hilos" onPointerDown={startDrag} onDoubleClick={() => saveWidth(LIST_W)}
          className="absolute inset-y-0 right-0 z-10 hidden w-1.5 translate-x-1/2 cursor-col-resize hover:bg-accent/40 sm:block" />}
        {/* The content keeps its width while the list slides open or closed. */}
        <div className={`flex min-h-0 w-full flex-1 flex-col sm:w-[var(--list-w)] sm:shrink-0 sm:transition-[opacity,visibility] sm:duration-300 motion-reduce:transition-none ${folded ? "sm:invisible sm:opacity-0" : ""}`}>
        <div className="flex items-center gap-1.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 items-center gap-2 px-2">
            <MagnifyingGlass weight="regular" className="size-4 shrink-0 text-slate-400" />
            <input ref={searchInput} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar hilo…" aria-label="Buscar hilos"
              className="min-h-9 w-full bg-transparent text-sm !outline-none placeholder:text-slate-400" />
          </div>
          <div className="relative shrink-0">
            {filterOpen && <div className="fixed inset-0 z-10" onClick={() => setFilterOpen(false)} />}
            <button type="button" onClick={() => setFilterOpen((v) => !v)} title={`Filtro: ${FILTERS[filter]}`} aria-label="Filtrar hilos" aria-haspopup="menu" aria-expanded={filterOpen}
              className={`relative grid size-8 place-items-center rounded-lg hover:bg-slate-100 dark:hover:bg-raised ${filter === "active" ? "text-slate-600 dark:text-text-secondary" : "text-slate-900 dark:text-white"}`}>
              <FunnelSimple weight={filter === "active" ? "regular" : "fill"} className="size-4" />
            </button>
            {filterOpen && <div role="menu" className="absolute right-0 top-9 z-20 w-44 rounded-lg border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
              {(Object.keys(FILTERS) as Filter[]).map((f) => (
                <button key={f} type="button" role="menuitemradio" aria-checked={filter === f} onClick={() => { setFilter(f); setFilterOpen(false); }}
                  className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                  {FILTERS[f]}{filter === f && <Check className="size-3.5" />}
                </button>
              ))}
            </div>}
          </div>
          <Link to="/threads" title="Nuevo hilo" aria-label="Nuevo hilo" activeOptions={{ exact: true }}
            className="grid size-8 shrink-0 place-items-center rounded-lg text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"><Plus weight="regular" className="size-4" /></Link>
        </div>
        <KeptWorktrees />
        <div className="min-h-0 flex-1 no-scrollbar overflow-y-auto p-1">
          <ErrorText error={threads.error} />
          {filter === "external" && <ErrorText error={external.error} />}
          {filter === "external" && (external.data ?? []).filter((e) => !project || e.project_id === project)
            .filter((e) => !query.trim() || e.title.toLowerCase().includes(query.trim().toLowerCase()) || e.dir.toLowerCase().includes(query.trim().toLowerCase()))
            .map((e) => (
              <Link key={`${e.provider}:${e.id}`} to={e.thread_id ? "/threads/$threadId" : "/threads/external/$node/$provider/$id"}
                params={(e.thread_id ? { threadId: e.thread_id } : { node: "local", provider: e.provider, id: e.id }) as never}
                className="flex items-center gap-2 rounded-md px-2 py-2 text-sm hover:bg-slate-100 dark:hover:bg-raised [&.active]:bg-slate-200 dark:[&.active]:bg-raised">
                <ProviderIcon driver={e.provider} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{e.title}</span>
                  <span className="block truncate font-mono text-[0.6875rem] text-slate-500 dark:text-muted">{e.dir.replace(/^\/home\/[^/]+/, "~")}</span>
                </span>
                {e.thread_id && <span className="shrink-0 text-[0.625rem] text-slate-500">en Dory</span>}
              </Link>
            ))}
          {filter === "external" && external.isPending && <p className="p-3 text-sm text-slate-500">Leyendo las conversaciones…</p>}
          {filter !== "external" && <ul className="space-y-0.5">{shown.map((t) => (
            <ThreadRow key={t.id} t={t} sub={repoOf(t.project_id)} refs={refsOf(t.task_id)} env={envOf(t.node_id)} listVisible={!folded} />
          ))}</ul>}
          {filter !== "external" && threads.isSuccess && shown.length === 0 && <p className="p-3 text-sm text-slate-500 dark:text-muted">No hay hilos aquí.</p>}
        </div>
        </div>
      </aside>
      <section className={`min-h-0 min-w-0 flex-1 ${folded ? "sm:-ml-2" : ""} sm:transition-[margin] sm:duration-300 sm:ease-out ${threadId ? "flex flex-col" : "hidden sm:flex sm:flex-col"}`}><ThreadsFold.Provider value={threadId ? { folded, toggle: () => foldList(!folded) } : null}><Outlet /></ThreadsFold.Provider></section>
    </div>
  );
}

// ago writes how long ago something happened: "ahora", "5m", "3h", "2d".
function ago(iso: string) {
  const s = (Date.now() - Date.parse(iso)) / 1000;
  if (!Number.isFinite(s) || s < 60) return "ahora";
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

// ThreadsHome fills the right side when no thread is open: a new chat.
export function ThreadsHome() {
  const navigate = useNavigate();
  // The main frame pads the right edge too: drop it here so the chat sits centred between the list and the window edge.
  return (
    <div className="grid h-full place-items-center p-4 sm:-mr-4">
      <div className="w-full max-w-3xl">
        <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
      </div>
    </div>
  );
}

// ThreadRow is one thread of the side list; right click opens its actions.
function ThreadRow({ t, sub, refs, env, listVisible }: { t: Thread; sub: string; refs: GhRef[]; env: { name: string; icon?: string; os?: string }; listVisible: boolean }) {
  const menu = useThreadMenu(t);
  const offline = t.state_stale;
  const unread = t.unread;
  const icons = <>
    {refs.map((r) => <RefChip key={r.kind} r={r} listVisible={listVisible} />)}
    <span className="flex shrink-0 items-center text-slate-400 dark:text-muted"><EnvIcon {...env} /></span>
    <ProviderIcon driver={t.provider} model={t.model} compact />
  </>;
  const body = <>
        {/* Repository, then the state: a pending action replaces the time. */}
        <span className="flex h-4 min-w-0 items-center gap-2 text-[0.6875rem] text-slate-500 dark:text-muted">
          <span className="min-w-0 flex-1 truncate">{sub || "Sin proyecto"}</span>
          {t.pinned_at && <PushPin weight="fill" className="size-3 shrink-0 text-slate-400" aria-label="Fijado" />}
          <RowState t={t} unread={unread} offline={offline} />
        </span>
        {/* Without a branch the last line goes away and its icons join the title. */}
        <span className="flex min-w-0 items-center gap-2">
          <span className={`min-w-0 flex-1 truncate text-sm ${unread ? "font-semibold" : ""}`}>{t.title || "Hilo"}</span>
          {!t.branch && icons}
        </span>
        {t.branch && <span className="flex h-4 min-w-0 items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-mono text-[0.6875rem] text-slate-500 dark:text-muted">{t.branch}</span>
          {icons}
        </span>}
  </>;
  return (
    <li>
      {/* A thread whose node is offline stays listed but cannot be opened. */}
      {offline ? <div aria-disabled title={`${env.name} no está conectado: este hilo no se puede abrir ahora`} onContextMenu={menu.open}
        className="flex min-w-0 cursor-not-allowed flex-col gap-0.5 rounded-lg px-3 py-2.5 opacity-50">{body}</div>
      : <Link to="/threads/$threadId" params={{ threadId: t.id }} onContextMenu={menu.open}
        className="group flex min-w-0 flex-col gap-0.5 rounded-lg px-3 py-2.5 transition-colors hover:bg-slate-100 dark:hover:bg-raised/60 [&.active]:bg-slate-200 dark:[&.active]:bg-raised">{body}</Link>}
      {menu.menu}
    </li>
  );
}

// RowState shows what a thread needs, or how long ago it was active when it needs nothing.
function RowState({ t, unread, offline }: { t: Thread; unread: boolean; offline?: boolean }) {
  const pill = (dot: ReactNode, label: string, tone: string) => (
    <span className={`inline-flex shrink-0 items-center gap-1.5 font-medium ${tone}`}>{dot}{label}</span>
  );
  if (offline) return pill(<span className="size-2 rounded-full bg-slate-400" />, "Sin conexión", "text-slate-500 dark:text-muted");
  if (t.status === "working") return pill(<span className="size-2.5 animate-spin rounded-full border-2 border-blue-500 border-t-transparent" />, "En marcha", "text-blue-600 dark:text-blue-400");
  if (t.status === "waiting") return pill(<span className="size-2 rounded-full bg-amber-500 motion-safe:animate-pulse" />, "Te necesita", "text-amber-600 dark:text-amber-400");
  if (t.status === "error") return pill(<span className="size-2 rounded-full bg-red-500" />, "Error", "text-red-600 dark:text-red-400");
  if (unread) return pill(<span className="size-2 rounded-full bg-emerald-500" />, "Terminado", "text-emerald-600 dark:text-emerald-400");
  return <span className="shrink-0 tabular-nums text-slate-400">{ago(t.last_activity_at ?? t.created_at)}</span>;
}
