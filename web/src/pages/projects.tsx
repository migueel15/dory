import { useState } from "react";
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
      <PageHeader title="Proyectos" subtitle="Elige uno para ver su tablero. El filtro global se cambia arriba.">
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
  const project = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list(), select: (l) => l.find((p) => p.id === id) });
  const repos = useQuery({ queryKey: ["repositories", id], queryFn: () => api.repositories.list({ project_id: id }) });
  const p = project.data;
  if (!p) return <ErrorText error={project.error} />;
  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <Link to="/projects" className="text-sm text-slate-500 hover:underline dark:text-muted">← Proyectos</Link>
        <div className="flex items-start justify-between gap-2">
          <h1 className="text-xl font-bold break-words">{p.name} <span className="text-sm font-normal text-slate-500 dark:text-muted">{p.slug}</span></h1>
          <Button variant="ghost" onClick={() => setEditing(true)}>Editar</Button>
        </div>
        {p.description && <Md>{p.description}</Md>}
        <div className="flex flex-wrap gap-1"><Badge>{p.status}</Badge>{p.stack.map((s) => <Badge key={s}>{s}</Badge>)}</div>
        {repos.data?.map((r) => <div key={r.id} className="break-all text-sm text-slate-600 dark:text-text-secondary">⎇ {r.remote_url} <span className="text-xs">({r.default_branch})</span></div>)}
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
      <PageHeader title="Tareas" subtitle={project ? `Proyecto: ${name.data ?? "…"}` : "Todos los proyectos. Elige uno arriba para filtrar."} />
      <Board projectId={project} />
      {project && <Features projectId={project} />}
    </div>
  );
}
