import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams } from "@tanstack/react-router";
import { ChatsCircle, MagnifyingGlass, PushPin } from "@phosphor-icons/react";
import { api, type Thread } from "../api/client";
import { ErrorText } from "../components/ui";
import { useProject } from "../project";
import { ProviderIcon } from "../components/launch-picker";
import { StatusDot } from "./thread";
import { NewThread } from "./home";

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
  const threads = useQuery({
    queryKey: ["threads", filter],
    queryFn: () => api.threads(undefined, filter === "archived"),
    refetchInterval: 15_000,
  });
  const nodes = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000 });
  // Conversations started in the harnesses of this node, outside Dory.
  const external = useQuery({ queryKey: ["external", "local"], queryFn: () => api.externalSessions("local"), enabled: filter === "external", refetchInterval: 30_000 });
  const local = nodes.data?.node.id;
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (threads.data ?? [])
      .filter((t) => !project || t.project_id === project)
      .filter((t) => !q || t.title.toLowerCase().includes(q) || t.branch.toLowerCase().includes(q))
      .filter((t) => {
        switch (filter) {
          case "archived": return !!t.archived_at;
          case "needs": return !t.archived_at && (t.status === "waiting" || t.status === "error");
          case "working": return !t.archived_at && t.status === "working";
          default: return !t.archived_at && !t.settled_at && !snoozed(t);
        }
      })
      .sort((a, b) => (b.pinned_at ? 1 : 0) - (a.pinned_at ? 1 : 0) || (b.last_activity_at ?? b.created_at).localeCompare(a.last_activity_at ?? a.created_at));
  }, [threads.data, filter, query, project]);
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] min-h-0 sm:h-[calc(100dvh-3rem-2px)]">
      <aside className={`min-h-0 w-full shrink-0 flex-col border-slate-200 sm:w-80 sm:border-r dark:border-border ${threadId ? "hidden sm:flex" : "flex"}`}>
        <div className="space-y-2 border-b border-slate-200 p-2 dark:border-border">
          <div className="relative">
            <MagnifyingGlass className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar hilo…" aria-label="Buscar hilo"
              className="w-full rounded-md border border-slate-300 bg-transparent py-1.5 pl-8 pr-2 text-sm dark:border-border" />
          </div>
          <div className="flex flex-wrap gap-1">
            {(Object.keys(FILTERS) as Filter[]).map((f) => (
              <button key={f} type="button" onClick={() => setFilter(f)} className={`rounded-full px-2.5 py-0.5 text-xs ${filter === f ? "bg-slate-900 text-white dark:bg-accent" : "bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-raised dark:text-muted"}`}>{FILTERS[f]}</button>
            ))}
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
          {filter !== "external" && shown.map((t) => (
            <Link key={t.id} to="/threads/$threadId" params={{ threadId: t.id }}
              className="flex items-center gap-2 rounded-md px-2 py-2 text-sm hover:bg-slate-100 dark:hover:bg-raised [&.active]:bg-slate-200 dark:[&.active]:bg-raised">
              <ProviderIcon driver={t.provider} model={t.model} />
              <span className="min-w-0 flex-1">
                <span className={`block truncate ${isUnread(t) ? "font-semibold" : ""}`}>{t.title || "Hilo"}</span>
                <span className="block truncate font-mono text-[11px] text-slate-500 dark:text-muted">{t.branch || (t.node_id !== local ? "otro nodo" : "")}</span>
              </span>
              {t.pinned_at && <PushPin className="size-3.5 shrink-0 text-amber-500" weight="fill" />}
              <StatusDot status={t.status} />
            </Link>
          ))}
          {filter !== "external" && threads.isSuccess && shown.length === 0 && <p className="p-3 text-sm text-slate-500 dark:text-muted">No hay hilos aquí.</p>}
        </div>
      </aside>
      <section className={`min-h-0 min-w-0 flex-1 ${threadId ? "flex flex-col" : "hidden sm:flex sm:flex-col"}`}><Outlet /></section>
    </div>
  );
}

// ThreadsHome fills the right side when no thread is open: a new chat.
export function ThreadsHome() {
  const navigate = useNavigate();
  return (
    <div className="grid h-full place-items-center p-4">
      <div className="w-full max-w-2xl space-y-3">
        <p className="flex items-center gap-2 text-slate-500 dark:text-muted"><ChatsCircle className="size-5" />Elige un hilo o empieza uno nuevo.</p>
        <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
      </div>
    </div>
  );
}
