import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DndContext, type DragEndEvent, useDraggable, useDroppable } from "@dnd-kit/core";
import { api, itemInput, type Item, type ItemStatus } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, Textarea, csv } from "../components/ui";

const STATUSES: ItemStatus[] = ["inbox", "triaged", "ready", "in_progress", "done"];

export function ItemForm({ item, projectId, onClose }: { item?: Item; projectId?: string | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [title, setTitle] = useState(item?.title ?? "");
  const [body, setBody] = useState(item?.body_md ?? "");
  const [tags, setTags] = useState(item?.tags.join(", ") ?? "");
  const [status, setStatus] = useState<ItemStatus>(item?.status ?? "inbox");
  const [priority, setPriority] = useState(item?.priority ?? 0);
  const [pid, setPid] = useState(item?.project_id ?? projectId ?? "");
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const save = useMutation({
    mutationFn: () => {
      const data = { title, body_md: body, tags: csv(tags), status, priority, project_id: pid || null };
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
      <div className="grid grid-cols-2 gap-2">
        <Label text="Estado">
          <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={status} onChange={(e) => setStatus(e.target.value as ItemStatus)}>
            {[...STATUSES, "archived" as const].map((s) => <option key={s}>{s}</option>)}
          </select>
        </Label>
        <Label text="Prioridad"><Input type="number" value={priority} onChange={(e) => setPriority(+e.target.value)} /></Label>
        <Label text="Proyecto">
          <select className="w-full rounded-md border border-slate-300 px-2 py-1.5 text-sm" value={pid} onChange={(e) => setPid(e.target.value)}>
            <option value="">— ninguno —</option>
            {projects.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        </Label>
        <Label text="Tags (coma)"><Input value={tags} onChange={(e) => setTags(e.target.value)} /></Label>
      </div>
      <ErrorText error={save.error ?? archive.error} />
      <div className="flex justify-between">
        {item ? <Button variant="danger" onClick={() => archive.mutate()}>Archivar</Button> : <span />}
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button disabled={!title.trim() || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        </div>
      </div>
    </Modal>
  );
}

export function Inbox() {
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const [quick, setQuick] = useState("");
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
        <Button onClick={() => setEditing("new")}>Nuevo item</Button>
      </div>
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (quick.trim()) add.mutate(quick.trim()); }}>
        <Input placeholder="Captura rápida…" value={quick} onChange={(e) => setQuick(e.target.value)} />
        <Button disabled={add.isPending}>Añadir</Button>
      </form>
      <ErrorText error={add.error ?? items.error} />
      <div className="space-y-2">
        {items.data?.map((i) => (
          <Card key={i.id} className="cursor-pointer hover:bg-slate-50">
            <div onClick={() => setEditing(i)}>
              <div className="flex items-center gap-2"><span className="font-medium">{i.title}</span>{i.tags.map((t) => <Badge key={t}>{t}</Badge>)}</div>
              {i.body_md && <Md>{i.body_md}</Md>}
            </div>
          </Card>
        ))}
        {items.data?.length === 0 && <p className="text-sm text-slate-500">Inbox vacío.</p>}
      </div>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

function Column({ status, children }: { status: ItemStatus; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  return (
    <div ref={setNodeRef} className={`min-h-48 w-64 shrink-0 space-y-2 rounded-lg p-2 ${isOver ? "bg-blue-100" : "bg-slate-100"}`}>
      <h3 className="text-sm font-semibold uppercase text-slate-600">{status.replace("_", " ")}</h3>
      {children}
    </div>
  );
}

function Draggable({ item, onOpen }: { item: Item; onOpen: () => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id });
  const style = transform ? { transform: `translate(${transform.x}px, ${transform.y}px)` } : undefined;
  return (
    <div ref={setNodeRef} style={style} {...listeners} {...attributes} className={isDragging ? "relative z-10 opacity-80" : ""}>
      <Card className="cursor-grab"><div onDoubleClick={onOpen} className="text-sm"><div className="font-medium">{item.title}</div>{item.priority > 0 && <Badge>P{item.priority}</Badge>}</div></Card>
    </div>
  );
}

export function Board({ projectId }: { projectId: string }) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Item | "new" | null>(null);
  const key = ["items", "board", projectId];
  const items = useQuery({ queryKey: key, queryFn: () => api.items.list({ project_id: projectId }) });
  const move = useMutation({
    mutationFn: ({ item, status }: { item: Item; status: ItemStatus }) => api.items.update(item.id, itemInput(item, { status })),
    onMutate: ({ item, status }) => {
      qc.setQueryData<Item[]>(key, (old) => old?.map((i) => (i.id === item.id ? { ...i, status } : i)));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["items"] }),
  });
  const onDragEnd = (e: DragEndEvent) => {
    const item = items.data?.find((i) => i.id === e.active.id);
    const status = e.over?.id as ItemStatus | undefined;
    if (item && status && status !== item.status) move.mutate({ item, status });
  };
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Kanban <span className="text-xs font-normal text-slate-500">(doble clic para editar)</span></h2>
        <Button onClick={() => setEditing("new")}>Nuevo item</Button>
      </div>
      <ErrorText error={move.error ?? items.error} />
      <DndContext onDragEnd={onDragEnd}>
        <div className="flex gap-3 overflow-x-auto pb-2">
          {STATUSES.map((s) => (
            <Column key={s} status={s}>
              {items.data?.filter((i) => i.status === s).sort((a, b) => b.priority - a.priority).map((i) => <Draggable key={i.id} item={i} onOpen={() => setEditing(i)} />)}
            </Column>
          ))}
        </div>
      </DndContext>
      {editing && <ItemForm item={editing === "new" ? undefined : editing} projectId={projectId} onClose={() => setEditing(null)} />}
    </div>
  );
}
