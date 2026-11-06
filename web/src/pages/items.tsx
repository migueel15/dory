import { ThreadStatus } from "../components/thread-status";
import { ChatCircle, GitPullRequest, Plus } from "@phosphor-icons/react";
import { ThreadLink } from "../components/thread-link";
import { LaunchDialog } from "./launch";
import { ContextMenu, type ContextAction } from "../components/context-menu";
import { useCallback, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DndContext, type DragEndEvent, PointerSensor, TouchSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import { api, itemInput, type Item, type ItemStatus } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, Notice, PageHeader, Select, Textarea, csv, opts, uuid } from "../components/ui";
import { useProject } from "../project";

const STATUSES: ItemStatus[] = ["inbox", "triaged", "ready", "in_progress", "validating", "blocked", "done", "cancelled"];
// Closed columns can grow to hundreds of items: show the most recent ones in pages.
const PAGED: ItemStatus[] = ["done", "cancelled"];
const PAGE = 10;
// DEFINABLE are the statuses whose plan can still be defined in the Define view.
const DEFINABLE: ItemStatus[] = ["inbox", "triaged", "ready"];

export function ItemForm({ item, projectId, onClose, initialEdit = false }: { initialEdit?: boolean; item?: Item; projectId?: string | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [launch, setLaunch] = useState<"implementation" | null>(null);
  const navigate = useNavigate();
  const [title, setTitle] = useState(item?.title ?? "");
  const [body, setBody] = useState(item?.body_md ?? "");
  const [progress, setProgress] = useState(item?.progress ?? "");
  const [refs, setRefs] = useState(item?.refs.join(", ") ?? "");
  const [tags, setTags] = useState(item?.tags.join(", ") ?? "");
  const [status, setStatus] = useState<ItemStatus>(item?.status ?? "inbox");
  const [priority, setPriority] = useState(item?.priority ?? 0);
  const [pid, setPid] = useState(item?.project_id ?? projectId ?? "");
  const [fid, setFid] = useState(item?.feature_id ?? "");
  const [reason, setReason] = useState(item?.blocked_reason ?? "");
  const [editMode, setEditMode] = useState(!item || initialEdit);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const features = useQuery({ queryKey: ["features", pid], queryFn: () => api.features.list({ project_id: pid }), enabled: !!pid });
  const save = useMutation({
    mutationFn: () => {
      const data = { title, body_md: body, progress, refs: csv(refs), tags: csv(tags), status, blocked_reason: status === "blocked" ? reason : "", priority, project_id: pid || null, feature_id: (pid && fid) || null };
      return item ? api.items.update(item.id, itemInput(item, data)) : api.items.create({ id: uuid(), ...data });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["items"] });
      onClose();
    },
  });
  const archive = useMutation({
    mutationFn: () => api.items.archive(item!.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["items"] });
      onClose();
    },
  });
  if (item && !editMode) {
    const project = projects.data?.find((p) => p.id === item.project_id);
    const feature = features.data?.find((f) => f.id === item.feature_id);
    return (
      <Modal title="Item" onClose={onClose} wide>
        <ItemView item={item} project={project?.name} feature={feature?.title} />
        {launch && <LaunchDialog items={[item]} role={launch} onClose={() => setLaunch(null)} onSuccess={onClose} />}
        <div className="sticky -bottom-3 -mx-3 flex flex-wrap justify-end gap-2 border-t border-slate-200 bg-white px-3 pt-3 dark:border-border dark:bg-surface sm:-bottom-4 sm:-mx-4 sm:px-4">
          <Button variant="ghost" onClick={() => setEditMode(true)}>Editar</Button>
          {DEFINABLE.includes(item.status) && <Button type="button" variant={item.status === "ready" ? "ghost" : "default"} onClick={() => navigate({ to: "/items/$id/define", params: { id: item.id } })}>{item.status === "ready" ? "Revisar plan" : "Definir"}</Button>}
          {item.status === "ready" && <Button type="button" onClick={() => setLaunch("implementation")}>Enviar a implementar</Button>}
        </div>
      </Modal>
    );
  }
  return (
    <Modal title={item ? "Editar item" : "Nuevo item"} onClose={onClose} wide={!!item}>
      <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus /></Label>
      <Label text="Descripción (markdown)"><Textarea value={body} onChange={(e) => setBody(e.target.value)} /></Label>
      <Label text="Progreso (estado actual)"><Input value={progress} onChange={(e) => setProgress(e.target.value)} placeholder="Rama · último commit · qué falta" /></Label>
      <Label text="Referencias (coma): PR, issue, rama"><Input value={refs} onChange={(e) => setRefs(e.target.value)} /></Label>
      <div className="grid gap-2 sm:grid-cols-2">
        <Label text="Estado">
          <Select value={status} onChange={(v) => setStatus(v as ItemStatus)} options={opts(STATUSES)} />
        </Label>
        <Label text="Prioridad"><Select value={String(priority)} onChange={(v) => setPriority(+v)} options={[{ value: "0", label: "Normal" }, { value: "1", label: "P1 · Alta" }, { value: "2", label: "P2 · Muy alta" }, { value: "3", label: "P3 · Urgente" }]} /></Label>
        <Label text="Proyecto">
          <Select value={pid} onChange={(v) => { setPid(v); setFid(""); }} options={[{ value: "", label: "— ninguno —" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
        </Label>
        <Label text="Feature">
          <Select value={fid} onChange={setFid} disabled={!pid} options={[{ value: "", label: "— ninguna —" }, ...(features.data ?? []).map((f) => ({ value: f.id, label: f.title }))]} />
        </Label>
        <Label text="Tags (coma)"><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Label>
      </div>
      {status === "blocked" && <Label text="Motivo del bloqueo"><Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Qué lo bloquea y qué lo desbloquea" /></Label>}
      <ErrorText error={save.error ?? archive.error} />
      <div className="flex justify-between">
        {item ? <Button variant="danger" onClick={() => { if (window.confirm(`¿Archivar «${item.title}»?`)) archive.mutate(); }}>Archivar</Button> : <span />}
        <div className="flex gap-2">
          <Button variant="ghost" onClick={item ? () => setEditMode(false) : onClose}>Cancelar</Button>
          <Button disabled={!title.trim() || (status === "blocked" && !reason.trim()) || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        </div>
      </div>
    </Modal>
  );
}

// BlockDialog asks why an item is blocked; it replaces the native prompt.
function BlockDialog({ item, onConfirm, onClose }: { item: Item; onConfirm: (reason: string) => void; onClose: () => void }) {
  const [reason, setReason] = useState("");
  return (
    <Modal title="Bloquear item" onClose={onClose}>
      <p className="text-sm break-words text-slate-600 dark:text-text-secondary">{item.title}</p>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (reason.trim()) onConfirm(reason.trim()); }}>
        <Label text="Motivo del bloqueo"><Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Qué lo bloquea y qué lo desbloquea" autoFocus /></Label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button disabled={!reason.trim()}>Bloquear</Button>
        </div>
      </form>
    </Modal>
  );
}

export const STATUS_INFO: Record<ItemStatus, { label: string; tone: string; hint: string }> = {
  inbox: { label: "Inbox", tone: "bg-slate-100 text-slate-700 dark:bg-raised dark:text-text-secondary", hint: "Idea capturada, sin revisar." },
  triaged: { label: "En definición", tone: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-300", hint: "Conversando con el agente hasta cerrar el plan." },
  ready: { label: "Listo", tone: "bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-300", hint: "Definido: un agente puede implementarlo." },
  in_progress: { label: "En curso", tone: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300", hint: "Alguien trabaja en él ahora." },
  validating: { label: "Validando", tone: "bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-300", hint: "Terminado, espera tu prueba." },
  blocked: { label: "Bloqueado", tone: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300", hint: "No puede avanzar hasta resolver el motivo." },
  done: { label: "Hecho", tone: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300", hint: "Confirmado y cerrado." },
  cancelled: { label: "Cancelado", tone: "bg-slate-100 text-slate-500 dark:bg-raised dark:text-muted", hint: "Descartado." },
};

const muted = "text-xs text-slate-500 dark:text-muted";
const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "medium", timeStyle: "short" });

// GitHub links read better as "PR #40" or "issue #12".
function refLabel(ref: string) {
  const m = ref.match(/github\.com\/([^/]+\/[^/]+)\/(pull|issues)\/(\d+)/);
  return m ? `${m[2] === "pull" ? "PR" : "Issue"} #${m[3]} · ${m[1]}` : ref;
}

function Section({ title, hint, children }: { title: string; hint: string; children: React.ReactNode }) {
  return (
    <section className="space-y-1.5">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        <p className={muted}>{hint}</p>
      </div>
      {children}
    </section>
  );
}

function ItemView({ item, project, feature }: { item: Item; project?: string; feature?: string }) {
  const info = STATUS_INFO[item.status];
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <h2 className="text-lg font-semibold break-words">{item.title}</h2>
        <div className="flex flex-wrap items-center gap-2">
          <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${info.tone}`}>{info.label}</span>
          {item.priority > 0 && <Badge>Prioridad {item.priority}</Badge>}
          {item.tags.map((t) => <Badge key={t}>#{t}</Badge>)}
        </div>
        <p className={muted}>{info.hint}</p>
      </div>
      {item.status === "blocked" && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm dark:border-red-900 dark:bg-red-950/40">
          <div className="font-medium text-red-800 dark:text-red-300">Motivo del bloqueo</div>
          <p className="text-red-700 dark:text-red-300">{item.blocked_reason || "Sin motivo indicado."}</p>
        </div>
      )}
      <dl className="grid grid-cols-2 gap-3 rounded-md bg-slate-50 p-3 text-sm dark:bg-raised/60">
        <div><dt className={muted}>Proyecto</dt><dd>{project ?? "—"}</dd></div>
        <div><dt className={muted}>Feature</dt><dd className="break-words">{feature ?? "—"}</dd></div>
        <div><dt className={muted}>Creado</dt><dd>{date(item.created_at)}</dd></div>
        <div><dt className={muted}>Actualizado</dt><dd>{date(item.updated_at)}</dd></div>
      </dl>
      <Section title="Progreso" hint="Estado actual en una línea. El agente lo reemplaza al avanzar para que otro pueda retomar.">
        {item.progress ? <p className="rounded-md border-l-4 border-amber-400 bg-amber-50 px-3 py-2 text-sm dark:bg-amber-950/30">{item.progress}</p> : <p className={muted}>Sin progreso registrado.</p>}
      </Section>
      <Section title="Descripción" hint="Objetivo, alcance y cómo probarlo. Es lo que lee el agente para trabajar.">
        {item.body_md ? <div className="rounded-md border border-slate-200 p-3 dark:border-border"><Md>{item.body_md}</Md></div> : <p className={muted}>Sin descripción.</p>}
      </Section>
      <Section title="Referencias" hint="Enlaces estables: pull requests, issues y ramas.">
        {item.refs.length ? (
          <ul className="space-y-1 text-sm">
            {item.refs.map((r) => {
              const pull = /^https:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)$/.exec(r);
              return <li key={r}>{pull ? (
                <Link className="break-all underline" to="/github/$owner/$repo/$kind/$number" params={{ owner: pull[1], repo: pull[2], kind: "pulls", number: pull[3] }}>{refLabel(r)}</Link>
              ) : /^https?:\/\//.test(r) ? <a className="break-all underline" href={r} target="_blank" rel="noreferrer">{refLabel(r)}</a> : <code className="break-all rounded bg-slate-100 px-1 dark:bg-raised">{r}</code>}</li>;
            })}
          </ul>
        ) : <p className={muted}>Sin referencias.</p>}
      </Section>
      <Section title="Agentes" hint="Hilos de T3 Code enlazados a este item.">
        <div><ThreadBadges itemId={item.id} /></div>
      </Section>
    </div>
  );
}

export function Inbox() {
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const [quick, setQuick] = useState("");
  const [batch, setBatch] = useState(false);
  const qc = useQueryClient();
  const project = useProject();
  const items = useQuery({ queryKey: ["items", "inbox"], queryFn: () => api.items.list({ status: "inbox" }), select: (l) => (project ? l.filter((i) => i.project_id === project) : l) });
  const add = useMutation({
    mutationFn: (title: string) => api.items.create({ id: uuid(), title, raw_text: title, source: "web", project_id: project || null }),
    onSuccess: () => {
      setQuick("");
      qc.invalidateQueries({ queryKey: ["items"] });
    },
  });
  return (
    <div className="space-y-3">
      <PageHeader title="Inbox" subtitle="Ideas capturadas pendientes de revisar">
        {!!items.data?.length && <Button variant="ghost" onClick={() => setBatch(true)}>Definir todo</Button>}
        <Button onClick={() => setEditing("new")}>Nuevo item</Button>
      </PageHeader>
      {batch && items.data && <LaunchDialog items={items.data} role="refine" onClose={() => setBatch(false)} />}
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (quick.trim()) add.mutate(quick.trim()); }}>
        <Input placeholder="Captura rápida…" value={quick} onChange={(e) => setQuick(e.target.value)} />
        <Button disabled={add.isPending}>Añadir</Button>
      </form>
      <ErrorText error={add.error ?? items.error} />
      <div className="space-y-2">
        {items.data?.map((i) => (
          <ItemActions key={i.id} item={i}><Card className="cursor-pointer hover:bg-slate-50 dark:hover:bg-raised">
            <div onClick={() => setEditing(i)}>
              <div className="flex flex-wrap items-center gap-2"><span className="font-medium break-words">{i.title}</span>{i.tags.map((t) => <Badge key={t}>{t}</Badge>)}</div>
              {i.progress && <p className="text-xs text-slate-500 dark:text-muted">{i.progress}</p>}
              {i.body_md && <p className="line-clamp-2 text-sm text-slate-600 dark:text-text-secondary">{i.body_md.replace(/[#*`>\[\]]/g, "")}</p>}
            </div>
          </Card></ItemActions>
        ))}
        {items.isPending && <Notice loading />}
        {items.data?.length === 0 && <Notice>Inbox vacío. Captura una idea arriba.</Notice>}
      </div>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} projectId={project || null} onClose={() => setEditing(null)} />}
    </div>
  );
}

function Column({ status, count, collapsed, onToggle, children }: { status: ItemStatus; count: number; collapsed?: boolean; onToggle?: () => void; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  const info = STATUS_INFO[status];
  return (
    <div ref={setNodeRef} className={`w-full space-y-2 rounded-lg p-2 ${collapsed ? "sm:w-40" : "min-h-24 sm:min-h-48 sm:w-64 xl:w-auto xl:min-w-0 xl:flex-1"} sm:shrink-0 ${collapsed ? "xl:flex-none" : ""} ${isOver ? "bg-blue-100 dark:bg-accent-subtle" : "bg-slate-100 dark:bg-raised"}`}>
      <div className="flex items-center justify-between gap-2" title={info.hint}>
        <h3 className="text-sm font-semibold text-slate-700 dark:text-text-secondary">{info.label}</h3>
        <div className="flex items-center gap-1">
          <span className={`rounded-full px-2 text-xs font-medium ${info.tone}`}>{count}</span>
          {onToggle && <button type="button" aria-expanded={!collapsed} aria-label={collapsed ? `Mostrar ${info.label}` : `Ocultar ${info.label}`} onClick={onToggle} className="rounded px-1 text-xs text-slate-500 hover:bg-slate-200 dark:text-muted dark:hover:bg-hover">{collapsed ? "▸" : "▾"}</button>}
        </div>
      </div>
      {!collapsed && children}
    </div>
  );
}

function Draggable({ item, onOpen }: { item: Item; onOpen: () => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id });
  const style = transform ? { transform: `translate(${transform.x}px, ${transform.y}px)` } : undefined;
  return (
    <div ref={setNodeRef} style={style} {...listeners} {...attributes} className={isDragging ? "relative z-10 opacity-80" : ""}>
      <ItemActions item={item}><Card className={`cursor-grab ${item.status === "blocked" ? "border-l-4 border-l-red-500 dark:border-l-red-500" : ""}`}><div onClick={onOpen} className="text-sm"><div className="flex items-start justify-between gap-2"><span className="font-medium break-words">{item.title}</span></div>{item.priority > 0 && <Badge>P{item.priority}</Badge>}<CardLinks item={item} />{item.status === "blocked" && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{item.blocked_reason}</p>}</div></Card></ItemActions>
    </div>
  );
}

// Board shows the kanban of one project, or of every project when projectId is empty.
export function Board({ projectId }: { projectId: string }) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const [blocking, setBlocking] = useState<Item | null>(null);
  const [folded, setFolded] = useState<Set<ItemStatus>>(new Set(PAGED));
  const [limits, setLimits] = useState<Partial<Record<ItemStatus, number>>>({});
  const key = ["items", "board", projectId];
  const items = useQuery({ queryKey: key, queryFn: () => api.items.list(projectId ? { project_id: projectId } : {}) });
  const move = useMutation({
    mutationFn: ({ item, status, reason }: { item: Item; status: ItemStatus; reason: string }) => api.items.update(item.id, itemInput(item, { status, blocked_reason: reason })),
    onMutate: ({ item, status }) => {
      qc.setQueryData<Item[]>(key, (old) => old?.map((i) => (i.id === item.id ? { ...i, status, updated_at: new Date().toISOString() } : i)));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["items"] }),
  });
  // Touch: hold to drag so the page can still scroll; a tap opens the card.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }), useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }));
  const onDragEnd = (e: DragEndEvent) => {
    const item = items.data?.find((i) => i.id === e.active.id);
    const status = e.over?.id as ItemStatus | undefined;
    if (!item || !status || status === item.status) return;
    // Blocking needs a reason; leaving blocked clears it.
    if (status === "blocked") { setBlocking(item); return; }
    move.mutate({ item, status, reason: "" });
  };
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Kanban <span className="hidden text-xs font-normal text-slate-500 dark:text-muted sm:inline">(clic para abrir · arrastra para cambiar de estado)</span></h2>
        <Button onClick={() => setEditing("new")} aria-label="Nuevo item" title="Nuevo item" className="inline-flex shrink-0 items-center gap-1.5"><Plus aria-hidden="true" className="size-4" /><span className="hidden sm:inline">Nuevo item</span></Button>
      </div>
      {blocking && <BlockDialog item={blocking} onClose={() => setBlocking(null)} onConfirm={(reason) => { move.mutate({ item: blocking, status: "blocked", reason }); setBlocking(null); }} />}
      <ErrorText error={move.error ?? items.error} />
      <DndContext sensors={sensors} onDragEnd={onDragEnd}>
        <div className="flex flex-col gap-3 pb-2 sm:flex-row sm:overflow-x-auto">
          {STATUSES.map((s) => {
            const col = items.data?.filter((i) => i.status === s) ?? [];
            const paged = PAGED.includes(s);
            const limit = limits[s] ?? PAGE;
            const shown = paged ? col.sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, limit) : col.sort((a, b) => b.priority - a.priority);
            return (
              <Column key={s} status={s} count={col.length} collapsed={paged && folded.has(s)} onToggle={paged ? () => setFolded((f) => { const n = new Set(f); if (n.has(s)) n.delete(s); else n.add(s); return n; }) : undefined}>
                {shown.map((i) => <Draggable key={i.id} item={i} onOpen={() => setEditing(i)} />)}
                {paged && col.length > limit && (
                  <Button variant="ghost" className="w-full" onClick={() => setLimits((l) => ({ ...l, [s]: limit + PAGE }))}>Cargar más ({col.length - limit})</Button>
                )}
              </Column>
            );
          })}
        </div>
      </DndContext>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} projectId={projectId || null} onClose={() => setEditing(null)} />}
    </div>
  );
}

// Shows the T3 threads linked to an item; both queries are shared by every card.
function useItemThreads(itemId: string) {
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 10_000 });
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 10_000 });
  return links.data?.filter((l) => l.item_id === itemId).flatMap((l) => {
    const thread = t3.data?.flatMap((s) => s.threads).find((t) => t.id === l.thread_id);
    return thread ? [thread] : [];
  }) ?? [];
}

function ThreadBadges({ itemId }: { itemId: string }) {
  return useItemThreads(itemId).map((t) => (
    <ThreadLink key={t.id} url={t.url} environmentId={t.t3_environment_id} threadId={t.id} title={t.title} onPointerDown={(e) => e.stopPropagation()} className="mr-1 inline-flex items-center gap-1 text-xs text-slate-500 underline dark:text-muted">
      <ThreadStatus status={t.status} attention={t.attention} />
      <span>{t.model}</span>
    </ThreadLink>
  ));
}

function dotFor(status: string, attention: boolean): { label: string; color: string; pulse: boolean } {
  const s = status.toLowerCase();
  if (["failed", "error"].includes(s)) return { label: "Error", color: "bg-red-500", pulse: false };
  if (attention || s === "attention") return { label: "Te necesita", color: "bg-amber-500", pulse: true };
  if (["running", "working"].includes(s)) return { label: "Trabajando", color: "bg-blue-500", pulse: true };
  if (["completed", "complete", "done"].includes(s)) return { label: "Listo", color: "bg-green-500", pulse: false };
  return { label: status || "Estado desconocido", color: "bg-slate-400", pulse: false };
}

// Kanban card: only the most recent thread (status dot) and the last PR ref.
function CardLinks({ item }: { item: Item }) {
  const threads = useItemThreads(item.id);
  const t = threads.length ? threads.reduce((a, b) => (b.updated_at >= a.updated_at ? b : a)) : undefined;
  const pull = [...(item.refs ?? [])].reverse().map((r) => /^https:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)$/.exec(r)).find(Boolean);
  if (!t && !pull) return null;
  const dot = t ? dotFor(t.status, t.attention) : undefined;
  const chip = "inline-flex max-w-full items-center gap-1.5 rounded-full border border-slate-200 px-2 py-0.5 text-xs text-slate-600 no-underline hover:bg-slate-100 dark:border-border dark:text-muted dark:hover:bg-white/5";
  return (
    <div className="mt-1 flex flex-wrap items-center gap-1" onClick={(e) => e.stopPropagation()}>
      {t && dot && (
        <ThreadLink url={t.url} environmentId={t.t3_environment_id} threadId={t.id} title={`${t.title} · ${dot.label}`} aria-label={`${t.title}: ${dot.label}`} onPointerDown={(e) => e.stopPropagation()} className={chip}>
          <ChatCircle aria-hidden="true" className="size-3 shrink-0" />
          <span className="min-w-0 truncate">{t.model}</span>
          {t.effort && <span className="shrink-0 opacity-60">· {t.effort}</span>}
          <span role="img" title={dot.label} aria-label={dot.label} className={`size-2 shrink-0 rounded-full ${dot.color} ${dot.pulse ? "motion-safe:animate-pulse" : ""}`} />
        </ThreadLink>
      )}
      {pull && (
        <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: pull[1], repo: pull[2], kind: "pulls", number: pull[3] }} title={`Pull request #${pull[3]}`} aria-label={`Pull request #${pull[3]}`} onPointerDown={(e) => e.stopPropagation()} className={chip}>
          <GitPullRequest aria-hidden="true" className="size-3 shrink-0" />
          <span>#{pull[3]}</span>
        </Link>
      )}
    </div>
  );
}

// Shared by kanban cards and inbox list rows; only these surfaces suppress the native menu.
function ItemActions({ item, children }: { item: Item; children: React.ReactNode }) {
  const qc = useQueryClient();
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null);
  const [detail, setDetail] = useState<"view" | "edit" | null>(null);
  const [launch, setLaunch] = useState<"implementation" | null>(null);
  const navigate = useNavigate();
  const [blocking, setBlocking] = useState(false);
  const close = useCallback(() => setPosition(null), []);
  const threads = useItemThreads(item.id);
  const thread = threads.reduce<(typeof threads)[number] | undefined>((a, b) => !a || b.updated_at >= a.updated_at ? b : a, undefined);
  const pull = [...item.refs].reverse().map((r) => /^https:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)$/.exec(r)).find(Boolean);
  const change = useMutation({
    mutationFn: async ({ status, reason = "" }: { status: ItemStatus; reason?: string }) => {
      await api.items.update(item.id, itemInput(item, { status, blocked_reason: reason }));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["items"] }),
  });
  const archive = useMutation({ mutationFn: () => api.items.archive(item.id), onSuccess: () => qc.invalidateQueries({ queryKey: ["items"] }) });
  const linkClass = "block rounded px-3 py-2 hover:bg-slate-100 focus:bg-slate-100 dark:hover:bg-raised dark:focus:bg-raised";
  const actions: ContextAction[] = [
    { label: "Abrir detalle", run: () => setDetail("view") },
    { label: "Editar", run: () => setDetail("edit") },
    ...(DEFINABLE.includes(item.status) ? [{ label: item.status === "ready" ? "Revisar plan" : "Definir", run: () => navigate({ to: "/items/$id/define", params: { id: item.id } }) }] : []),
    ...(item.status === "ready" ? [{ label: "Enviar a implementar", run: () => setLaunch("implementation") }] : []),
    { label: "Mover a estado…", children: STATUSES.filter((s) => s !== item.status).map((s) => ({ label: STATUS_INFO[s].label, run: () => (s === "blocked" ? setBlocking(true) : change.mutate({ status: s })) })) },
    ...(thread ? [{ label: "Abrir último hilo", content: <ThreadLink role="menuitem" className={linkClass} url={thread.url} environmentId={thread.t3_environment_id} threadId={thread.id}>Abrir último hilo</ThreadLink> }] : []),
    ...(pull ? [{ label: "Abrir última PR", content: <Link role="menuitem" className={linkClass} to="/github/$owner/$repo/$kind/$number" params={{ owner: pull[1], repo: pull[2], kind: "pulls", number: pull[3] }}>Abrir última PR</Link> }] : []),
    { label: "Archivar", run: () => { if (window.confirm(`¿Archivar «${item.title}»?`)) archive.mutate(); } },
  ];
  return <div onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); setPosition({ x: e.clientX, y: e.clientY }); }}>
    {children}
    <ErrorText error={change.error ?? archive.error} />
    {position && <ContextMenu {...position} actions={actions} onClose={close} />}
    {detail && <ItemForm item={item} initialEdit={detail === "edit"} onClose={() => setDetail(null)} />}
    {launch && <LaunchDialog items={[item]} role={launch} onClose={() => setLaunch(null)} />}
    {blocking && <BlockDialog item={item} onClose={() => setBlocking(false)} onConfirm={(reason) => { change.mutate({ status: "blocked", reason }); setBlocking(false); }} />}
  </div>;
}
