import { ThreadLink } from "../components/thread-link";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type T3Status } from "../api/client";
import { Badge, Card, ErrorText } from "../components/ui";
import { useT3 } from "./attention";

type Thread = T3Status["threads"][number];

// state turns a live thread into what the user cares about. A thread missing
// from the snapshot is an older settled one, so it counts as finished.
function state(t: Thread | undefined): { label: string; tone: string } {
  if (!t) return { label: "Terminado", tone: "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300" };
  if (t.attention) return { label: "Requiere input", tone: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300" };
  if (["running", "pending", "starting", "working"].includes(t.status)) return { label: "Trabajando", tone: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300" };
  if (["failed", "error", "interrupted"].includes(t.status)) return { label: "Error", tone: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300" };
  return { label: "Terminado", tone: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300" };
}

type Job = { key: string; kind: string; title: string; to?: { owner: string; repo: string; number: number }; model: string; effort: string; created: string; thread?: Thread; host: string };

// Jobs lists the threads Dory launched in the background with their live state.
export function Jobs() {
  const t3 = useT3();
  const issues = useQuery({ queryKey: ["issue-threads"], queryFn: api.issueThreads, refetchInterval: 10_000 });
  const items = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 30_000 });
  const live = new Map<string, { thread: Thread; host: string }>();
  for (const s of t3.data ?? []) for (const t of s.threads) live.set(`${s.instance_id}:${t.id}`, { thread: t, host: s.base_url });
  const host = (instance: string) => t3.data?.find((s) => s.instance_id === instance)?.base_url ?? "";
  const jobs: Job[] = [
    ...(issues.data ?? []).map((j) => {
      const l = live.get(`${j.instance_id}:${j.thread_id}`);
      const [owner, repo] = j.repo.split("/");
      return { key: j.id, kind: "Definir tareas", title: `${j.repo}#${j.number} ${j.title}`, to: { owner, repo, number: j.number }, model: j.model, effort: j.effort, created: j.created_at, thread: l?.thread, host: l?.host ?? host(j.instance_id) };
    }),
    ...(items.data ?? []).map((j) => {
      const l = live.get(`${j.instance_id}:${j.thread_id}`);
      return { key: j.id, kind: j.role === "triage" ? "Analizar item" : j.role === "review" ? "Revisión" : "Implementar item", title: l?.thread.title ?? j.thread_id, model: l?.thread.model ?? "", effort: "", created: j.created_at, thread: l?.thread, host: l?.host ?? host(j.instance_id) };
    }),
  ].sort((a, b) => b.created.localeCompare(a.created));
  return (
    <div className="space-y-3">
      <h1 className="text-lg font-semibold">Trabajos</h1>
      <ErrorText error={t3.error ?? issues.error ?? items.error} />
      {jobs.length === 0 && issues.isSuccess && <p className="text-sm text-slate-500">Aún no se ha lanzado ningún hilo desde Dory.</p>}
      {jobs.map((j) => {
        const s = state(j.thread);
        return (
          <Card key={j.key} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-3 text-sm">
            <span className={`rounded px-2 py-0.5 text-xs font-medium ${s.tone}`}>{s.label}</span>
            <Badge>{j.kind}</Badge>
            {j.to ? <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: j.to.owner, repo: j.to.repo, kind: "issues", number: String(j.to.number) }} className="min-w-0 flex-1 truncate font-medium hover:underline">{j.title}</Link>
              : <span className="min-w-0 flex-1 truncate font-medium">{j.title}</span>}
            <span className="text-xs text-slate-500 dark:text-slate-400">{[j.model, j.effort].filter(Boolean).join(" · ")}</span>
            <span className="text-xs text-slate-400">{new Date(j.created).toLocaleString()}</span>
            {j.thread?.url && <ThreadLink url={j.thread.url} environmentId={j.thread.t3_environment_id} threadId={j.thread.id} className="text-xs text-blue-600 hover:underline dark:text-blue-400">Abrir en T3</ThreadLink>}
          </Card>
        );
      })}
    </div>
  );
}
