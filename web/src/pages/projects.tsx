import { DictationField } from "../components/dictation-field";
import { useDictationInProgress } from "../components/local-dictation";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowUp, Desktop, FolderSimple, GitBranch, GithubLogo, HardDrive, House, Plus } from "@phosphor-icons/react";
import { api, repoName, type Project } from "../api/client";
import { Arrow, Button, Card, ErrorText, Input, Label, Modal, PageHeader, pill } from "../components/ui";
import { setProject } from "../project";
import { askConfirm } from "../components/confirm";
import { qk } from "../api/queries/keys";

const slugify = (s: string) => s.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");

// NewProject creates a project from a folder: first the environment, then the
// folder on that machine; name, remote, branch and path are detected.
function NewProject({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const dictating = useDictationInProgress();
  const node = useQuery({ queryKey: qk.node(), queryFn: api.node });
  const mesh = useQuery({ queryKey: qk.mesh(), queryFn: api.mesh });
  const repos = useQuery({ queryKey: qk.repositories(), queryFn: () => api.repositories.list() });
  const [env, setEnv] = useState<{ id: string; name: string }>();
  const [dir, setDir] = useState("");
  const [hidden, setHidden] = useState(false);
  const [picked, setPicked] = useState<string>();
  const envs = [
    ...(node.data ? [{ id: "local", name: node.data.node.name, online: true }] : []),
    ...(mesh.data?.peers ?? []).filter((p) => !p.revoked).map((p) => ({ id: p.id, name: p.name, online: p.connected })),
  ];
  const folders = useQuery({ queryKey: qk.folders(env?.id, dir, hidden), queryFn: () => api.folders(env!.id, dir, hidden), enabled: !!env && !picked });
  const found = useQuery({ queryKey: qk.checkout(env?.id, picked), queryFn: () => api.checkout(env!.id, picked!), enabled: !!env && !!picked });
  const [form, setForm] = useState<{ name: string; slug: string; description: string; remote: string; branch: string }>();
  const c = found.data;
  const existing = c?.remote ? repos.data?.find((r) => repoName(r.remote_url).toLowerCase() === repoName(c.remote).toLowerCase()) : undefined;
  if (c && !form) setForm({ name: c.name, slug: slugify(c.name), description: "", remote: c.remote, branch: c.default_branch || "main" });
  const save = useMutation({
    mutationFn: async () => {
      const fields = { name: form!.name.trim(), slug: form!.slug || slugify(form!.name), description: form!.description };
      if (!form!.remote.trim()) return (await api.projects.create(fields)).id;
      // One project per repository, whatever environment adds it.
      const r = await api.projectFromRepository({ ...fields, remote_url: form!.remote.trim(), default_branch: form!.branch || "main" });
      await api.setNodeRepositoryPath(env!.id, r.id, c!.path);
      return r.project_id;
    },
    onSuccess: (id) => { qc.invalidateQueries({ queryKey: qk.projects() }); qc.invalidateQueries({ queryKey: qk.repositories() }); qc.invalidateQueries({ queryKey: qk.repositoryLocations() }); setProject(id); onClose(); },
  });
  const row = "flex w-full min-h-11 sm:min-h-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <Modal title="Nuevo proyecto" onClose={onClose} wide>
      {!env ? (
        <div className="space-y-1">
          <p className="mb-2 text-sm text-slate-500 dark:text-muted">¿En qué entorno está el proyecto?</p>
          {envs.map((e) => (
            <button key={e.id} type="button" disabled={!e.online} onClick={() => setEnv(e)} className={`${row} disabled:opacity-40`}>
              <Desktop className="size-4" /><span className="flex-1">{e.name}</span>{!e.online && <span className="text-xs">desconectado</span>}
            </button>
          ))}
        </div>
      ) : !picked ? (
        <div className="space-y-2">
          <div className="flex items-center gap-1 text-sm">
            <button type="button" className="rounded px-1.5 py-1 hover:bg-slate-100 dark:hover:bg-raised" onClick={() => setEnv(undefined)}>{env.name}</button>
            <span className="text-slate-400">/</span>
            <span className="min-w-0 flex-1 truncate font-mono text-xs text-slate-600 dark:text-muted" title={folders.data?.path}>{folders.data?.path}</span>
            <button type="button" title="Inicio" className="rounded p-1.5 hover:bg-slate-100 dark:hover:bg-raised" onClick={() => setDir(folders.data?.home ?? "")}><House className="size-4" /></button>
            <button type="button" title="Subir" disabled={!folders.data?.parent} className="rounded p-1.5 hover:bg-slate-100 disabled:opacity-40 dark:hover:bg-raised" onClick={() => setDir(folders.data!.parent)}><ArrowUp className="size-4" /></button>
          </div>
          <ErrorText error={folders.error} />
          <div className="max-h-[50vh] overflow-y-auto rounded-md border border-slate-200 p-1 dark:border-border">
            {(folders.data?.folders ?? []).map((f) => (
              <div key={f.path} className="flex items-center gap-1">
                <button type="button" onClick={() => setDir(f.path)} className={`${row} flex-1`}>
                  {f.git ? <GitBranch className="size-4 text-emerald-600" /> : <FolderSimple className="size-4 text-slate-400" />}
                  <span className="truncate">{f.name}</span>
                </button>
                {f.git && <Button variant="ghost" className="!py-0.5 text-xs" onClick={() => setPicked(f.path)}>Elegir</Button>}
              </div>
            ))}
            {folders.isSuccess && folders.data.folders.length === 0 && <p className="p-2 text-sm text-slate-500">Carpeta vacía.</p>}
          </div>
          <div className="flex items-center gap-2">
            <label className="flex items-center gap-1.5 text-xs text-slate-500"><input type="checkbox" checked={hidden} onChange={(e) => setHidden(e.target.checked)} />Mostrar ocultas</label>
            <Button variant="ghost" className="ml-auto" disabled={!folders.data} onClick={() => setPicked(folders.data!.path)}>Usar esta carpeta</Button>
          </div>
        </div>
      ) : (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (!dictating && form?.name.trim()) save.mutate(); }}>
          <p className="flex items-center gap-2 font-mono text-xs text-slate-500"><HardDrive className="size-4" />{env.name}: {c?.path ?? picked}
            <button type="button" className="ml-auto text-slate-500 underline" onClick={() => { setPicked(undefined); setForm(undefined); }}>cambiar</button></p>
          <ErrorText error={found.error} />
          {c && !c.git && <p className="text-sm text-amber-700 dark:text-amber-300">No es un repositorio de Git: el proyecto se crea sin repositorio y no podrás lanzar hilos con worktree.</p>}
          {existing && <p className="text-sm text-slate-600 dark:text-muted">Este repositorio ya está en Dory: se guarda la ruta en este entorno y se usa su proyecto.</p>}
          {form && !existing && <>
            <div className="grid gap-2 sm:grid-cols-2">
              <Label text="Nombre"><Input autoFocus value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value, slug: slugify(e.target.value) })} /></Label>
              <Label text="Slug"><Input value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value })} className="font-mono" /></Label>
            </div>
            <DictationField label="Descripción" rows={2} value={form.description} onValue={(description) => setForm((current) => current && { ...current, description })} />
            {c?.git && <div className="grid gap-2 sm:grid-cols-[1fr_10rem]">
              <Label text="Remoto"><Input value={form.remote} onChange={(e) => setForm({ ...form, remote: e.target.value })} className="font-mono !text-xs" placeholder="Sin remoto" /></Label>
              <Label text="Rama principal"><Input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} className="font-mono !text-xs" /></Label>
            </div>}
          </>}
          <ErrorText error={save.error} />
          <Button disabled={dictating || !form?.name.trim() || save.isPending}>{existing ? "Guardar ruta" : "Crear proyecto"}</Button>
        </form>
      )}
    </Modal>
  );
}

// useEnvironments names, by repository id, the environments that keep it and where.
function useEnvironments() {
  const node = useQuery({ queryKey: qk.node(), queryFn: api.node });
  const mesh = useQuery({ queryKey: qk.mesh(), queryFn: api.mesh });
  const locations = useQuery({ queryKey: qk.repositoryLocations(), queryFn: api.repositoryLocations });
  const name = (id: string) => id === node.data?.node.id ? node.data.node.name : mesh.data?.peers.find((p) => p.id === id)?.name ?? id.slice(0, 8);
  return (repoId: string) => (locations.data ?? []).filter((l) => l.repository_id === repoId).map((l) => ({ node: l.node_id, name: name(l.node_id), path: l.path }));
}

// Projects lists the projects and creates new ones.
export function Projects() {
  const projects = useQuery({ queryKey: qk.projects(), queryFn: () => api.projects.list() });
  const repos = useQuery({ queryKey: qk.repositories(), queryFn: () => api.repositories.list() });
  const envs = useEnvironments();
  const [adding, setAdding] = useState(false);
  return (
    <div className="space-y-3">
      <PageHeader title="Proyectos"><Button onClick={() => setAdding(true)}><Plus weight="regular" className="size-4" />Nuevo proyecto</Button></PageHeader>
      <ErrorText error={projects.error} />
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {(projects.data ?? []).map((p) => (
          <Link key={p.id} to="/projects/$id" params={{ id: p.id }} className="rounded-lg border border-slate-200 bg-white p-3 hover:border-slate-400 dark:border-border dark:bg-surface">
            <p className="flex items-center gap-2 font-medium"><FolderSimple className="size-4" />{p.name}</p>
            {p.description && <p className="mt-1 line-clamp-2 text-sm text-slate-500 dark:text-muted">{p.description}</p>}
            <p className="mt-2 font-mono text-xs text-slate-500">{(repos.data ?? []).filter((r) => r.project_id === p.id).map((r) => repoName(r.remote_url)).join(", ") || "sin repositorio"}</p>
            {(() => { const names = [...new Set((repos.data ?? []).filter((r) => r.project_id === p.id).flatMap((r) => envs(r.id).map((e) => e.name)))]; return names.length > 0 && <p className="mt-1 flex items-center gap-1 text-xs text-slate-500"><Desktop className="size-3.5" />{names.join(", ")}</p>; })()}
          </Link>
        ))}
      </div>
      {projects.isSuccess && projects.data.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Crea tu primer proyecto y asócialo a un repositorio de Git.</p>}
      {adding && <NewProject onClose={() => setAdding(false)} />}
    </div>
  );
}

function ProjectForm({ project, onClose }: { project?: Project; onClose: () => void }) {
  const qc = useQueryClient();
  const dictating = useDictationInProgress();
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
    onSuccess: (p) => { qc.invalidateQueries({ queryKey: qk.projects() }); qc.invalidateQueries({ queryKey: qk.repositories() }); setProject(p.id); onClose(); },
  });
  return (
    <Modal title={project ? "Editar proyecto" : "Nuevo proyecto"} onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (!dictating && name.trim()) save.mutate(); }}>
        <Label text="Nombre"><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Label>
        <Label text="Slug"><Input value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="Se genera del nombre" /></Label>
        <DictationField label="Descripción" rows={2} value={description} onValue={setDescription} />
        {!project && <>
          <Label text="Repositorio (remoto de Git)"><Input value={remote} onChange={(e) => setRemote(e.target.value)} placeholder="git@github.com:usuario/repo.git" /></Label>
          <Label text="Ruta local en este nodo"><Input value={path} onChange={(e) => setPath(e.target.value)} placeholder="/home/usuario/repos/repo" /></Label>
        </>}
        <ErrorText error={save.error} />
        <Button disabled={dictating || !name.trim() || save.isPending}>Guardar</Button>
      </form>
    </Modal>
  );
}

// ProjectDetail shows a project with its repositories and where this node
// keeps each one (requirement 4.9: the path is local to each machine).
export function ProjectDetail() {
  const { id } = useParams({ strict: false }) as { id: string };
  const qc = useQueryClient();
  const project = useQuery({ queryKey: qk.project(id), queryFn: () => api.projects.get(id) });
  const repos = useQuery({ queryKey: qk.repositoriesOf(id), queryFn: () => api.repositories.list({ project_id: id }) });
  const paths = useQuery({ queryKey: qk.repositoryPaths(), queryFn: api.repositoryPaths });
  const envs = useEnvironments();
  const [editing, setEditing] = useState(false);
  const [remote, setRemote] = useState("");
  const add = useMutation({
    mutationFn: () => api.repositories.create({ project_id: id, remote_url: remote.trim() }),
    onSuccess: () => { setRemote(""); qc.invalidateQueries({ queryKey: qk.repositories() }); },
  });
  const archive = useMutation({ mutationFn: () => api.projects.archive(id), onSuccess: () => { qc.invalidateQueries({ queryKey: qk.projects() }); history.back(); } });
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
          {(repos.data ?? []).map((r) => <RepoRow key={r.id} id={r.id} remote={r.remote_url} branch={r.default_branch} path={paths.data?.[r.id] ?? ""} envs={envs(r.id)} />)}
          <form className="mt-3 flex gap-2" onSubmit={(e) => { e.preventDefault(); if (remote.trim()) add.mutate(); }}>
            <Input value={remote} onChange={(e) => setRemote(e.target.value)} placeholder="Añadir remoto: git@github.com:usuario/repo.git" />
            <Button disabled={!remote.trim() || add.isPending}>Añadir</Button>
          </form>
          <ErrorText error={add.error} />
        </Card>
        <div className="flex gap-2">
          <Link to="/tasks" className={pill} onClick={() => setProject(p.id)}>Ver tareas</Link>
          <Link to="/memories" className={pill} onClick={() => setProject(p.id)}>Ver memorias</Link>
          <Button variant="ghost" className="ml-auto" onClick={() => askConfirm("¿Archivar el proyecto?", "Archivar").then((ok) => ok && archive.mutate())}>Archivar</Button>
        </div>
        {editing && <ProjectForm project={p} onClose={() => setEditing(false)} />}
      </>}
    </div>
  );
}

function RepoRow({ id, remote, branch, path, envs }: { id: string; remote: string; branch: string; path: string; envs: { node: string; name: string; path: string }[] }) {
  const qc = useQueryClient();
  const [value, setValue] = useState(path);
  const save = useMutation({ mutationFn: () => api.setRepositoryPath(id, value.trim()), onSuccess: () => { qc.invalidateQueries({ queryKey: qk.repositoryPaths() }); qc.invalidateQueries({ queryKey: qk.repositoryLocations() }); } });
  return (
    <div className="space-y-1 border-t border-slate-200 py-2 first:border-t-0 dark:border-border">
      <p className="flex items-center gap-2 text-sm"><GithubLogo className="size-4" /><span className="font-mono">{repoName(remote)}</span><span className="text-xs text-slate-500">· {branch}</span></p>
      {envs.map((e) => <p key={e.node} className="flex items-center gap-2 text-xs text-slate-500"><Desktop className="size-3.5 shrink-0" />{e.name}<span className="truncate font-mono">{e.path}</span></p>)}
      <form className="flex items-center gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <HardDrive className="size-4 shrink-0 text-slate-400" />
        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="Ruta local en este nodo (sin ella no se pueden lanzar hilos aquí)" className="font-mono !text-xs" />
        {value !== path && <Button disabled={save.isPending}>Guardar</Button>}
      </form>
      <ErrorText error={save.error} />
    </div>
  );
}
