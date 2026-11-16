import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { FolderSimple, GithubLogo, HardDrive, Plus } from "@phosphor-icons/react";
import { api, repoName, type Project } from "../api/client";
import { Arrow, Button, Card, ErrorText, Input, Label, Modal, PageHeader, Textarea, pill } from "../components/ui";
import { setProject } from "../project";

// Projects lists the projects and creates new ones.
export function Projects() {
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const repos = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list() });
  const [adding, setAdding] = useState(false);
  return (
    <div className="space-y-3">
      <PageHeader title="Proyectos"><Button onClick={() => setAdding(true)}><Plus className="size-4" />Nuevo proyecto</Button></PageHeader>
      <ErrorText error={projects.error} />
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {(projects.data ?? []).map((p) => (
          <Link key={p.id} to="/projects/$id" params={{ id: p.id }} className="rounded-lg border border-slate-200 bg-white p-3 hover:border-slate-400 dark:border-border dark:bg-surface">
            <p className="flex items-center gap-2 font-medium"><FolderSimple className="size-4" />{p.name}</p>
            {p.description && <p className="mt-1 line-clamp-2 text-sm text-slate-500 dark:text-muted">{p.description}</p>}
            <p className="mt-2 font-mono text-xs text-slate-500">{(repos.data ?? []).filter((r) => r.project_id === p.id).map((r) => repoName(r.remote_url)).join(", ") || "sin repositorio"}</p>
          </Link>
        ))}
      </div>
      {projects.isSuccess && projects.data.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Crea tu primer proyecto y asócialo a un repositorio de Git.</p>}
      {adding && <ProjectForm onClose={() => setAdding(false)} />}
    </div>
  );
}

function ProjectForm({ project, onClose }: { project?: Project; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(project?.name ?? "");
  const [slug, setSlug] = useState(project?.slug ?? "");
  const [description, setDescription] = useState(project?.description ?? "");
  const [remote, setRemote] = useState("");
  const [path, setPath] = useState("");
  const save = useMutation({
    mutationFn: async () => {
      const auto = slug || name.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
      const p = project ? await api.projects.update(project.id, { name, slug: auto, description }) : await api.projects.create({ name, slug: auto, description });
      if (remote.trim()) {
        const r = await api.repositories.create({ project_id: p.id, remote_url: remote.trim() });
        if (path.trim()) await api.setRepositoryPath(r.id, path.trim());
      }
      return p;
    },
    onSuccess: (p) => { qc.invalidateQueries({ queryKey: ["projects"] }); qc.invalidateQueries({ queryKey: ["repositories"] }); setProject(p.id); onClose(); },
  });
  return (
    <Modal title={project ? "Editar proyecto" : "Nuevo proyecto"} onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (name.trim()) save.mutate(); }}>
        <Label text="Nombre"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Label>
        <Label text="Slug"><Input value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="Se genera del nombre" /></Label>
        <Label text="Descripción"><Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} /></Label>
        {!project && <>
          <Label text="Repositorio (remoto de Git)"><Input value={remote} onChange={(e) => setRemote(e.target.value)} placeholder="git@github.com:usuario/repo.git" /></Label>
          <Label text="Ruta local en este nodo"><Input value={path} onChange={(e) => setPath(e.target.value)} placeholder="/home/usuario/repos/repo" /></Label>
        </>}
        <ErrorText error={save.error} />
        <Button disabled={!name.trim() || save.isPending}>Guardar</Button>
      </form>
    </Modal>
  );
}

// ProjectDetail shows a project with its repositories and where this node
// keeps each one (requirement 4.9: the path is local to each machine).
export function ProjectDetail() {
  const { id } = useParams({ strict: false }) as { id: string };
  const qc = useQueryClient();
  const project = useQuery({ queryKey: ["projects", id], queryFn: () => api.projects.get(id) });
  const repos = useQuery({ queryKey: ["repositories", id], queryFn: () => api.repositories.list({ project_id: id }) });
  const paths = useQuery({ queryKey: ["repositories", "paths"], queryFn: api.repositoryPaths });
  const [editing, setEditing] = useState(false);
  const [remote, setRemote] = useState("");
  const add = useMutation({
    mutationFn: () => api.repositories.create({ project_id: id, remote_url: remote.trim() }),
    onSuccess: () => { setRemote(""); qc.invalidateQueries({ queryKey: ["repositories"] }); },
  });
  const archive = useMutation({ mutationFn: () => api.projects.archive(id), onSuccess: () => { qc.invalidateQueries({ queryKey: ["projects"] }); history.back(); } });
  const p = project.data;
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <Link to="/projects" className={pill}><Arrow back /> Proyectos</Link>
      <ErrorText error={project.error} />
      {p && <>
        <PageHeader title={p.name} subtitle={p.description}>
          <Button variant="ghost" onClick={() => setEditing(true)}>Editar</Button>
          <Button variant="ghost" onClick={() => { setProject(p.id); }}>Seleccionar</Button>
        </PageHeader>
        <Card>
          <h2 className="mb-2 font-semibold">Repositorios</h2>
          {(repos.data ?? []).map((r) => <RepoRow key={r.id} id={r.id} remote={r.remote_url} branch={r.default_branch} path={paths.data?.[r.id] ?? ""} />)}
          <form className="mt-3 flex gap-2" onSubmit={(e) => { e.preventDefault(); if (remote.trim()) add.mutate(); }}>
            <Input value={remote} onChange={(e) => setRemote(e.target.value)} placeholder="Añadir remoto: git@github.com:usuario/repo.git" />
            <Button disabled={!remote.trim() || add.isPending}>Añadir</Button>
          </form>
          <ErrorText error={add.error} />
        </Card>
        <div className="flex gap-2">
          <Link to="/tasks" className={pill} onClick={() => setProject(p.id)}>Ver tareas</Link>
          <Link to="/memories" className={pill} onClick={() => setProject(p.id)}>Ver memorias</Link>
          <Button variant="ghost" className="ml-auto" onClick={() => confirm("¿Archivar el proyecto?") && archive.mutate()}>Archivar</Button>
        </div>
        {editing && <ProjectForm project={p} onClose={() => setEditing(false)} />}
      </>}
    </div>
  );
}

function RepoRow({ id, remote, branch, path }: { id: string; remote: string; branch: string; path: string }) {
  const qc = useQueryClient();
  const [value, setValue] = useState(path);
  const save = useMutation({ mutationFn: () => api.setRepositoryPath(id, value.trim()), onSuccess: () => qc.invalidateQueries({ queryKey: ["repositories", "paths"] }) });
  return (
    <div className="space-y-1 border-t border-slate-200 py-2 first:border-t-0 dark:border-border">
      <p className="flex items-center gap-2 text-sm"><GithubLogo className="size-4" /><span className="font-mono">{repoName(remote)}</span><span className="text-xs text-slate-500">· {branch}</span></p>
      <form className="flex items-center gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <HardDrive className="size-4 shrink-0 text-slate-400" />
        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="Ruta local en este nodo (sin ella no se pueden lanzar hilos aquí)" className="font-mono !text-xs" />
        {value !== path && <Button disabled={save.isPending}>Guardar</Button>}
      </form>
      <ErrorText error={save.error} />
    </div>
  );
}
