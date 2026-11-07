import { ThreadLink } from "../components/thread-link";
import { useState } from "react";
import { MagnifyingGlass } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type T3Status } from "../api/client";
import { Badge, Card, ErrorText, Input, Notice, PageHeader, StatusPill, type Tone } from "../components/ui";
import { useT3 } from "./attention";

type Thread = T3Status["threads"][number];

// state turns a live thread into what the user cares about. A thread missing
// from the snapshot is an older settled one, so it counts as finished.
function state(t: Thread | undefined): { label: string; tone: Tone } {
  if (!t) return { label: "Terminado", tone: "slate" };
  if (t.attention) return { label: "Requiere input", tone: "amber" };
  if (["running", "pending", "starting", "working"].includes(t.status)) return { label: "Trabajando", tone: "blue" };
  if (["failed", "error", "interrupted"].includes(t.status)) return { label: "Error", tone: "red" };
  return { label: "Terminado", tone: "green" };
}

type Job = { key: string; kind: string; title: string; to?: { owner: string; repo: string; number: number; kind: string }; model: string; effort: string; created: string; thread?: Thread; host: string };

// Jobs lists the threads Dory launched in the background with their live state.
export function Jobs() {
  const t3 = useT3();
  const [query, setQuery] = useState("");
  const issues = useQuery({ queryKey: ["issue-threads"], queryFn: api.issueThreads, refetchInterval: 60_000 });
  const items = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 60_000 });
  const live = new Map<string, { thread: Thread; host: string }>();
  for (const s of t3.data ?? []) for (const t of s.threads) live.set(`${s.instance_id}:${t.id}`, { thread: t, host: s.base_url });
  const host = (instance: string) => t3.data?.find((s) => s.instance_id === instance)?.base_url ?? "";
  const jobs: Job[] = [
    ...(issues.data ?? []).map((j) => {
      const l = live.get(`${j.instance_id}:${j.thread_id}`);
      const [owner, repo] = j.repo.split("/");
      return { key: j.id, kind: j.kind === "resolve_conflicts" ? "Resolver conflictos" : "Definir tareas", title: `${j.repo}#${j.number} ${j.title}`, to: { owner, repo, number: j.number, kind: j.kind === "resolve_conflicts" ? "pulls" : "issues" }, model: j.model, effort: j.effort, created: j.created_at, thread: l?.thread, host: l?.host ?? host(j.instance_id) };
    }),
    ...(items.data ?? []).map((j) => {
      const l = live.get(`${j.instance_id}:${j.thread_id}`);
      return { key: j.id, kind: j.role === "refine" ? "Definir item" : j.role === "triage" ? "Analizar item" : j.role === "review" ? "Revisión" : j.role === "replan" ? "Revisar plan" : "Implementar item", title: l?.thread.title ?? j.thread_id, model: l?.thread.model ?? "", effort: "", created: j.created_at, thread: l?.thread, host: l?.host ?? host(j.instance_id) };
    }),
  ].sort((a, b) => b.created.localeCompare(a.created));
  // The search matches title, type, state, model and effort.
  const q = query.trim().toLowerCase();
  const shown = q ? jobs.filter((j) => [j.title, j.kind, state(j.thread).label, j.model, j.effort].join(" ").toLowerCase().includes(q)) : jobs;
  return (
    <div className="space-y-3">
      <PageHeader title="Trabajos" />
      <label className="relative block">
        <MagnifyingGlass aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
        <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar por título, tipo, estado o modelo…" aria-label="Buscar trabajos" className="pl-8" />
      </label>
      <ErrorText error={t3.error ?? issues.error ?? items.error} />
      {(t3.isPending || issues.isPending) && <Notice loading />}
      {jobs.length === 0 && issues.isSuccess && <Notice>Aún no se ha lanzado ningún hilo desde Dory.</Notice>}
      {q && jobs.length > 0 && shown.length === 0 && <Notice>Ningún trabajo coincide con «{query.trim()}».</Notice>}
      {shown.map((j) => {
        const s = state(j.thread);
        return (
          <Card key={j.key} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-3 text-sm">
            <StatusPill tone={s.tone}>{s.label}</StatusPill>
            <Badge>{j.kind}</Badge>
            {j.to ? <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: j.to.owner, repo: j.to.repo, kind: j.to.kind, number: String(j.to.number) }} className="min-w-0 flex-1 truncate font-medium hover:underline">{j.title}</Link>
              : <span className="min-w-0 flex-1 truncate font-medium">{j.title}</span>}
            <span className="text-xs text-slate-500 dark:text-muted">{[j.model, j.effort].filter(Boolean).join(" · ")}</span>
            <span className="text-xs text-slate-500 dark:text-muted">{new Date(j.created).toLocaleString()}</span>
            {j.thread?.url && <ThreadLink url={j.thread.url} environmentId={j.thread.t3_environment_id} threadId={j.thread.id} className="text-xs text-blue-600 hover:underline dark:text-accent-text">Abrir hilo</ThreadLink>}
          </Card>
        );
      })}
    </div>
  );
}
