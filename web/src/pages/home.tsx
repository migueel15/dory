import { ItemTitle, pendingPoll } from "../components/item-title";
import { ThreadLink } from "../components/thread-link";
import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type GithubPull, type Repository } from "../api/client";
import { Arrow, Badge, Card, PageHeader, errMsg, pill } from "../components/ui";
import { useProject, useProjectThreads } from "../project";
import { IdeaCapture } from "../components/capture";
import { ThreadStatus } from "../components/thread-status";
import { useBlocked, useT3, useValidating } from "./attention";
import { PullStatus, useGithubOpen } from "./github";

const LIMIT = 5;
const muted = "text-sm text-slate-500 dark:text-muted";


type Source = { name: string; error: unknown };
type Section = "/attention" | "/github" | "/inbox" | "/environments";

// Block renders whatever its sources returned and names the ones that failed,
// so one source being down never hides the rest of the home page.
function Block({ title, to, sources, pending, empty, children }: { title: string; to: Section; sources: Source[]; pending: boolean; empty: boolean; children: ReactNode }) {
  const failed = sources.filter((s) => s.error);
  return (
    <Card className="min-w-0 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-semibold">{title}</h2>
        <Link to={to} className={pill}>Ver sección <Arrow /></Link>
      </div>
      {failed.map((s) => <p key={s.name} className="text-sm text-amber-700 dark:text-amber-400 break-words">Sin datos de {s.name}: {errMsg(s.error)}</p>)}
      {pending ? <p className={muted}>Cargando…</p> : empty ? failed.length === 0 && <p className={muted}>Nada que mostrar.</p> : children}
    </Card>
  );
}

const belongsTo = (pull: GithubPull, repos: Repository[]) =>
  repos.some((r) => {
    const remote = r.remote_url.toLowerCase().replace(/\.git$/, "");
    const repo = pull.repo.toLowerCase();
    return remote.endsWith(`/${repo}`) || remote.endsWith(`:${repo}`);
  });

export function Home() {
  const project = useProject();
  const t3 = useT3();
  const blocked = useBlocked();
  const validating = useValidating();
  const github = useGithubOpen();
  const ideas = useQuery({ queryKey: ["items", "inbox"], queryFn: () => api.items.list({ status: "inbox" }), refetchInterval: pendingPoll });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const repos = useQuery({ queryKey: ["repositories", project], queryFn: () => api.repositories.list({ project_id: project }), enabled: !!project });
  const linked = useProjectThreads(project);
  const threads = (t3.data ?? [])
    .flatMap((s) => s.threads.map((t) => ({ ...t, instance_id: s.instance_id, environment_id: s.environment_id })))
    .filter((t) => !linked.keys || linked.keys.has(`${t.instance_id}:${t.id}`));
  const needing = threads.filter((t) => t.attention);
  const stuck = (blocked.data ?? []).filter((i) => !project || i.project_id === project);
  const recent = (ideas.data ?? [])
    .filter((i) => !project || i.project_id === project)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const toValidate = (validating.data ?? []).filter((i) => !project || i.project_id === project);
  const pulls = (github.data?.pulls ?? []).filter((p) => !project || belongsTo(p, repos.data ?? []));
  const envName = (id: string, fallback = "") => envs.data?.find((e) => e.id === id)?.name ?? fallback;
  const projectSources: Source[] = project ? [{ name: "los enlaces de agentes", error: linked.error }] : [];
  const projectPending = linked.pending;

  return (
    <div className="space-y-3">
      <PageHeader title="Inicio" />
      <IdeaCapture focusOnSlash />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[
          { to: "/attention", n: needing.length, label: "Agentes te necesitan", warn: needing.length > 0 },
          { to: "/attention", n: stuck.length, label: "Items bloqueados", warn: stuck.length > 0 },
          { to: "/attention", n: toValidate.length, label: "Esperan tu validación", warn: false },
          { to: "/github", n: pulls.length, label: "Pull requests abiertas", warn: false },
        ].map((k) => (
          <Link key={k.label} to={k.to as Section} className={`rounded-lg border bg-white p-3 shadow-sm hover:bg-slate-50 dark:bg-surface dark:hover:bg-raised ${k.warn ? "border-amber-400 dark:border-amber-700" : "border-slate-200 dark:border-border"}`}>
            <div className={`text-2xl font-bold ${k.warn ? "text-amber-600 dark:text-amber-400" : ""}`}>{k.n}</div>
            <div className={muted}>{k.label}</div>
          </Link>
        ))}
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <Block title={`Atención (${needing.length + stuck.length})`} to="/attention" sources={[{ name: "T3", error: t3.error }, { name: "items bloqueados", error: blocked.error }, ...projectSources]} pending={(t3.isPending && blocked.isPending) || projectPending} empty={needing.length + stuck.length === 0}>
          {needing.slice(0, LIMIT).map((t) => <ThreadLink key={`${t.instance_id}:${t.id}`} url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className="block text-sm break-words">{t.title} <ThreadStatus status={t.status} attention={t.attention} /></ThreadLink>)}
          {stuck.slice(0, LIMIT).map((i) => (
            <div key={i.id} className="text-sm break-words">
              {i.title}
              <p className="text-xs text-red-600 dark:text-red-400">{i.blocked_reason}</p>
            </div>
          ))}
        </Block>
        <Block title={`Agentes activos (${threads.length})`} to="/attention" sources={[{ name: "T3", error: t3.error }, ...projectSources]} pending={t3.isPending || projectPending} empty={threads.length === 0}>
          {threads.slice(0, LIMIT).map((t) => (
            <ThreadLink key={`${t.instance_id}:${t.id}`} url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className="block space-y-1 text-sm">
              <div className="break-words">{t.title}</div>
              <div className="flex flex-wrap gap-1"><ThreadStatus status={t.status} attention={t.attention} /><Badge>{envName(t.environment_id, "Entorno")}</Badge><Badge>{t.model}</Badge></div>
            </ThreadLink>
          ))}
        </Block>
        <Block title="Máquinas" to="/environments" sources={[{ name: "T3", error: t3.error }, { name: "entornos", error: envs.error }]} pending={t3.isPending} empty={(t3.data ?? []).length === 0}>
          {t3.data?.map((s) => (
            <div key={s.instance_id} className="flex items-center justify-between gap-2 text-sm">
              <span className="min-w-0 break-words">{envName(s.environment_id, s.base_url)}</span>
              <span className={s.state === "available" ? "text-green-700 dark:text-green-400" : "text-red-600 dark:text-red-400"}>{s.state === "available" ? "Disponible" : "No disponible"}</span>
            </div>
          ))}
        </Block>
        <Block title={`Ideas recientes (${recent.length})`} to="/inbox" sources={[{ name: "ideas", error: ideas.error }]} pending={ideas.isPending} empty={recent.length === 0}>
          {recent.slice(0, LIMIT).map((i) => <div key={i.id} className="text-sm"><ItemTitle item={i} /></div>)}
        </Block>
        <Block title={`Pull requests abiertas (${pulls.length})`} to="/github" sources={[{ name: "GitHub", error: github.error }, ...(project ? [{ name: "repositorios", error: repos.error }] : [])]} pending={github.isPending || (!!project && repos.isPending)} empty={pulls.length === 0}>
          {pulls.slice(0, LIMIT).map((p) => {
            const [owner, repo] = p.repo.split("/");
            return (
              <Link key={p.url} to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} className="block space-y-1 text-sm">
                <div className="break-words">{p.title}</div>
                <div className="text-xs text-slate-500 dark:text-muted">{p.repo}#{p.number}</div>
                <PullStatus pull={p} />
              </Link>
            );
          })}
        </Block>
      </div>
    </div>
  );
}
