import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Environment, type Feature, type Memory, type Project } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, Textarea, csv } from "../components/ui";

// Hook compartido: guardar/archivar e invalidar la lista.
function useSave<T>(key: string, fn: () => Promise<T>, onClose: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [key] });
      onClose();
    },
  });
}

function Footer({ onClose, onSave, onArchive, error, disabled }: { onClose: () => void; onSave: () => void; onArchive?: () => void; error: unknown; disabled?: boolean }) {
  return (
    <>
      <ErrorText error={error} />
      <div className="flex justify-between">
        {onArchive ? <Button variant="danger" onClick={onArchive}>Archivar</Button> : <span />}
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button disabled={disabled} onClick={onSave}>Guardar</Button>
        </div>
      </div>
    </>
  );
}

function Header({ title, onNew }: { title: string; onNew: () => void }) {
  return (
    <div className="flex items-center justify-between">
      <h1 className="text-xl font-bold">{title}</h1>
      <Button onClick={onNew}>Nuevo</Button>
    </div>
  );
}

/* ---------- Proyectos ---------- */
export function ProjectForm({ project, onClose }: { project?: Project; onClose: () => void }) {
  const [name, setName] = useState(project?.name ?? "");
  const [slug, setSlug] = useState(project?.slug ?? "");
  const [description, setDescription] = useState(project?.description ?? "");
  const [status, setStatus] = useState(project?.status ?? "active");
  const [stack, setStack] = useState(project?.stack.join(", ") ?? "");
  const data = { name, slug, description, status, stack: csv(stack) };
  const save = useSave("projects", () => (project ? api.projects.update(project.id, { id: project.id, ...data }) : api.projects.create(data)), onClose);
  const arch = useSave("projects", () => api.projects.archive(project!.id), onClose);
  return (
    <Modal title={project ? "Editar proyecto" : "Nuevo proyecto"} onClose={onClose}>
      <Label text="Nombre"><Input value={name} onChange={(e) => setName(e.target.value)} /></Label>
      <Label text="Slug"><Input value={slug} onChange={(e) => setSlug(e.target.value)} /></Label>
      <Label text="Descripción"><Textarea value={description} onChange={(e) => setDescription(e.target.value)} /></Label>
      <Label text="Estado"><Input value={status} onChange={(e) => setStatus(e.target.value)} /></Label>
      <Label text="Stack (coma)"><Input value={stack} onChange={(e) => setStack(e.target.value)} /></Label>
      <Footer onClose={onClose} onSave={() => save.mutate()} onArchive={project && (() => arch.mutate())} error={save.error ?? arch.error} disabled={!name.trim() || !slug.trim()} />
    </Modal>
  );
}

/* ---------- Entornos ---------- */
function EnvironmentForm({ env, onClose }: { env?: Environment; onClose: () => void }) {
  const [name, setName] = useState(env?.name ?? "");
  const [os, setOs] = useState(env?.os ?? "");
  const [access, setAccess] = useState(env?.access ?? "");
  const [agents, setAgents] = useState(env?.agents_available.join(", ") ?? "");
  const [notes, setNotes] = useState(env?.notes ?? "");
  const data = { name, os, access, agents_available: csv(agents), notes };
  const save = useSave("environments", () => (env ? api.environments.update(env.id, { id: env.id, ...data }) : api.environments.create(data)), onClose);
  const arch = useSave("environments", () => api.environments.archive(env!.id), onClose);
  return (
    <Modal title={env ? "Editar entorno" : "Nuevo entorno"} onClose={onClose}>
      <Label text="Nombre"><Input value={name} onChange={(e) => setName(e.target.value)} /></Label>
      <Label text="SO"><Input value={os} onChange={(e) => setOs(e.target.value)} /></Label>
      <Label text="Acceso (alias SSH, Tailscale…)"><Textarea value={access} onChange={(e) => setAccess(e.target.value)} /></Label>
      <Label text="Agentes disponibles (coma)"><Input value={agents} onChange={(e) => setAgents(e.target.value)} /></Label>
      <Label text="Notas"><Textarea value={notes} onChange={(e) => setNotes(e.target.value)} /></Label>
      <Footer onClose={onClose} onSave={() => save.mutate()} onArchive={env && (() => arch.mutate())} error={save.error ?? arch.error} disabled={!name.trim()} />
    </Modal>
  );
}

export function Environments() {
  const [editing, setEditing] = useState<Environment | "new" | null>(null);
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  return (
    <div className="space-y-3">
      <Header title="Entornos" onNew={() => setEditing("new")} />
      <ErrorText error={envs.error} />
      <div className="grid gap-3 md:grid-cols-2">
        {envs.data?.map((e) => (
          <Card key={e.id} className="cursor-pointer hover:bg-slate-50">
            <div onClick={() => setEditing(e)} className="space-y-1">
              <div className="font-medium">{e.name} <span className="text-xs text-slate-500">{e.os}</span></div>
              <div className="flex flex-wrap gap-1">{e.agents_available.map((a) => <Badge key={a}>{a}</Badge>)}</div>
              {e.access && <Md>{e.access}</Md>}
              {e.notes && <Md>{e.notes}</Md>}
            </div>
          </Card>
        ))}
      </div>
      {editing && <EnvironmentForm env={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

/* ---------- Memorias ---------- */
function MemoryForm({ memory, projects, envs, onClose }: { memory?: Memory; projects: Project[]; envs: Environment[]; onClose: () => void }) {
  const [scope, setScope] = useState<Memory["scope"]>(memory?.scope ?? "global");
  const [scopeId, setScopeId] = useState(memory?.scope_id ?? "");
  const [kind, setKind] = useState<Memory["kind"]>(memory?.kind ?? "context");
  const [title, setTitle] = useState(memory?.title ?? "");
  const [content, setContent] = useState(memory?.content ?? "");
  const [tags, setTags] = useState(memory?.tags.join(", ") ?? "");
  const data = { scope, scope_id: scope === "global" ? null : scopeId || null, kind, title, content, tags: csv(tags) };
  const save = useSave("memories", () => (memory ? api.memories.update(memory.id, { id: memory.id, ...data }) : api.memories.create(data)), onClose);
  const arch = useSave("memories", () => api.memories.archive(memory!.id), onClose);
  const options = scope === "project" ? projects : envs;
  return (
    <Modal title={memory ? "Editar memoria" : "Nueva memoria"} onClose={onClose}>
      <div className="grid grid-cols-2 gap-2">
        <Label text="Ámbito">
          <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={scope} onChange={(e) => { setScope(e.target.value as Memory["scope"]); setScopeId(""); }}>
            <option>global</option><option>project</option><option>environment</option>
          </select>
        </Label>
        {scope !== "global" && (
          <Label text={scope === "project" ? "Proyecto" : "Entorno"}>
            <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
              <option value="">—</option>
              {options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
            </select>
          </Label>
        )}
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Label text="Tipo">
          <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={kind} onChange={(e) => setKind(e.target.value as Memory["kind"])}>
            <option>decision</option><option>lesson</option><option>context</option>
          </select>
        </Label>
        <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} /></Label>
      </div>
      <Label text="Contenido (markdown)"><Textarea value={content} onChange={(e) => setContent(e.target.value)} /></Label>
      <Label text="Tags (coma)"><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Label>
      <Footer onClose={onClose} onSave={() => save.mutate()} onArchive={memory && (() => arch.mutate())} error={save.error ?? arch.error} disabled={!content.trim() || (scope !== "global" && !scopeId)} />
    </Modal>
  );
}

export function Memories() {
  const [editing, setEditing] = useState<Memory | "new" | null>(null);
  const mems = useQuery({ queryKey: ["memories"], queryFn: () => api.memories.list() });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  return (
    <div className="space-y-3">
      <Header title="Memorias" onNew={() => setEditing("new")} />
      <ErrorText error={mems.error} />
      <div className="space-y-2">
        {mems.data?.map((m) => (
          <Card key={m.id} className="cursor-pointer hover:bg-slate-50">
            <div onClick={() => setEditing(m)}>
              {m.title && <div className="font-medium">{m.title}</div>}
              <div className="mb-1 flex gap-1"><Badge>{m.kind}</Badge><Badge>{m.scope}</Badge>{m.tags.map((t) => <Badge key={t}>{t}</Badge>)}</div>
              <Md>{m.content}</Md>
            </div>
          </Card>
        ))}
      </div>
      {editing && <MemoryForm memory={editing === "new" ? undefined : editing} projects={projects.data ?? []} envs={envs.data ?? []} onClose={() => setEditing(null)} />}
    </div>
  );
}

/* ---------- Features (por proyecto) ---------- */
function FeatureForm({ feature, projectId, onClose }: { feature?: Feature; projectId: string; onClose: () => void }) {
  const [title, setTitle] = useState(feature?.title ?? "");
  const [body, setBody] = useState(feature?.body_md ?? "");
  const [refs, setRefs] = useState(feature?.code_refs.join(", ") ?? "");
  const [status, setStatus] = useState<Feature["status"]>(feature?.status ?? "planned");
  const data = { project_id: projectId, title, body_md: body, code_refs: csv(refs), status };
  const save = useSave("features", () => (feature ? api.features.update(feature.id, { id: feature.id, ...data }) : api.features.create(data)), onClose);
  const arch = useSave("features", () => api.features.archive(feature!.id), onClose);
  return (
    <Modal title={feature ? "Editar feature" : "Nueva feature"} onClose={onClose}>
      <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} /></Label>
      <Label text="Descripción (markdown)"><Textarea value={body} onChange={(e) => setBody(e.target.value)} /></Label>
      <Label text="Referencias de código (coma)"><Input value={refs} onChange={(e) => setRefs(e.target.value)} /></Label>
      <Label text="Estado">
        <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={status} onChange={(e) => setStatus(e.target.value as Feature["status"])}>
          <option>planned</option><option>in_progress</option><option>implemented</option><option>deprecated</option>
        </select>
      </Label>
      <Footer onClose={onClose} onSave={() => save.mutate()} onArchive={feature && (() => arch.mutate())} error={save.error ?? arch.error} disabled={!title.trim()} />
    </Modal>
  );
}

export function Features({ projectId }: { projectId: string }) {
  const [editing, setEditing] = useState<Feature | "new" | null>(null);
  const feats = useQuery({ queryKey: ["features", projectId], queryFn: () => api.features.list({ project_id: projectId }) });
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Features</h2>
        <Button variant="ghost" onClick={() => setEditing("new")}>Nueva feature</Button>
      </div>
      <ErrorText error={feats.error} />
      {feats.data?.map((f) => (
        <Card key={f.id} className="cursor-pointer hover:bg-slate-50">
          <div onClick={() => setEditing(f)}>
            <div className="flex items-center gap-2"><span className="font-medium">{f.title}</span><Badge>{f.status}</Badge>{f.code_refs.map((r) => <code key={r} className="text-xs text-slate-500">{r}</code>)}</div>
            {f.body_md && <Md>{f.body_md}</Md>}
          </div>
        </Card>
      ))}
      {editing && <FeatureForm feature={editing === "new" ? undefined : editing} projectId={projectId} onClose={() => setEditing(null)} />}
    </div>
  );
}
