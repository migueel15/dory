import { useId, useState } from "react";
import { ArrowLeft, GithubLogo, Info, PencilSimple } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api } from "../api/client";
import { Badge, Button, Card, ErrorText, Md, PageHeader, Notice } from "../components/ui";
import { Features, ProjectForm } from "./crud";
import { Board } from "./items";
import { useProject } from "../project";

// Projects always lists every project; the selected one is marked as the active filter.
export function Projects() {
  return <ProjectList />;
}

function ProjectList() {
  const [creating, setCreating] = useState(false);
  const active = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  return (
    <div className="space-y-3">
      <PageHeader title="Proyectos">
        <Button onClick={() => setCreating(true)}>Nuevo</Button>
      </PageHeader>
      <ErrorText error={projects.error} />
      {projects.isPending && <Notice loading />}
      {projects.data?.length === 0 && <Notice>Aún no hay proyectos. Crea el primero con «Nuevo».</Notice>}
      <div className="grid gap-3 md:grid-cols-2">
        {projects.data?.map((p) => (
          <Link key={p.id} to="/projects/$id" params={{ id: p.id }}>
            <Card className="hover:bg-slate-50 dark:hover:bg-raised">
              <div className="font-medium">{p.name} <span className="text-xs text-slate-500 dark:text-muted">{p.slug}</span></div>
              <p className="text-sm text-slate-600 dark:text-text-secondary">{p.description}</p>
              <div className="mt-1 flex flex-wrap gap-1"><Badge>{p.status}</Badge>{p.id === active && <Badge>Filtro activo</Badge>}{p.stack.map((s) => <Badge key={s}>{s}</Badge>)}</div>
            </Card>
          </Link>
        ))}
      </div>
      {creating && <ProjectForm onClose={() => setCreating(false)} />}
    </div>
  );
}

export function ProjectDetail() {
  const { id } = useParams({ from: "/projects/$id" });
  return <ProjectView id={id} />;
}

function ProjectView({ id }: { id: string }) {
  const [editing, setEditing] = useState(false);
  const infoId = useId();
  const project = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list(), select: (l) => l.find((p) => p.id === id) });
  const repos = useQuery({ queryKey: ["repositories", id], queryFn: () => api.repositories.list({ project_id: id }) });
  const p = project.data;
  if (!p) return <ErrorText error={project.error} />;
  return (
    <div className="space-y-3">
      <div className="flex min-w-0 items-center gap-1">
        <Link to="/projects" aria-label="Volver a proyectos" title="Volver a proyectos" className="grid min-h-11 min-w-11 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised sm:min-h-0 sm:min-w-0 sm:p-2">
          <ArrowLeft aria-hidden="true" className="size-5" />
        </Link>
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
      </div>
      <Board projectId={id} />
      <Features projectId={id} />
      {editing && <ProjectForm project={p} onClose={() => setEditing(false)} />}
    </div>
  );
}

// Tasks shows the kanban and features of the selected project, or every task when none is selected.
export function Tasks() {
  const project = useProject();
  const name = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list(), select: (l) => l.find((p) => p.id === project)?.name });
  return (
    <div className="space-y-6">
      <PageHeader title="Tareas" subtitle={project ? name.data : undefined} />
      <Board projectId={project} />
      {project && <Features projectId={project} />}
    </div>
  );
}
