import { ThreadStatus } from "../components/thread-status";
import { ThreadLink } from "../components/thread-link";
import { LaunchDialog } from "./launch";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DndContext, type DragEndEvent, PointerSensor, TouchSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import { api, itemInput, type Item, type ItemStatus } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, Select, Textarea, csv, opts } from "../components/ui";
import { useProject } from "../project";

const STATUSES: ItemStatus[] = ["inbox", "triaged", "ready", "in_progress", "validating", "blocked", "done", "cancelled"];
// Closed columns can grow to hundreds of items: show the most recent ones in pages.
const PAGED: ItemStatus[] = ["done", "cancelled"];
const PAGE = 10;

export function ItemForm({ item, projectId, onClose }: { item?: Item; projectId?: string | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [launch, setLaunch] = useState<"triage" | "implementation" | null>(null);
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
  const [editMode, setEditMode] = useState(!item);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const features = useQuery({ queryKey: ["features", pid], queryFn: () => api.features.list({ project_id: pid }), enabled: !!pid });
  const save = useMutation({
    mutationFn: () => {
      const data = { title, body_md: body, progress, refs: csv(refs), tags: csv(tags), status, blocked_reason: status === "blocked" ? reason : "", priority, project_id: pid || null, feature_id: (pid && fid) || null };
      return item ? api.items.update(item.id, itemInput(item, data)) : api.items.create({ id: crypto.randomUUID(), ...data });
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
      <Modal title="Item" onClose={onClose}>
        <ItemView item={item} project={project?.name} feature={feature?.title} />
        {(item.status === "inbox" || item.status === "triaged") && <Button type="button" variant="ghost" onClick={() => setLaunch("triage")}>Analizar con un agente</Button>}
        {item.status === "ready" && <Button type="button" variant="ghost" onClick={() => setLaunch("implementation")}>Enviar a implementar</Button>}
        {launch && <LaunchDialog items={[item]} role={launch} onClose={() => setLaunch(null)} />}
        <div className="flex justify-end gap-2 border-t border-slate-200 pt-3 dark:border-border">
          <Button variant="ghost" onClick={onClose}>Cerrar</Button>
          <Button onClick={() => setEditMode(true)}>Editar</Button>
        </div>
      </Modal>
    );
  }
  return (
    <Modal title={item ? "Editar item" : "Nuevo item"} onClose={onClose}>
      <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus /></Label>
      <Label text="Descripción (markdown)"><Textarea value={body} onChange={(e) => setBody(e.target.value)} /></Label>
      <Label text="Progreso (estado actual)"><Input value={progress} onChange={(e) => setProgress(e.target.value)} placeholder="Rama · último commit · qué falta" /></Label>
      <Label text="Referencias (coma): PR, issue, rama"><Input value={refs} onChange={(e) => setRefs(e.target.value)} /></Label>
      <div className="grid gap-2 sm:grid-cols-2">
        <Label text="Estado">
          <Select value={status} onChange={(v) => setStatus(v as ItemStatus)} options={opts(STATUSES)} />
        </Label>
        <Label text="Prioridad"><Input type="number" value={priority} onChange={(e) => setPriority(+e.target.value)} /></Label>
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
        {item ? <Button variant="danger" onClick={() => archive.mutate()}>Archivar</Button> : <span />}
        <div className="flex gap-2">
          <Button variant="ghost" onClick={item ? () => setEditMode(false) : onClose}>Cancelar</Button>
          <Button disabled={!title.trim() || (status === "blocked" && !reason.trim()) || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        </div>
      </div>
    </Modal>
  );
}

const STATUS_INFO: Record<ItemStatus, { label: string; tone: string; hint: string }> = {
  inbox: { label: "Inbox", tone: "bg-slate-100 text-slate-700 dark:bg-raised dark:text-text-secondary", hint: "Idea capturada, sin revisar." },
  triaged: { label: "Revisado", tone: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-300", hint: "Revisado, falta definir el trabajo." },
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
            {item.refs.map((r) => <li key={r}>{/^https?:\/\//.test(r) ? <a className="break-all underline" href={r} target="_blank" rel="noreferrer">{refLabel(r)}</a> : <code className="break-all rounded bg-slate-100 px-1 dark:bg-raised">{r}</code>}</li>)}
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
    mutationFn: (title: string) => api.items.create({ id: crypto.randomUUID(), title, raw_text: title, source: "web", project_id: project || null }),
    onSuccess: () => {
      setQuick("");
      qc.invalidateQueries({ queryKey: ["items"] });
    },
  });
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Inbox</h1>
        <div className="flex gap-2">
          {!!items.data?.length && <Button variant="ghost" onClick={() => setBatch(true)}>Analizar todo</Button>}
          <Button onClick={() => setEditing("new")}>Nuevo item</Button>
        </div>
      </div>
      {batch && items.data && <LaunchDialog items={items.data} role="triage" onClose={() => setBatch(false)} />}
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (quick.trim()) add.mutate(quick.trim()); }}>
        <Input placeholder="Captura rápida…" value={quick} onChange={(e) => setQuick(e.target.value)} />
        <Button disabled={add.isPending}>Añadir</Button>
      </form>
      <ErrorText error={add.error ?? items.error} />
      <div className="space-y-2">
        {items.data?.map((i) => (
          <Card key={i.id} className="cursor-pointer hover:bg-slate-50 dark:hover:bg-raised">
            <div onClick={() => setEditing(i)}>
              <div className="flex flex-wrap items-center gap-2"><span className="font-medium break-words">{i.title}</span>{i.tags.map((t) => <Badge key={t}>{t}</Badge>)}</div>
              {i.progress && <p className="text-xs text-slate-500 dark:text-muted">{i.progress}</p>}
              {i.body_md && <Md>{i.body_md}</Md>}
            </div>
          </Card>
        ))}
        {items.data?.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Inbox vacío.</p>}
      </div>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} projectId={project || null} onClose={() => setEditing(null)} />}
    </div>
  );
}

function Column({ status, children }: { status: ItemStatus; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  return (
    <div ref={setNodeRef} className={`min-h-24 w-full space-y-2 sm:min-h-48 sm:w-64 sm:shrink-0 xl:w-auto xl:min-w-0 xl:flex-1 rounded-lg p-2 ${isOver ? "bg-blue-100 dark:bg-accent-subtle" : "bg-slate-100 dark:bg-raised"}`}>
      <h3 className="text-sm font-semibold uppercase text-slate-600 dark:text-text-secondary">{status.replace("_", " ")}</h3>
      {children}
    </div>
  );
}

function Draggable({ item, onOpen }: { item: Item; onOpen: () => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id });
  const style = transform ? { transform: `translate(${transform.x}px, ${transform.y}px)` } : undefined;
  return (
    <div ref={setNodeRef} style={style} {...listeners} {...attributes} className={isDragging ? "relative z-10 opacity-80" : ""}>
      <Card className="cursor-grab"><div onDoubleClick={onOpen} className="text-sm"><div className="flex items-start justify-between gap-2"><span className="font-medium break-words">{item.title}</span><button className="-m-2 size-11 shrink-0 text-slate-500 dark:text-muted sm:hidden" aria-label="Editar" onPointerDown={(e) => e.stopPropagation()} onClick={onOpen}>✎</button></div>{item.priority > 0 && <Badge>P{item.priority}</Badge>}<ThreadBadges itemId={item.id} />{item.status === "blocked" && <p className="text-xs text-red-600 dark:text-red-400">{item.blocked_reason}</p>}</div></Card>
    </div>
  );
}

export function Board({ projectId }: { projectId: string }) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const [limits, setLimits] = useState<Partial<Record<ItemStatus, number>>>({});
  const key = ["items", "board", projectId];
  const items = useQuery({ queryKey: key, queryFn: () => api.items.list({ project_id: projectId }) });
  const move = useMutation({
    mutationFn: ({ item, status, reason }: { item: Item; status: ItemStatus; reason: string }) => api.items.update(item.id, itemInput(item, { status, blocked_reason: reason })),
    onMutate: ({ item, status }) => {
      qc.setQueryData<Item[]>(key, (old) => old?.map((i) => (i.id === item.id ? { ...i, status, updated_at: new Date().toISOString() } : i)));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["items"] }),
  });
  // Touch: hold to drag so the page can still scroll; the ✎ button opens the editor.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }), useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }));
  const onDragEnd = (e: DragEndEvent) => {
    const item = items.data?.find((i) => i.id === e.active.id);
    const status = e.over?.id as ItemStatus | undefined;
    if (!item || !status || status === item.status) return;
    // Blocking needs a reason; leaving blocked clears it.
    const reason = status === "blocked" ? window.prompt("Motivo del bloqueo")?.trim() ?? "" : "";
    if (status === "blocked" && !reason) return;
    move.mutate({ item, status, reason });
  };
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Kanban <span className="hidden text-xs font-normal text-slate-500 dark:text-muted sm:inline">(doble clic para editar)</span></h2>
        <Button onClick={() => setEditing("new")}>Nuevo item</Button>
      </div>
      <ErrorText error={move.error ?? items.error} />
      <DndContext sensors={sensors} onDragEnd={onDragEnd}>
        <div className="flex flex-col gap-3 pb-2 sm:flex-row sm:overflow-x-auto">
          {STATUSES.map((s) => {
            const col = items.data?.filter((i) => i.status === s) ?? [];
            const paged = PAGED.includes(s);
            const limit = limits[s] ?? PAGE;
            const shown = paged ? col.sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, limit) : col.sort((a, b) => b.priority - a.priority);
            return (
              <Column key={s} status={s}>
                {shown.map((i) => <Draggable key={i.id} item={i} onOpen={() => setEditing(i)} />)}
                {paged && col.length > limit && (
                  <Button variant="ghost" className="w-full" onClick={() => setLimits((l) => ({ ...l, [s]: limit + PAGE }))}>Cargar más ({col.length - limit})</Button>
                )}
              </Column>
            );
          })}
        </div>
      </DndContext>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} projectId={projectId} onClose={() => setEditing(null)} />}
    </div>
  );
}

// Shows the T3 threads linked to an item; both queries are shared by every card.
function ThreadBadges({ itemId }: { itemId: string }) {
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 10_000 });
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 10_000 });
  const threads = links.data?.filter((l) => l.item_id === itemId).flatMap((l) => {
    const thread = t3.data?.flatMap((s) => s.threads).find((t) => t.id === l.thread_id);
    return thread ? [thread] : [];
  }) ?? [];
  return threads.map((t) => (
    <ThreadLink key={t.id} url={t.url} environmentId={t.t3_environment_id} threadId={t.id} title={t.title} onPointerDown={(e) => e.stopPropagation()} className="mr-1 inline-flex items-center gap-1 text-xs text-slate-500 underline dark:text-muted">
      <ThreadStatus status={t.status} attention={t.attention} />
      <span>{t.model}</span>
    </ThreadLink>
  ));
}
