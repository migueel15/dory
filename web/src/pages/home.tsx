import { ItemTitle, pendingPoll } from "../components/item-title";
import { ThreadLink } from "../components/thread-link";
import { useState, type ReactNode } from "react";
import { ItemForm } from "./items";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type GithubPull, type Item, type Repository } from "../api/client";
import { Arrow, Badge, Card, PageHeader, errMsg } from "../components/ui";
import { BellRinging, Flask, GitPullRequest, HardDrives, Lightning, Prohibit, Robot, Tray, type Icon as PhIcon } from "@phosphor-icons/react";
import { useProject, useProjectThreads } from "../project";
import { IdeaCapture } from "../components/capture";
import { ThreadStatus } from "../components/thread-status";
import { useBlocked, useT3, useValidating } from "./attention";
import { PullStatus, useGithubOpen } from "./github";

const LIMIT = 5;
const muted = "text-sm text-slate-500 dark:text-muted";


type Source = { name: string; error: unknown };
type Section = "/attention" | "/github" | "/inbox" | "/environments" | "/tasks" | "/jobs";

// Block is one home panel: icon, title, count, a link to its section and the
// sources that failed, so one source being down never hides the rest.
function Block({ icon: Icon, title, count, to, sources, pending, empty, emptyText = "Nada que mostrar.", className = "", children }: { icon: PhIcon; title: string; count?: number; to: Section; sources: Source[]; pending: boolean; empty: boolean; emptyText?: string; className?: string; children: ReactNode }) {
  const failed = sources.filter((s) => s.error);
  return (
    <Card className={`min-w-0 space-y-3 ${className}`}>
      <div className="flex items-center gap-2">
        <Icon weight="duotone" aria-hidden="true" className="size-5 shrink-0 text-slate-500 dark:text-muted" />
        <h2 className="font-semibold">{title}</h2>
        {count !== undefined && <span className="rounded-md bg-slate-100 px-1.5 text-xs font-medium tabular-nums text-slate-600 dark:bg-raised dark:text-text-secondary">{count}</span>}
        <Link to={to} className="ml-auto inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">Ver todo <Arrow /></Link>
      </div>
      {failed.map((s) => <p key={s.name} className="break-words text-xs text-amber-700 dark:text-amber-400">Sin datos de {s.name}: {errMsg(s.error)}</p>)}
      {pending ? <div className="space-y-2">{[0, 1, 2].map((i) => <div key={i} className="h-9 rounded-md bg-slate-100 motion-safe:animate-pulse dark:bg-raised" />)}</div>
        : empty ? failed.length === 0 && <p className={muted}>{emptyText}</p> : <ul className="-mx-1 divide-y divide-slate-100 dark:divide-border/60">{children}</ul>}
    </Card>
  );
}

const row = "flex items-start gap-2.5 rounded-md px-1 py-2 hover:bg-slate-50 dark:hover:bg-raised";

const belongsTo = (pull: GithubPull, repos: Repository[]) =>
  repos.some((r) => {
    const remote = r.remote_url.toLowerCase().replace(/\.git$/, "");
    const repo = pull.repo.toLowerCase();
    return remote.endsWith(`/${repo}`) || remote.endsWith(`:${repo}`);
  });

export function Home() {
  const project = useProject();
  const [openItem, setOpenItem] = useState<Item | null>(null);
  const t3 = useT3();
  const blocked = useBlocked();
  const validating = useValidating();
  const github = useGithubOpen();
  const ideas = useQuery({ queryKey: ["items", "inbox"], queryFn: () => api.items.list({ status: "inbox" }), refetchInterval: pendingPoll });
  const working = useQuery({ queryKey: ["items", "in_progress"], queryFn: () => api.items.list({ status: "in_progress" }) });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const repos = useQuery({ queryKey: ["repositories", project], queryFn: () => api.repositories.list({ project_id: project }), enabled: !!project });
  const linked = useProjectThreads(project);
  const mine = <T extends { project_id: string | null }>(l: T[] | undefined) => (l ?? []).filter((i) => !project || i.project_id === project);
  const threads = (t3.data ?? [])
    .flatMap((s) => s.threads.map((t) => ({ ...t, instance_id: s.instance_id, environment_id: s.environment_id })))
    .filter((t) => !linked.keys || linked.keys.has(`${t.instance_id}:${t.id}`));
  const needing = threads.filter((t) => t.attention);
  const busy = threads.filter((t) => !t.attention && ["running", "pending", "starting", "working"].includes(t.status));
  const stuck = mine(blocked.data);
  const toValidate = mine(validating.data);
  const inProgress = mine(working.data).sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const recent = mine(ideas.data).sort((a, b) => b.created_at.localeCompare(a.created_at));
  const pulls = (github.data?.pulls ?? []).filter((p) => !project || belongsTo(p, repos.data ?? []));
  const envName = (id: string, fallback = "") => envs.data?.find((e) => e.id === id)?.name ?? fallback;
  const projectName = (id: string | null) => (id ? projects.data?.find((p) => p.id === id)?.name : undefined);
  const projectSources: Source[] = project ? [{ name: "los enlaces de agentes", error: linked.error }] : [];
  const actions = needing.length + stuck.length + toValidate.length;
  const machines = [...(t3.data ?? [])].sort((a, b) => envName(a.environment_id, a.base_url).localeCompare(envName(b.environment_id, b.base_url), "es", { sensitivity: "base" }));
  const meta = "text-xs text-slate-500 dark:text-muted";

  const tiles: { to: Section; n: number; label: string; icon: PhIcon; tone: string }[] = [
    { to: "/attention", n: needing.length, label: "Agentes te necesitan", icon: BellRinging, tone: "text-amber-600 dark:text-amber-400" },
    { to: "/attention", n: stuck.length, label: "Items bloqueados", icon: Prohibit, tone: "text-red-600 dark:text-red-400" },
    { to: "/attention", n: toValidate.length, label: "Por validar", icon: Flask, tone: "text-violet-600 dark:text-violet-400" },
    { to: "/tasks", n: inProgress.length, label: "En curso", icon: Lightning, tone: "text-blue-600 dark:text-accent-text" },
  ];

  return (
    <div className="space-y-4">
      <PageHeader title="Inicio" subtitle={projectName(project)} />
      <IdeaCapture focusOnSlash />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {tiles.map((k) => (
          <Link key={k.label} to={k.to} className="group flex items-center gap-3 rounded-lg border border-slate-200 bg-white p-3 transition-colors hover:border-slate-300 dark:border-border dark:bg-surface dark:hover:border-border-strong">
            <span className={`grid size-10 shrink-0 place-items-center rounded-lg bg-slate-100 dark:bg-raised ${k.n > 0 ? k.tone : "text-slate-400 dark:text-muted"}`}><k.icon weight="duotone" aria-hidden="true" className="size-5" /></span>
            <span className="min-w-0">
              <span className={`block text-2xl font-bold leading-tight tabular-nums ${k.n > 0 ? "" : "text-slate-400 dark:text-muted"}`}>{k.n}</span>
              <span className="block truncate text-xs text-slate-500 dark:text-muted">{k.label}</span>
            </span>
          </Link>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Block icon={BellRinging} title="Requiere tu acción" count={actions} to="/attention" className="lg:col-span-2" emptyText="Todo al día: nada espera una decisión tuya."
          sources={[{ name: "T3", error: t3.error }, { name: "items bloqueados", error: blocked.error }, { name: "items por validar", error: validating.error }, ...projectSources]}
          pending={(t3.isPending && blocked.isPending) || linked.pending} empty={actions === 0}>
          {needing.slice(0, LIMIT).map((t) => (
            <li key={`${t.instance_id}:${t.id}`}><ThreadLink url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className={row}>
              <Robot weight="duotone" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-amber-500" />
              <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{t.title}</span><span className={meta}>Hilo · {envName(t.environment_id, "Entorno")} · {t.model}</span></span>
              <ThreadStatus status={t.status} attention={t.attention} />
            </ThreadLink></li>
          ))}
          {stuck.slice(0, LIMIT).map((i) => (
            <li key={i.id}><Link to="/tasks" className={row}>
              <Prohibit weight="duotone" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-red-500" />
              <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium"><ItemTitle item={i} /></span><span className="line-clamp-1 text-xs text-red-600 dark:text-red-400">{i.blocked_reason || "Bloqueado sin motivo indicado"}</span></span>
              {projectName(i.project_id) && <Badge>{projectName(i.project_id)}</Badge>}
            </Link></li>
          ))}
          {toValidate.slice(0, LIMIT).map((i) => (
            <li key={i.id}><Link to="/tasks" className={row}>
              <Flask weight="duotone" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-violet-500" />
              <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium"><ItemTitle item={i} /></span><span className={meta}>Terminado, espera tu prueba</span></span>
              {projectName(i.project_id) && <Badge>{projectName(i.project_id)}</Badge>}
            </Link></li>
          ))}
        </Block>

        <Block icon={Robot} title="Agentes trabajando" count={busy.length} to="/jobs" emptyText="Ningún agente trabajando ahora."
          sources={[{ name: "T3", error: t3.error }, ...projectSources]} pending={t3.isPending || linked.pending} empty={busy.length === 0}>
          {busy.slice(0, LIMIT).map((t) => (
            <li key={`${t.instance_id}:${t.id}`}><ThreadLink url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className={row}>
              <span className="mt-1.5 size-2 shrink-0 rounded-full bg-blue-500 motion-safe:animate-pulse" />
              <span className="min-w-0 flex-1"><span className="block truncate text-sm">{t.title}</span><span className={meta}>{envName(t.environment_id, "Entorno")} · {t.model}</span></span>
            </ThreadLink></li>
          ))}
        </Block>

        <Block icon={Lightning} title="En curso" count={inProgress.length} to="/tasks" className="lg:col-span-2" emptyText="Ningún item en curso."
          sources={[{ name: "items en curso", error: working.error }]} pending={working.isPending} empty={inProgress.length === 0}>
          {inProgress.slice(0, LIMIT).map((i) => (
            <li key={i.id}><Link to="/tasks" className={row}>
              <Lightning weight="duotone" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-blue-500" />
              <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium"><ItemTitle item={i} /></span>{i.progress && <span className="line-clamp-1 text-xs text-slate-500 dark:text-muted">{i.progress}</span>}</span>
              {projectName(i.project_id) && <Badge>{projectName(i.project_id)}</Badge>}
            </Link></li>
          ))}
        </Block>

        <Block icon={GitPullRequest} title="Pull requests" count={pulls.length} to="/github" emptyText="Sin pull requests abiertas."
          sources={[{ name: "GitHub", error: github.error }, ...(project ? [{ name: "repositorios", error: repos.error }] : [])]} pending={github.isPending || (!!project && repos.isPending)} empty={pulls.length === 0}>
          {pulls.slice(0, LIMIT).map((p) => {
            const [owner, repo] = p.repo.split("/");
            return (
              <li key={p.url}><Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} className={`${row} flex-col gap-1`}>
                <span className="block w-full truncate text-sm">{p.title}</span>
                <span className="flex w-full items-center justify-between gap-2"><span className={meta}>{p.repo}#{p.number}</span><PullStatus pull={p} /></span>
              </Link></li>
            );
          })}
        </Block>

        <Block icon={Tray} title="Ideas recientes" count={recent.length} to="/inbox" className="lg:col-span-2" emptyText="Inbox vacío."
          sources={[{ name: "ideas", error: ideas.error }]} pending={ideas.isPending} empty={recent.length === 0}>
          {recent.slice(0, LIMIT).map((i) => (
            <li key={i.id}><button type="button" onClick={() => setOpenItem(i)} className={`${row} w-full text-left`}>
              <Tray weight="duotone" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-slate-400" />
              <span className="min-w-0 flex-1 truncate text-sm"><ItemTitle item={i} /></span>
              <span className={`${meta} shrink-0`}>{new Date(i.created_at).toLocaleDateString("es-ES", { day: "numeric", month: "short" })}</span>
            </button></li>
          ))}
        </Block>

        <Block icon={HardDrives} title="Entornos" count={machines.filter((s) => s.state === "available").length} to="/environments"
          sources={[{ name: "T3", error: t3.error }, { name: "entornos", error: envs.error }]} pending={t3.isPending} empty={machines.length === 0}>
          {machines.map((s) => {
            const up = s.state === "available";
            const n = s.threads.filter((t) => t.attention).length;
            return (
              <li key={s.instance_id} className="flex items-start gap-2.5 px-1 py-2">
                <span className={`mt-1.5 size-2 shrink-0 rounded-full ${up ? "bg-emerald-500" : "bg-slate-400"}`} />
                <span className="min-w-0 flex-1"><span className="block truncate text-sm">{envName(s.environment_id, s.base_url)}</span><span className={meta}>{up ? `${s.threads.length} hilos${n ? ` · ${n} te necesitan` : ""}` : "No disponible"}</span></span>
              </li>
            );
          })}
        </Block>
      </div>
      {openItem && <ItemForm item={openItem} onClose={() => setOpenItem(null)} />}
    </div>
  );
}
