import { ThreadLink } from "../components/thread-link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { api, type Environment, type Feature, type Memory, type Project, type T3Status } from "../api/client";
import { AppleLogo, Desktop, FolderSimple, LinuxLogo, Robot, Warning, WindowsLogo } from "@phosphor-icons/react";
import { Badge, Button, Card, ErrorText, Arrow, Input, Label, Md, Modal, PageHeader, pill, Select, StatusPill, Textarea, csv, opts, type Tone } from "../components/ui";
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

const stateTone: Record<T3Status["state"], Tone> = { available: "green", disconnected: "slate", auth_failed: "red", incompatible: "amber" };
const since = (iso: string | null) => {
  if (!iso) return "nunca";
  const m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60_000));
  return m < 1 ? "ahora" : m < 60 ? `hace ${m} min` : m < 1440 ? `hace ${Math.round(m / 60)} h` : `hace ${Math.round(m / 1440)} d`;
};
const osIcon = (os: string) => (/mac|darwin/i.test(os) ? AppleLogo : /win/i.test(os) ? WindowsLogo : /linux|debian|ubuntu|arch|fedora/i.test(os) ? LinuxLogo : Desktop);

// Machine describes one device: identity, connection, agent activity, providers and projects.
function Machine({ env, status }: { env: Environment; status: T3Status }) {
  const up = status.state === "available";
  const catalog = useQuery({ queryKey: ["t3-catalog", status.instance_id], queryFn: () => api.t3Catalog(status.instance_id), enabled: up, staleTime: 60_000 });
  const working = status.threads.filter((t) => ["running", "pending", "starting", "working"].includes(t.status));
  const needing = status.threads.filter((t) => t.attention);
  const Os = osIcon(env.os);
  const host = (() => { try { return new URL(status.base_url).host; } catch { return status.base_url; } })();
  const muted = "text-xs text-slate-500 dark:text-muted";
  return (
    <Card className="flex h-full flex-col gap-4">
      <div className="flex items-start gap-3">
        <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-slate-100 text-slate-700 dark:bg-raised dark:text-text"><Os weight="duotone" aria-hidden="true" className="size-6" /></span>
        <div className="min-w-0 flex-1">
          <div className="truncate font-semibold">{env.name}</div>
          <div className={`truncate ${muted}`}>{env.os || "Sistema desconocido"}</div>
        </div>
        <StatusPill tone={stateTone[status.state]}><span className="inline-flex items-center gap-1.5"><span className={`size-1.5 rounded-full ${up ? "bg-emerald-500" : "bg-current"}`} />{stateLabels[status.state]}</span></StatusPill>
      </div>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <div className="min-w-0"><dt className={muted}>Dirección</dt><dd className="truncate font-mono text-xs" title={status.base_url}>{host}</dd></div>
        <div className="min-w-0"><dt className={muted}>T3 Code</dt><dd className="truncate">{status.version || "—"}</dd></div>
        <div className="min-w-0"><dt className={muted}>Última conexión</dt><dd title={status.last_seen ? new Date(status.last_seen).toLocaleString() : undefined}>{since(status.last_seen)}</dd></div>
        <div className="min-w-0"><dt className={muted}>Acceso</dt><dd className="truncate" title={env.access}>{env.access || "—"}</dd></div>
      </dl>

      <div className="grid grid-cols-3 divide-x divide-slate-200 rounded-lg border border-slate-200 text-center dark:divide-border dark:border-border">
        {[["Hilos", status.threads.length, ""], ["Trabajando", working.length, working.length ? "text-blue-700 dark:text-accent-text" : ""], ["Te necesitan", needing.length, needing.length ? "text-amber-700 dark:text-amber-400" : ""]].map(([k, v, c]) => (
          <div key={k} className="py-2"><div className={`text-lg font-semibold tabular-nums ${c}`}>{v}</div><div className={muted}>{k}</div></div>
        ))}
      </div>

      {needing.length > 0 && (
        <ul className="space-y-1 text-sm">
          {needing.map((t) => <li key={t.id} className="flex items-center gap-2"><Warning weight="fill" aria-hidden="true" className="size-4 shrink-0 text-amber-500" /><ThreadLink className="min-w-0 truncate hover:underline" url={t.url} environmentId={t.t3_environment_id} threadId={t.id}>{t.title}</ThreadLink></li>)}
        </ul>
      )}

      <section className="space-y-1.5">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-muted">Agentes</h3>
        {!up && <p className={muted}>La máquina no responde.</p>}
        <ErrorText error={catalog.error} />
        {catalog.data?.providers.map((p) => (
          <details key={p.instance_id} className="group text-sm">
            <summary className="flex cursor-pointer list-none items-center gap-2">
              <Robot weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
              <span className="flex-1 font-medium">{p.name}</span>
              <span className={muted}>{p.models.length} modelos</span>
              <span aria-hidden="true" className="text-slate-400 transition-transform group-open:rotate-90 motion-reduce:transition-none">›</span>
            </summary>
            <div className="mt-1.5 flex flex-wrap gap-1 pl-6">{p.models.map((m) => <Badge key={m.slug}>{m.name || m.slug}</Badge>)}</div>
          </details>
        ))}
        {up && catalog.data?.providers.length === 0 && <p className={muted}>Sin agentes configurados.</p>}
      </section>

      <section className="space-y-1.5">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-muted">Proyectos en T3 {catalog.data && <span className="font-normal normal-case tracking-normal">({catalog.data.projects.length})</span>}</h3>
        {catalog.data?.projects.length === 0 && <p className={muted}>Sin proyectos.</p>}
        <ul className="space-y-1 text-sm">{catalog.data?.projects.map((p) => (
          <li key={p.id} className="flex min-w-0 items-center gap-2"><FolderSimple weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" /><span className="shrink-0">{p.title}</span><span className="min-w-0 truncate font-mono text-[11px] text-slate-400 dark:text-muted" title={p.root}>{p.root}</span></li>
        ))}</ul>
      </section>

      {env.notes && <p className="whitespace-pre-wrap rounded-md bg-slate-50 p-2 text-xs text-slate-600 dark:bg-raised dark:text-text-secondary">{env.notes}</p>}

      <div className="mt-auto flex justify-end border-t border-slate-100 pt-3 dark:border-border">
        <button type="button" className={pill} onClick={() => openT3(status)}>Abrir T3 Code <Arrow /></button>
      </div>
    </Card>
  );
}

// Environments lists the devices T3 Code reports, in name order. Usage lives in Uso.
export function Environments() {
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  const machines = (envs.data ?? []).flatMap((env) => {
    const status = t3.data?.find((s) => s.environment_id === env.id);
    return status ? [{ env, status }] : [];
  }).sort((a, b) => a.env.name.localeCompare(b.env.name));
  const online = machines.filter((m) => m.status.state === "available").length;
  return (
    <div className="space-y-3">
      <PageHeader title="Entornos" subtitle={machines.length ? `${online} de ${machines.length} disponibles` : undefined} />
      <ErrorText error={envs.error ?? t3.error} />
      {envs.data && t3.data && machines.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">T3 Code no ha detectado ninguna máquina.</p>}
      <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
        {machines.map(({ env, status }) => <Machine key={env.id} env={env} status={status} />)}
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

const FEATURE_TONE = { planned: "slate", in_progress: "blue", implemented: "green", deprecated: "amber" } as const;

function FeatureDetail({ feature, onEdit, onClose }: { feature: Feature; onEdit: () => void; onClose: () => void }) {
  return (
    <Modal title={feature.title} onClose={onClose} wide>
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill tone={FEATURE_TONE[feature.status] ?? "slate"}>{feature.status.replace("_", " ")}</StatusPill>
      </div>
      {feature.body_md ? <div className="max-h-[60vh] overflow-y-auto"><Md>{feature.body_md}</Md></div> : <p className="text-sm italic text-slate-500 dark:text-muted">Sin descripción.</p>}
      {feature.code_refs.length > 0 && (
        <div className="space-y-1">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-400 dark:text-muted">Código</h3>
          <div className="flex flex-wrap gap-1.5">{feature.code_refs.map((r) => <code key={r} className="break-all rounded bg-slate-100 px-1.5 py-0.5 text-xs dark:bg-raised">{r}</code>)}</div>
        </div>
      )}
      <div className="flex justify-end gap-2 border-t border-slate-200 pt-3 dark:border-border">
        <Button variant="ghost" onClick={onClose}>Cerrar</Button>
        <Button onClick={onEdit}>Editar</Button>
      </div>
    </Modal>
  );
}

export function Features({ projectId }: { projectId: string }) {
  const [editing, setEditing] = useState<Feature | "new" | null>(null);
  const [viewing, setViewing] = useState<Feature | null>(null);
  const feats = useQuery({ queryKey: ["features", projectId], queryFn: () => api.features.list({ project_id: projectId }) });
  return (
    <>
    <details className="group rounded-lg border border-slate-200 bg-white dark:border-border dark:bg-surface">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-3 py-2 text-sm">
        <span className="font-semibold">Features <span className="font-normal text-slate-500 dark:text-muted">({feats.data?.length ?? 0})</span><span aria-hidden="true" className="ml-1 inline-block text-slate-400 transition-transform group-open:rotate-90 motion-reduce:transition-none">›</span></span>
        <Button variant="ghost" onClick={(e) => { e.preventDefault(); setEditing("new"); }}>Nueva feature</Button>
      </summary>
      <ErrorText error={feats.error} />
      <ul className="divide-y divide-slate-100 border-t border-slate-200 dark:divide-border dark:border-border">
        {feats.data?.map((f) => (
          <li key={f.id}>
            <button type="button" onClick={() => setViewing(f)} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-slate-50 dark:hover:bg-raised">
              <span className="min-w-0 flex-1 truncate">{f.title}</span><Badge>{f.status}</Badge>
            </button>
          </li>
        ))}
      </ul>
    </details>
    {viewing && <FeatureDetail feature={viewing} onClose={() => setViewing(null)} onEdit={() => { setEditing(viewing); setViewing(null); }} />}
    {editing && <FeatureForm feature={editing === "new" ? undefined : editing} projectId={projectId} onClose={() => setEditing(null)} />}
    </>
  );
}
