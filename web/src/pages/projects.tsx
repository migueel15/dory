import { useEffect, useId, useState } from "react";
import { CheckCircle, Clock, Flask, GitBranch, GithubLogo, Info, Lightning, PencilSimple, Prohibit, Tray } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api } from "../api/client";
import { Badge, Button, Card, ErrorText, Md, PageHeader, Notice, StatusPill, type Tone } from "../components/ui";
import { Features, ProjectForm } from "./crud";
import { Board } from "./items";
import { setProject, useProject } from "../project";

// Projects always lists every project; the selected one is marked as the active filter.
export function Projects() {
  return <ProjectList />;
}

// Project avatars take a fixed categorical hue from the slug, so a project keeps its color.
const HUES = ["bg-[#2a78d6]", "bg-[#eb6834]", "bg-[#1baf7a]", "bg-[#eda100]", "bg-[#e87ba4]", "bg-[#008300]", "bg-[#6250d6]", "bg-[#e34948]"];
const hue = (key: string) => HUES[[...key].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7) % HUES.length];
const ago = (iso: string) => {
  const d = Math.round((Date.now() - new Date(iso).getTime()) / 86_400_000);
  return d <= 0 ? "hoy" : d === 1 ? "ayer" : d < 30 ? `hace ${d} días` : new Date(iso).toLocaleDateString("es-ES", { day: "numeric", month: "short" });
};
const OPEN: { status: string; label: string; tone: Tone; icon: typeof Lightning }[] = [
  { status: "in_progress", label: "En curso", tone: "amber", icon: Lightning },
  { status: "validating", label: "Validando", tone: "violet", icon: Flask },
  { status: "blocked", label: "Bloqueados", tone: "red", icon: Prohibit },
  { status: "ready", label: "Listos", tone: "indigo", icon: CheckCircle },
];

function ProjectList() {
  const [creating, setCreating] = useState(false);
  const active = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const items = useQuery({ queryKey: ["items", "all"], queryFn: () => api.items.list() });
  const repos = useQuery({ queryKey: ["repositories", "all"], queryFn: () => api.repositories.list() });
  return (
    <div className="space-y-3">
      <PageHeader title="Proyectos" subtitle={projects.data ? `${projects.data.length}` : undefined}>
        <Button onClick={() => setCreating(true)}>Nuevo</Button>
      </PageHeader>
      <ErrorText error={projects.error ?? items.error ?? repos.error} />
      {projects.isPending && <Notice loading />}
      {projects.data?.length === 0 && <Notice>Aún no hay proyectos. Crea el primero con «Nuevo».</Notice>}
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {projects.data?.map((p) => {
          const mine = (items.data ?? []).filter((i) => i.project_id === p.id && !i.archived_at);
          const count = (st: string) => mine.filter((i) => i.status === st).length;
          const done = count("done"), cancelled = count("cancelled");
          const total = mine.length - cancelled;
          const inbox = count("inbox") + count("triaged");
          const last = [p.updated_at, ...mine.map((i) => i.updated_at)].sort().at(-1)!;
          const nRepos = (repos.data ?? []).filter((r) => r.project_id === p.id).length;
          return (
            <Link key={p.id} to="/tasks" onClick={() => setProject(p.id)} className="group block">
              <Card className={`flex h-full flex-col gap-3 transition-colors group-hover:border-slate-300 dark:group-hover:border-border-strong ${p.id === active ? "ring-1 ring-slate-900/20 dark:ring-white/20" : ""}`}>
                <div className="flex items-start gap-3">
                  <span className={`grid size-10 shrink-0 place-items-center rounded-lg text-base font-bold text-white ${hue(p.slug)}`}>{p.name[0]?.toUpperCase()}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-semibold">{p.name}</span>
                      {p.status !== "active" && <StatusPill tone="slate">{p.status}</StatusPill>}
                    </div>
                    <div className="truncate font-mono text-xs text-slate-500 dark:text-muted">{p.slug}</div>
                  </div>
                  {p.id === active && <span title="Proyecto del filtro global" className="shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] font-medium text-slate-600 dark:bg-raised dark:text-text-secondary">Filtro</span>}
                </div>
                <p className="line-clamp-2 min-h-[2.5rem] text-sm text-slate-600 dark:text-text-secondary">{p.description || <span className="italic text-slate-400 dark:text-muted">Sin descripción</span>}</p>
                <div className="flex flex-wrap gap-1.5">
                  {OPEN.map(({ status, label, tone, icon: Icon }) => { const n = count(status); return n > 0 && <StatusPill key={status} tone={tone}><span className="inline-flex items-center gap-1"><Icon weight="bold" aria-hidden="true" className="size-3" />{n} {label.toLowerCase()}</span></StatusPill>; })}
                  {inbox > 0 && <StatusPill tone="slate"><span className="inline-flex items-center gap-1"><Tray weight="bold" aria-hidden="true" className="size-3" />{inbox} por definir</span></StatusPill>}
                  {OPEN.every(({ status }) => count(status) === 0) && inbox === 0 && <span className="text-xs text-slate-400 dark:text-muted">Sin trabajo abierto</span>}
                </div>
                <div className="mt-auto space-y-2">
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-slate-500 dark:text-muted"><span>Completado</span><span className="tabular-nums">{done}/{total}</span></div>
                    <div className="h-1.5 overflow-hidden rounded bg-slate-200 dark:bg-hover"><div className="h-full rounded bg-emerald-600" style={{ width: `${total ? (done / total) * 100 : 0}%` }} /></div>
                  </div>
                  <div className="flex items-center gap-3 border-t border-slate-100 pt-2 text-xs text-slate-500 dark:border-border dark:text-muted">
                    <span className="inline-flex items-center gap-1" title="Repositorios"><GitBranch aria-hidden="true" className="size-3.5" />{nRepos}</span>
                    {p.stack.length > 0 && <span className="min-w-0 flex-1 truncate" title={p.stack.join(", ")}>{p.stack.join(" · ")}</span>}
                    <span className="ml-auto shrink-0 inline-flex items-center gap-1" title={new Date(last).toLocaleString()}><Clock aria-hidden="true" className="size-3.5" />{ago(last)}</span>
                  </div>
                </div>
              </Card>
            </Link>
          );
        })}
      </div>
      {creating && <ProjectForm onClose={() => setCreating(false)} />}
    </div>
  );
}

// ProjectDetail keeps old links working: it selects the project globally and opens
// Tareas, the one kanban view, instead of a second board.
export function ProjectDetail() {
  const { id } = useParams({ from: "/projects/$id" });
  const navigate = useNavigate();
  useEffect(() => {
    setProject(id);
    navigate({ to: "/tasks", replace: true });
  }, [id, navigate]);
  return null;
}

// ProjectHeader is the title of Tareas when a project is selected: name, repositories, info and edit.
function ProjectHeader({ id }: { id: string }) {
  const [editing, setEditing] = useState(false);
  const infoId = useId();
  const project = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list(), select: (l) => l.find((p) => p.id === id) });
  const repos = useQuery({ queryKey: ["repositories", id], queryFn: () => api.repositories.list({ project_id: id }) });
  const p = project.data;
  if (!p) return <ErrorText error={project.error} />;
  return (
    <div className="flex min-w-0 items-center gap-1">
      <h1 className="min-w-0 flex-1 truncate text-xl font-bold" title={p.name}>{p.name}</h1>
      <div className="flex max-w-[35%] min-w-0 items-center overflow-x-auto">
        {repos.data?.map((r) => (
          <a key={r.id} href={r.remote_url.replace(/\.git$/, "")} target="_blank" rel="noopener noreferrer"
            aria-label={`Abrir repositorio ${r.remote_url}`} title={r.remote_url.replace(/\.git$/, "")}
            className="grid min-h-11 min-w-11 shrink-0 place-items-center rounded-md hover:bg-slate-100 dark:hover:bg-raised sm:min-h-0 sm:min-w-0 sm:p-2">
            <GithubLogo aria-hidden="true" className="size-5" />
          </a>
        ))}
      </div>
      <Button variant="bare" className="shrink-0 px-2" aria-label="Información del proyecto" title={p.description || "Información del proyecto"} popoverTarget={infoId}>
        <Info aria-hidden="true" className="size-5" />
      </Button>
      <div id={infoId} popover="auto" className="fixed inset-0 m-auto max-h-[80dvh] w-80 max-w-[calc(100vw-2rem)] space-y-3 overflow-auto rounded-lg border border-slate-200 bg-white p-4 text-slate-900 shadow-lg dark:border-border dark:bg-surface dark:text-text">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-semibold">Información del proyecto</h2>
          <Button variant="bare" popoverTarget={infoId} popoverTargetAction="hide" aria-label="Cerrar información">×</Button>
        </div>
        {p.description && <Md>{p.description}</Md>}
        <p className="break-words text-sm">{p.slug}</p>
        <div className="flex flex-wrap gap-1"><Badge>{p.status}</Badge>{p.stack.map((s) => <Badge key={s}>{s}</Badge>)}</div>
        {repos.data?.map((r) => <p key={r.id} className="break-all text-sm">{r.remote_url} ({r.default_branch})</p>)}
      </div>
      <Button variant="bare" className="shrink-0 px-2" onClick={() => setEditing(true)} aria-label="Editar proyecto" title="Editar proyecto">
        <PencilSimple aria-hidden="true" className="size-5" />
      </Button>
      {editing && <ProjectForm project={p} onClose={() => setEditing(false)} />}
    </div>
  );
}

// Tasks is the single kanban view: every task, or the project chosen in the global filter.
export function Tasks() {
  const project = useProject();
  return (
    <div className="space-y-6">
      {project ? <ProjectHeader id={project} /> : <PageHeader title="Tareas" />}
      <Board projectId={project} />
      {project && <Features projectId={project} />}
    </div>
  );
}
