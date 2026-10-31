import { LaunchDialog } from "./launch";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DndContext, type DragEndEvent, PointerSensor, TouchSensor, useDraggable, useDroppable, useSensor, useSensors } from "@dnd-kit/core";
import { api, itemInput, type Item, type ItemStatus } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, field, Label, Md, Modal, Textarea, csv } from "../components/ui";

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
  return (
    <Modal title={item ? "Editar item" : "Nuevo item"} onClose={onClose}>
      <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus /></Label>
      <Label text="Descripción (markdown)"><Textarea value={body} onChange={(e) => setBody(e.target.value)} /></Label>
      <Label text="Progreso (estado actual)"><Input value={progress} onChange={(e) => setProgress(e.target.value)} placeholder="Rama · último commit · qué falta" /></Label>
      <Label text="Referencias (coma): PR, issue, rama"><Input value={refs} onChange={(e) => setRefs(e.target.value)} /></Label>
      <div className="grid gap-2 sm:grid-cols-2">
        <Label text="Estado">
          <select className={field} value={status} onChange={(e) => setStatus(e.target.value as ItemStatus)}>
            {STATUSES.map((s) => <option key={s}>{s}</option>)}
          </select>
        </Label>
        <Label text="Prioridad"><Input type="number" value={priority} onChange={(e) => setPriority(+e.target.value)} /></Label>
        <Label text="Proyecto">
          <select className={field} value={pid} onChange={(e) => { setPid(e.target.value); setFid(""); }}>
            <option value="">— ninguno —</option>
            {projects.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        </Label>
        <Label text="Feature">
          <select className={field} value={fid} onChange={(e) => setFid(e.target.value)} disabled={!pid}>
            <option value="">— ninguna —</option>
            {features.data?.map((f) => <option key={f.id} value={f.id}>{f.title}</option>)}
          </select>
        </Label>
        <Label text="Tags (coma)"><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Label>
      </div>
      {status === "blocked" && <Label text="Motivo del bloqueo"><Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Qué lo bloquea y qué lo desbloquea" /></Label>}
      {item && (item.status === "inbox" || item.status === "triaged") && <Button type="button" variant="ghost" onClick={() => setLaunch("triage")}>Analizar con un agente</Button>}
      {item?.status === "ready" && <Button type="button" variant="ghost" onClick={() => setLaunch("implementation")}>Enviar a implementar</Button>}
      {item && launch && <LaunchDialog items={[item]} role={launch} onClose={() => setLaunch(null)} />}
      <ErrorText error={save.error ?? archive.error} />
      <div className="flex justify-between">
        {item ? <Button variant="danger" onClick={() => archive.mutate()}>Archivar</Button> : <span />}
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button disabled={!title.trim() || (status === "blocked" && !reason.trim()) || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        </div>
      </div>
    </Modal>
  );
}

export function Inbox() {
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const [quick, setQuick] = useState("");
  const [batch, setBatch] = useState(false);
  const qc = useQueryClient();
  const items = useQuery({ queryKey: ["items", "inbox"], queryFn: () => api.items.list({ status: "inbox" }) });
  const add = useMutation({
    mutationFn: (title: string) => api.items.create({ id: crypto.randomUUID(), title, raw_text: title, source: "web" }),
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
          <Card key={i.id} className="cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-800">
            <div onClick={() => setEditing(i)}>
              <div className="flex flex-wrap items-center gap-2"><span className="font-medium break-words">{i.title}</span>{i.tags.map((t) => <Badge key={t}>{t}</Badge>)}</div>
              {i.progress && <p className="text-xs text-slate-500 dark:text-slate-400">{i.progress}</p>}
              {i.body_md && <Md>{i.body_md}</Md>}
            </div>
          </Card>
        ))}
        {items.data?.length === 0 && <p className="text-sm text-slate-500 dark:text-slate-400">Inbox vacío.</p>}
      </div>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

function Column({ status, children }: { status: ItemStatus; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  return (
    <div ref={setNodeRef} className={`min-h-24 w-full space-y-2 sm:min-h-48 sm:w-64 sm:shrink-0 xl:w-auto xl:min-w-0 xl:flex-1 rounded-lg p-2 ${isOver ? "bg-blue-100 dark:bg-blue-950" : "bg-slate-100 dark:bg-slate-800"}`}>
      <h3 className="text-sm font-semibold uppercase text-slate-600 dark:text-slate-300">{status.replace("_", " ")}</h3>
      {children}
    </div>
  );
}

function Draggable({ item, onOpen }: { item: Item; onOpen: () => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id });
  const style = transform ? { transform: `translate(${transform.x}px, ${transform.y}px)` } : undefined;
  return (
    <div ref={setNodeRef} style={style} {...listeners} {...attributes} className={isDragging ? "relative z-10 opacity-80" : ""}>
      <Card className="cursor-grab"><div onDoubleClick={onOpen} className="text-sm"><div className="flex items-start justify-between gap-2"><span className="font-medium break-words">{item.title}</span><button className="-m-2 size-11 shrink-0 text-slate-500 dark:text-slate-400 sm:hidden" aria-label="Editar" onPointerDown={(e) => e.stopPropagation()} onClick={onOpen}>✎</button></div>{item.priority > 0 && <Badge>P{item.priority}</Badge>}<ThreadBadges itemId={item.id} />{item.status === "blocked" && <p className="text-xs text-red-600 dark:text-red-400">{item.blocked_reason}</p>}</div></Card>
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
        <h2 className="font-semibold">Kanban <span className="hidden text-xs font-normal text-slate-500 dark:text-slate-400 sm:inline">(doble clic para editar)</span></h2>
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
    <a key={t.id} href={t.url} target="_blank" rel="noreferrer" title={t.title} onPointerDown={(e) => e.stopPropagation()} className={`mr-1 inline-block text-xs underline ${t.attention ? "text-red-600 dark:text-red-400" : "text-slate-500 dark:text-slate-400"}`}>
      {t.attention ? "Te necesita" : `${t.model} · ${t.status}`}
    </a>
  ));
}
