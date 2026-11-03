import { ThreadLink } from "../components/thread-link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { api, type Environment, type Feature, type Memory, type Project, type T3InstanceUsage, type T3Status } from "../api/client";
import { Limit, Models } from "./usage";
import { Badge, Button, Card, ErrorText, Arrow, Input, Label, Md, Modal, pill, Select, Textarea, csv, opts } from "../components/ui";
import { useProject } from "../project";

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

/* ---------- Entornos (solo lectura, descubiertos por T3 Code) ---------- */
const stateLabels: Record<T3Status["state"], string> = { available: "Disponible", disconnected: "Desconectada", auth_failed: "Token rechazado", incompatible: "Versión incompatible" };

// The tab opens before the request so browsers do not block it as a popup.
async function openT3(status: T3Status) {
  const tab = window.open("", "_blank");
  try {
    const { url } = await api.t3Pair(status.instance_id);
    if (tab) tab.location.href = url;
  } catch {
    if (tab) tab.location.href = status.base_url;
  }
}

function InstanceStatus({ status }: { status?: T3Status }) {
  if (!status) return <div className="text-xs text-slate-500 dark:text-muted">T3 Code sin configurar</div>;
  return (
    <div className="space-y-1 text-sm" onClick={(e) => e.stopPropagation()}>
      <div className="flex flex-wrap items-center gap-2">
        <Badge>{stateLabels[status.state]}</Badge>
        <button className={pill} onClick={() => openT3(status)}>Abrir T3 Code <Arrow /></button>
        {status.version && <span className="text-xs text-slate-500 dark:text-muted">{status.version}</span>}
      </div>
      {status.threads.filter((t) => t.attention || t.status === "running").map((t) => (
        <div key={t.id} className="flex items-start justify-between gap-2">
          <ThreadLink className="min-w-0 break-words underline" url={t.url} environmentId={t.t3_environment_id} threadId={t.id}>{t.title}</ThreadLink>
          <span className={`shrink-0 text-xs ${t.attention ? "text-red-600 dark:text-red-400" : "text-slate-500 dark:text-muted"}`}>{t.attention ? "Te necesita" : t.model}</span>
        </div>
      ))}
    </div>
  );
}

function Machine({ env, status, usage }: { env: Environment; status: T3Status; usage?: T3InstanceUsage }) {
  const catalog = useQuery({ queryKey: ["t3-catalog", status.instance_id], queryFn: () => api.t3Catalog(status.instance_id), enabled: status.state === "available", staleTime: 60_000 });
  const muted = "text-sm text-slate-500 dark:text-muted";
  return (
    <Card className="space-y-3">
      <div className="font-medium">{env.name} <span className="text-xs text-slate-500 dark:text-muted">{env.os}</span></div>
      <InstanceStatus status={status} />
      <section className="space-y-2">
        <h3 className="text-sm font-medium">Uso</h3>
        {!usage || usage.limits_error ? <p className={muted}>Sin datos de uso.</p> : (
          <div className="space-y-3">{usage.providers.map((p) => (
            <div key={p.provider} className="space-y-2">
              <div className="text-sm">{p.name}</div>
              {p.windows.length > 0 ? p.windows.map((w) => <Limit key={w.label} w={w} />) : <p className={muted}>Sin datos de plan.</p>}
            </div>
          ))}</div>
        )}
        {usage && !usage.limits_error && <Models usage={usage} />}
      </section>
      <section className="space-y-1">
        <h3 className="text-sm font-medium">Modelos</h3>
        <ErrorText error={catalog.error} />
        {catalog.data?.providers.map((p) => (
          <div key={p.instance_id} className="text-sm">
            <span className="font-medium">{p.name}</span>
            <div className="mt-1 flex flex-wrap gap-1">{p.models.map((m) => <Badge key={m.slug}>{m.name || m.slug}</Badge>)}</div>
          </div>
        ))}
        {status.state !== "available" && <p className={muted}>La máquina no responde.</p>}
      </section>
      <section className="space-y-1">
        <h3 className="text-sm font-medium">Proyectos</h3>
        {catalog.data?.projects.length === 0 && <p className={muted}>Sin proyectos.</p>}
        <ul className="space-y-1 text-sm">{catalog.data?.projects.map((p) => (
          <li key={p.id}>{p.title} <span className="break-all text-xs text-slate-500 dark:text-muted">{p.root}</span></li>
        ))}</ul>
      </section>
    </Card>
  );
}

// Las máquinas las descubre T3 Code; aquí solo se muestran las que tienen instancia.
export function Environments() {
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 10_000 });
  const usage = useQuery({ queryKey: ["t3-usage"], queryFn: api.t3Usage, refetchInterval: 30_000 });
  const machines = (envs.data ?? []).flatMap((env) => {
    const status = t3.data?.find((s) => s.environment_id === env.id);
    return status ? [{ env, status }] : [];
  });
  return (
    <div className="space-y-3">
      <h1 className="text-xl font-bold">Entornos</h1>
      <ErrorText error={envs.error ?? t3.error} />
      {envs.data && t3.data && machines.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">T3 Code no ha detectado ninguna máquina.</p>}
      <div className="grid gap-3 md:grid-cols-2">
        {machines.map(({ env, status }) => <Machine key={env.id} env={env} status={status} usage={usage.data?.find((u) => u.instance_id === status.instance_id)} />)}
      </div>
    </div>
  );
}

/* ---------- Memorias ---------- */
function MemoryForm({ memory, projects, envs, project, onClose }: { memory?: Memory; projects: Project[]; envs: Environment[]; project: string; onClose: () => void }) {
  const [scope, setScope] = useState<Memory["scope"]>(memory?.scope ?? (project ? "project" : "global"));
  const [scopeId, setScopeId] = useState(memory?.scope_id ?? project);
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
      <div className="grid gap-2 sm:grid-cols-2">
        <Label text="Ámbito">
          <Select value={scope} onChange={(v) => { setScope(v as Memory["scope"]); setScopeId(""); }} options={opts(["global", "project", "environment"])} />
        </Label>
        {scope !== "global" && (
          <Label text={scope === "project" ? "Proyecto" : "Entorno"}>
            <Select value={scopeId} onChange={setScopeId} options={[{ value: "", label: "—" }, ...options.map((o) => ({ value: o.id, label: o.name }))]} />
          </Label>
        )}
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <Label text="Tipo">
          <Select value={kind} onChange={(v) => setKind(v as Memory["kind"])} options={opts(["decision", "lesson", "context"])} />
        </Label>
        <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} /></Label>
      </div>
      <Label text="Contenido (markdown)"><Textarea value={content} onChange={(e) => setContent(e.target.value)} /></Label>
      <Label text="Tags (coma)"><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Label>
      <Footer onClose={onClose} onSave={() => save.mutate()} onArchive={memory && (() => arch.mutate())} error={save.error ?? arch.error} disabled={!content.trim() || (scope !== "global" && !scopeId)} />
    </Modal>
  );
}

// Filtros de la página de memorias, reflejados en la URL. El proyecto es el global de la app.
export type MemorySearch = { kind?: Memory["kind"]; q?: string };
export const memorySearch = (s: Record<string, unknown>): MemorySearch => ({
  kind: ["decision", "lesson", "context"].includes(s.kind as string) ? (s.kind as Memory["kind"]) : undefined,
  q: typeof s.q === "string" && s.q ? s.q : undefined,
});


export function Memories() {
  const [editing, setEditing] = useState<Memory | "new" | null>(null);
  const search = useSearch({ from: "/memories" });
  const navigate = useNavigate({ from: "/memories" });
  const setFilter = (patch: MemorySearch) => navigate({ search: memorySearch({ ...search, ...patch }), replace: true });
  const mems = useQuery({ queryKey: ["memories"], queryFn: () => api.memories.list() });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const names = new Map([...(projects.data ?? []), ...(envs.data ?? [])].map((o) => [o.id, o.name]));
  const project = useProject();
  const text = search.q?.toLowerCase() ?? "";
  // With a project selected, show its memories plus the global ones.
  const shown = mems.data?.filter((m) =>
    (!project || m.scope === "global" || m.scope_id === project) &&
    (!search.kind || m.kind === search.kind) &&
    (!text || [m.title, m.content, ...m.tags].some((t) => t.toLowerCase().includes(text))),
  );
  return (
    <div className="space-y-3">
      <Header title="Memorias" onNew={() => setEditing("new")} />
      <div className="flex flex-wrap gap-2">
        <Select className="w-full sm:w-44" value={search.kind ?? ""} onChange={(v) => setFilter({ kind: v as Memory["kind"] })} options={[{ value: "", label: "Todos los tipos" }, ...opts(["decision", "lesson", "context"])]} />
        <Input className="min-w-40 flex-1" placeholder="Buscar texto…" value={search.q ?? ""} onChange={(e) => setFilter({ q: e.target.value })} />
      </div>
      <ErrorText error={mems.error} />
      <div className="space-y-2">
        {shown?.map((m) => (
          <Card key={m.id} className="cursor-pointer hover:bg-slate-50 dark:hover:bg-raised">
            <div onClick={() => setEditing(m)}>
              {m.title && <div className="font-medium">{m.title}</div>}
              <div className="mb-1 flex flex-wrap gap-1">
                <Badge>{m.kind}</Badge>
                <Badge>{m.scope}{m.scope_id && `: ${names.get(m.scope_id) ?? "?"}`}</Badge>
                {m.tags.map((t) => <Badge key={t}>{t}</Badge>)}
              </div>
              <Md>{m.content}</Md>
            </div>
          </Card>
        ))}
        {shown?.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Sin memorias para estos filtros.</p>}
      </div>
      {editing && <MemoryForm memory={editing === "new" ? undefined : editing} projects={projects.data ?? []} envs={envs.data ?? []} project={project} onClose={() => setEditing(null)} />}
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
        <Select value={status} onChange={(v) => setStatus(v as Feature["status"])} options={opts(["planned", "in_progress", "implemented", "deprecated"])} />
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
        <Card key={f.id} className="cursor-pointer hover:bg-slate-50 dark:hover:bg-raised">
          <div onClick={() => setEditing(f)}>
            <div className="flex flex-wrap items-center gap-2"><span className="font-medium">{f.title}</span><Badge>{f.status}</Badge>{f.code_refs.map((r) => <code key={r} className="break-all text-xs text-slate-500 dark:text-muted">{r}</code>)}</div>
            {f.body_md && <Md>{f.body_md}</Md>}
          </div>
        </Card>
      ))}
      {editing && <FeatureForm feature={editing === "new" ? undefined : editing} projectId={projectId} onClose={() => setEditing(null)} />}
    </div>
  );
}
