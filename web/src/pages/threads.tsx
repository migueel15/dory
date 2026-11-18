import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams } from "@tanstack/react-router";
import { ChatsCircle, Check, FunnelSimple, MagnifyingGlass, Plus, PushPin, SidebarSimple } from "@phosphor-icons/react";
import { api, repoName, type Thread } from "../api/client";
import { ErrorText } from "../components/ui";
import { useProject } from "../project";
import { ProviderIcon } from "../components/launch-picker";
import { StatusDot } from "./thread";
import { NewThread } from "./home";
import { useThreadMenu } from "../components/thread-menu";

// Filters of the single inbox of threads (requirement RF-19).
const FILTERS = { active: "Activos", needs: "Te necesitan", working: "Trabajando", archived: "Archivados", external: "En los agentes" } as const;
type Filter = keyof typeof FILTERS;

export const isUnread = (t: Thread) => !t.read_at && t.status !== "working" && !!t.last_activity_at;
const snoozed = (t: Thread) => !!t.snoozed_until && new Date(t.snoozed_until) > new Date();

// Threads keeps the list of threads on the left and the open one on the right.
export function Threads() {
  const { threadId } = useParams({ strict: false }) as { threadId?: string };
  const project = useProject();
  const [filter, setFilter] = useState<Filter>("active");
  const [query, setQuery] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  // On desktop the thread list folds away while a thread is open; remembered in this browser.
  const [listFolded, setListFolded] = useState(() => { try { return localStorage.getItem("dory.threads-folded") === "1"; } catch { return false; } });
  const foldList = (v: boolean) => { setListFolded(v); try { localStorage.setItem("dory.threads-folded", v ? "1" : "0"); } catch { /* per-browser only */ } };
  const folded = listFolded && !!threadId;
  const threads = useQuery({
    queryKey: ["threads", filter],
    queryFn: () => api.threads(undefined, filter === "archived"),
    refetchInterval: 15_000,
  });
  const nodes = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000 });
  // Conversations started in the harnesses of this node, outside Dory.
  const external = useQuery({ queryKey: ["external", "local"], queryFn: () => api.externalSessions("local"), enabled: filter === "external", refetchInterval: 30_000 });
  const local = nodes.data?.node.id;
  // Each thread shows the repositories of its project as owner/repo.
  const repos = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list(), staleTime: 60_000 });
  const repoOf = (projectID: string | null) => (repos.data ?? []).filter((r) => projectID && r.project_id === projectID && !r.archived_at).map((r) => repoName(r.remote_url)).join(", ");
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (threads.data ?? [])
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
    <div className="flex h-[calc(100dvh-4rem-1px)] min-h-0 sm:h-[calc(100dvh-3rem-2px)]">
      {folded && <div className="hidden shrink-0 border-r border-slate-200 p-2 sm:block dark:border-border">
        <button type="button" onClick={() => foldList(false)} title="Mostrar los hilos" aria-label="Mostrar los hilos"
          className="grid size-9 place-items-center rounded-lg text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised"><SidebarSimple weight="regular" className="size-4" /></button>
      </div>}
      <aside className={`min-h-0 w-full shrink-0 flex-col border-slate-200 sm:w-80 sm:border-r dark:border-border ${threadId ? (folded ? "hidden" : "hidden sm:flex") : "flex"}`}>
        <div className="flex items-center gap-1.5 p-2">
          {threadId && <button type="button" onClick={() => foldList(true)} title="Ocultar los hilos" aria-label="Ocultar los hilos"
            className="hidden size-9 shrink-0 place-items-center rounded-lg text-slate-600 hover:bg-slate-100 sm:grid dark:text-text-secondary dark:hover:bg-raised"><SidebarSimple weight="regular" className="size-4" /></button>}
          <div className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-slate-200 bg-white px-2 focus-within:border-slate-400 dark:border-border dark:bg-surface dark:focus-within:border-muted">
            <MagnifyingGlass className="size-4 shrink-0 text-slate-400" />
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar hilo…" aria-label="Buscar hilos"
              className="min-h-9 w-full bg-transparent text-sm !outline-none placeholder:text-slate-400" />
          </div>
          <Link to="/threads" title="Nuevo hilo" aria-label="Nuevo hilo" activeOptions={{ exact: true }}
            className="grid size-9 shrink-0 place-items-center rounded-lg border border-slate-200 text-slate-600 hover:bg-slate-100 dark:border-border dark:text-text-secondary dark:hover:bg-raised"><Plus weight="regular" className="size-4" /></Link>
          <div className="relative shrink-0">
            {filterOpen && <div className="fixed inset-0 z-10" onClick={() => setFilterOpen(false)} />}
            <button type="button" onClick={() => setFilterOpen((v) => !v)} title={`Filtro: ${FILTERS[filter]}`} aria-label="Filtrar hilos" aria-haspopup="menu" aria-expanded={filterOpen}
              className={`relative grid size-9 place-items-center rounded-lg border border-slate-200 hover:bg-slate-100 dark:border-border dark:hover:bg-raised ${filter === "active" ? "text-slate-600 dark:text-text-secondary" : "text-slate-900 dark:text-white"}`}>
              <FunnelSimple weight={filter === "active" ? "regular" : "fill"} className="size-4" />
            </button>
            {filterOpen && <div role="menu" className="absolute right-0 top-10 z-20 w-44 rounded-lg border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
              {(Object.keys(FILTERS) as Filter[]).map((f) => (
                <button key={f} type="button" role="menuitemradio" aria-checked={filter === f} onClick={() => { setFilter(f); setFilterOpen(false); }}
                  className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                  {FILTERS[f]}{filter === f && <Check className="size-3.5" />}
                </button>
              ))}
            </div>}
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-1">
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
                  <span className="block truncate font-mono text-[11px] text-slate-500 dark:text-muted">{e.dir.replace(/^\/home\/[^/]+/, "~")}</span>
                </span>
                {e.thread_id && <span className="shrink-0 text-[10px] text-slate-500">en Dory</span>}
              </Link>
            ))}
          {filter === "external" && external.isPending && <p className="p-3 text-sm text-slate-500">Leyendo las conversaciones…</p>}
          {filter !== "external" && <ul className="space-y-0.5">{shown.map((t) => (
            <ThreadRow key={t.id} t={t} sub={[repoOf(t.project_id), t.node_id !== local ? "otro nodo" : ""].filter(Boolean).join(" · ")} />
          ))}</ul>}
          {filter !== "external" && threads.isSuccess && shown.length === 0 && <p className="p-3 text-sm text-slate-500 dark:text-muted">No hay hilos aquí.</p>}
        </div>
      </aside>
      <section className={`min-h-0 min-w-0 flex-1 ${threadId ? "flex flex-col" : "hidden sm:flex sm:flex-col"}`}><Outlet /></section>
    </div>
  );
}

// ListDot is the v1 state of a thread in the list.
function ListDot({ status }: { status: string }) {
  if (status === "working") return <span className="size-3 animate-spin rounded-full border-2 border-blue-500 border-t-transparent" aria-label="Trabajando" />;
  if (status === "waiting") return <span className="size-2 rounded-full bg-amber-500 motion-safe:animate-pulse" aria-label="Te necesita" />;
  if (status === "error") return <span className="size-2 rounded-full bg-red-500" aria-label="Error" />;
  return <ChatsCircle className="size-4 text-slate-300 dark:text-muted" aria-label="Inactivo" />;
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
  return (
    <div className="grid h-full place-items-center p-4">
      <div className="w-full max-w-3xl">
        <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
      </div>
    </div>
  );
}

// ThreadRow is one thread of the side list; right click opens its actions.
function ThreadRow({ t, sub }: { t: Thread; sub: string }) {
  const menu = useThreadMenu(t);
  return (
    <li>
              <Link to="/threads/$threadId" params={{ threadId: t.id }} onContextMenu={menu.open}
                className="group flex min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-slate-100 dark:hover:bg-raised/60 [&.active]:bg-slate-200 dark:[&.active]:bg-raised">
                <span className="grid w-4 shrink-0 place-items-center"><ListDot status={t.status} /></span>
                <span className="min-w-0 flex-1">
                  <span className={`block truncate text-sm ${isUnread(t) ? "font-semibold" : ""}`}>{t.title || "Hilo"}</span>
                  {sub && <span className="block truncate font-mono text-[11px] text-slate-500 dark:text-muted">{sub}</span>}
                </span>
                {isUnread(t) && <span className="size-2 shrink-0 rounded-full bg-blue-500" aria-label="Sin leer" />}
                {t.pinned_at && <PushPin weight="fill" className="size-3 shrink-0 text-slate-400" aria-label="Fijado" />}
                <span className="shrink-0"><ProviderIcon driver={t.provider} model={t.model} /></span>
                <span className="shrink-0 text-[11px] tabular-nums text-slate-400">{ago(t.last_activity_at ?? t.created_at)}</span>
              </Link>
      {menu.menu}
    </li>
  );
}
