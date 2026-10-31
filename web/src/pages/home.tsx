import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { api, type GithubPull, type Repository } from "../api/client";
import { Badge, Card, errMsg, field, Label } from "../components/ui";
import { useBlocked, useT3 } from "./attention";
import { PullStatus, useGithubOpen } from "./github";

const LIMIT = 5;
const muted = "text-sm text-slate-500 dark:text-slate-400";

export const homeSearch = (s: Record<string, unknown>): { project?: string } => (typeof s.project === "string" && s.project ? { project: s.project } : {});

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
        <Link to={to} className="flex min-h-11 items-center text-xs underline sm:min-h-0">Ver sección</Link>
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
  const { project } = useSearch({ from: "/" });
  const navigate = useNavigate();
  const t3 = useT3();
  const blocked = useBlocked();
  const github = useGithubOpen();
  const ideas = useQuery({ queryKey: ["items", "inbox"], queryFn: () => api.items.list({ status: "inbox" }) });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const repos = useQuery({ queryKey: ["repositories", project], queryFn: () => api.repositories.list({ project_id: project }), enabled: !!project });
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), enabled: !!project });
  const projectItems = useQuery({ queryKey: ["items", "project", project], queryFn: () => api.items.list({ project_id: project }), enabled: !!project });

  const itemIds = new Set(projectItems.data?.map((i) => i.id));
  const threadKeys = new Set(links.data?.filter((l) => itemIds.has(l.item_id)).map((l) => `${l.instance_id}:${l.thread_id}`));
  const threads = (t3.data ?? [])
    .flatMap((s) => s.threads.map((t) => ({ ...t, instance_id: s.instance_id, environment_id: s.environment_id })))
    .filter((t) => !project || threadKeys.has(`${t.instance_id}:${t.id}`));
  const needing = threads.filter((t) => t.attention);
  const stuck = (blocked.data ?? []).filter((i) => !project || i.project_id === project);
  const recent = (ideas.data ?? [])
    .filter((i) => !project || i.project_id === project)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const pulls = (github.data?.pulls ?? []).filter((p) => !project || belongsTo(p, repos.data ?? []));
  const envName = (id: string, fallback = "") => envs.data?.find((e) => e.id === id)?.name ?? fallback;
  const projectSources: Source[] = project ? [{ name: "los enlaces de agentes", error: links.error ?? projectItems.error }] : [];
  const projectPending = !!project && (links.isPending || projectItems.isPending);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <h1 className="text-xl font-bold">Inicio</h1>
        <div className="w-full sm:w-64">
          <Label text="Proyecto">
            <select className={field} value={project ?? ""} onChange={(e) => navigate({ to: "/", search: e.target.value ? { project: e.target.value } : {} })}>
              <option value="">Todos</option>
              {projects.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </Label>
          {projects.error ? <p className="text-xs text-amber-700 dark:text-amber-400">Sin datos de proyectos: {errMsg(projects.error)}</p> : null}
        </div>
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <Block title={`Atención (${needing.length + stuck.length})`} to="/attention" sources={[{ name: "T3", error: t3.error }, { name: "items bloqueados", error: blocked.error }, ...projectSources]} pending={(t3.isPending && blocked.isPending) || projectPending} empty={needing.length + stuck.length === 0}>
          {needing.slice(0, LIMIT).map((t) => <a key={`${t.instance_id}:${t.id}`} href={t.url} target="_blank" rel="noreferrer" className="block text-sm break-words">{t.title} <Badge>{t.status}</Badge></a>)}
          {stuck.slice(0, LIMIT).map((i) => (
            <div key={i.id} className="text-sm break-words">
              {i.title}
              <p className="text-xs text-red-600 dark:text-red-400">{i.blocked_reason}</p>
            </div>
          ))}
        </Block>
        <Block title={`Agentes activos (${threads.length})`} to="/attention" sources={[{ name: "T3", error: t3.error }, ...projectSources]} pending={t3.isPending || projectPending} empty={threads.length === 0}>
          {threads.slice(0, LIMIT).map((t) => (
            <a key={`${t.instance_id}:${t.id}`} href={t.url} target="_blank" rel="noreferrer" className="block space-y-1 text-sm">
              <div className="break-words">{t.title}</div>
              <div className="flex flex-wrap gap-1"><Badge>{envName(t.environment_id, "Entorno")}</Badge><Badge>{t.status}</Badge><Badge>{t.model}</Badge></div>
            </a>
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
          {recent.slice(0, LIMIT).map((i) => <div key={i.id} className="text-sm break-words">{i.title}</div>)}
        </Block>
        <Block title={`Pull requests abiertas (${pulls.length})`} to="/github" sources={[{ name: "GitHub", error: github.error }, ...(project ? [{ name: "repositorios", error: repos.error }] : [])]} pending={github.isPending || (!!project && repos.isPending)} empty={pulls.length === 0}>
          {pulls.slice(0, LIMIT).map((p) => {
            const [owner, repo] = p.repo.split("/");
            return (
              <Link key={p.url} to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} className="block space-y-1 text-sm">
                <div className="break-words">{p.title}</div>
                <div className="text-xs text-slate-500 dark:text-slate-400">{p.repo}#{p.number}</div>
                <PullStatus pull={p} />
              </Link>
            );
          })}
        </Block>
      </div>
    </div>
  );
}
