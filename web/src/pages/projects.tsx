import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api } from "../api/client";
import { Badge, Button, Card, ErrorText, Md } from "../components/ui";
import { Features, ProjectForm } from "./crud";
import { Board } from "./items";

export function Projects() {
  const [creating, setCreating] = useState(false);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Proyectos</h1>
        <Button onClick={() => setCreating(true)}>Nuevo</Button>
      </div>
      <ErrorText error={projects.error} />
      <div className="grid gap-3 md:grid-cols-2">
        {projects.data?.map((p) => (
          <Link key={p.id} to="/projects/$id" params={{ id: p.id }}>
            <Card className="hover:bg-slate-50">
              <div className="font-medium">{p.name} <span className="text-xs text-slate-500">{p.slug}</span></div>
              <p className="text-sm text-slate-600">{p.description}</p>
              <div className="mt-1 flex flex-wrap gap-1"><Badge>{p.status}</Badge>{p.stack.map((s) => <Badge key={s}>{s}</Badge>)}</div>
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
  const [editing, setEditing] = useState(false);
  const project = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list(), select: (l) => l.find((p) => p.id === id) });
  const repos = useQuery({ queryKey: ["repositories", id], queryFn: () => api.repositories.list({ project_id: id }) });
  const p = project.data;
  if (!p) return <ErrorText error={project.error} />;
  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <div className="flex items-start justify-between gap-2">
          <h1 className="text-xl font-bold break-words">{p.name} <span className="text-sm font-normal text-slate-500">{p.slug}</span></h1>
          <Button variant="ghost" onClick={() => setEditing(true)}>Editar</Button>
        </div>
        {p.description && <Md>{p.description}</Md>}
        <div className="flex flex-wrap gap-1"><Badge>{p.status}</Badge>{p.stack.map((s) => <Badge key={s}>{s}</Badge>)}</div>
        {repos.data?.map((r) => <div key={r.id} className="break-all text-sm text-slate-600">⎇ {r.remote_url} <span className="text-xs">({r.default_branch})</span></div>)}
      </div>
      <Board projectId={id} />
      <Features projectId={id} />
      {editing && <ProjectForm project={p} onClose={() => setEditing(false)} />}
    </div>
  );
}
