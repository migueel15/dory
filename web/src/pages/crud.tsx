import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { AppleLogo, Desktop, LinuxLogo, Plus, WindowsLogo } from "@phosphor-icons/react";
import { api, type Memory, type MemoryInput } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, PageHeader, Select, StatusPill, Textarea, csv } from "../components/ui";
import { ProviderIcon } from "../components/launch-picker";
import { useProject } from "../project";

export const memorySearch = (s: Record<string, unknown>) => ({ q: typeof s.q === "string" ? s.q : undefined, kind: typeof s.kind === "string" ? s.kind : undefined });

const KINDS = { decision: "Decisión", lesson: "Lección", context: "Contexto" } as const;

// Memories lists the notes agents read and write, filtered by the selected
// project, kind, label and text (kept in the URL).
export function Memories() {
  const project = useProject();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { q?: string; kind?: string };
  const memories = useQuery({ queryKey: ["memories"], queryFn: () => api.memories.list() });
  const [editing, setEditing] = useState<Memory | "new">();
  const q = (search.q ?? "").toLowerCase();
  const shown = (memories.data ?? [])
    .filter((m) => !project || m.scope !== "project" || m.scope_id === project)
    .filter((m) => !search.kind || m.kind === search.kind)
    .filter((m) => !q || m.title.toLowerCase().includes(q) || m.content.toLowerCase().includes(q) || m.labels.some((l) => l.toLowerCase().includes(q)))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const set = (patch: Record<string, string | undefined>) => navigate({ to: "/memories", search: { ...search, ...patch } as never, replace: true });
  return (
    <div className="space-y-3">
      <PageHeader title="Memorias" subtitle="Lo que los agentes recuerdan entre sesiones. Se replica entre nodos."><Button onClick={() => setEditing("new")}><Plus className="size-4" />Nueva</Button></PageHeader>
      <div className="flex flex-wrap gap-2">
        <Input className="max-w-xs" value={search.q ?? ""} onChange={(e) => set({ q: e.target.value || undefined })} placeholder="Buscar por texto o label" />
        <Select value={search.kind ?? ""} onChange={(v) => set({ kind: v || undefined })} options={[{ value: "", label: "Todos los tipos" }, ...Object.entries(KINDS).map(([value, label]) => ({ value, label }))]} />
      </div>
      <ErrorText error={memories.error} />
      <div className="space-y-2">
        {shown.map((m) => (
          <Card key={m.id}>
            <button type="button" className="block w-full text-left" onClick={() => setEditing(m)}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{m.title || m.content.split("\n")[0].slice(0, 80)}</span>
                <StatusPill tone={m.kind === "decision" ? "blue" : m.kind === "lesson" ? "amber" : "slate"}>{KINDS[m.kind]}</StatusPill>
                <span className="text-xs text-slate-500">{m.scope === "global" ? "global" : m.scope === "node" ? "de nodo" : "de proyecto"}</span>
                {m.labels.map((l) => <Badge key={l}>{l}</Badge>)}
              </div>
              <div className="mt-1 line-clamp-4 text-sm text-slate-600 dark:text-muted"><Md>{m.content}</Md></div>
            </button>
          </Card>
        ))}
      </div>
      {editing && <MemoryForm memory={editing === "new" ? undefined : editing} onClose={() => setEditing(undefined)} />}
    </div>
  );
}

function MemoryForm({ memory, onClose }: { memory?: Memory; onClose: () => void }) {
  const qc = useQueryClient();
  const project = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const [m, setM] = useState<MemoryInput>(memory ?? { scope: project ? "project" : "global", scope_id: project, kind: "context", title: "", content: "", labels: [] });
  const [labels, setLabels] = useState((memory?.labels ?? []).join(", "));
  const save = useMutation({
    mutationFn: () => {
      const body = { ...m, labels: csv(labels), scope_id: m.scope === "global" ? "" : m.scope_id };
      return memory ? api.memories.update(memory.id, body) : api.memories.create(body);
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["memories"] }); onClose(); },
  });
  const archive = useMutation({ mutationFn: () => api.memories.archive(memory!.id), onSuccess: () => { qc.invalidateQueries({ queryKey: ["memories"] }); onClose(); } });
  return (
    <Modal title={memory ? "Memoria" : "Nueva memoria"} onClose={onClose} wide>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (m.content.trim()) save.mutate(); }}>
        <Label text="Título"><Input value={m.title ?? ""} onChange={(e) => setM({ ...m, title: e.target.value })} /></Label>
        <Label text="Contenido"><Textarea rows={10} value={m.content} onChange={(e) => setM({ ...m, content: e.target.value })} /></Label>
        <div className="grid gap-2 sm:grid-cols-3">
          <Label text="Tipo"><Select value={m.kind ?? "context"} onChange={(v) => setM({ ...m, kind: v as Memory["kind"] })} options={Object.entries(KINDS).map(([value, label]) => ({ value, label }))} /></Label>
          <Label text="Ámbito"><Select value={m.scope ?? "global"} onChange={(v) => setM({ ...m, scope: v as Memory["scope"] })} options={[{ value: "global", label: "Global" }, { value: "project", label: "Proyecto" }]} /></Label>
          {m.scope === "project" && <Label text="Proyecto"><Select value={m.scope_id ?? ""} onChange={(v) => setM({ ...m, scope_id: v })} options={(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))} /></Label>}
        </div>
        <Label text="Labels (separadas por comas)"><Input value={labels} onChange={(e) => setLabels(e.target.value)} /></Label>
        <ErrorText error={save.error ?? archive.error} />
        <div className="flex gap-2">
          <Button disabled={!m.content.trim() || save.isPending}>Guardar</Button>
          {memory && <Button type="button" variant="ghost" className="ml-auto" onClick={() => confirm("¿Archivar la memoria?") && archive.mutate()}>Archivar</Button>}
        </div>
      </form>
    </Modal>
  );
}

const osIcon = (os = "") => (os === "darwin" ? AppleLogo : os === "windows" ? WindowsLogo : os === "linux" ? LinuxLogo : Desktop);

// Environments lists the nodes of the mesh: this one and its peers, with
// their harnesses (requirements RF-06, RF-07).
export function Environments() {
  const node = useQuery({ queryKey: ["node"], queryFn: api.node, refetchInterval: 15_000 });
  const n = node.data;
  const OsIcon = osIcon(n?.node.os);
  return (
    <div className="space-y-3">
      <PageHeader title="Entornos" subtitle="Los nodos de Dory en tus máquinas." />
      <ErrorText error={node.error} />
      {n && (
        <Card>
          <div className="flex flex-wrap items-center gap-2">
            <OsIcon className="size-5" />
            <span className="font-semibold">{n.node.name}</span>
            <StatusPill tone="green">este nodo</StatusPill>
            <span className="font-mono text-xs text-slate-500">{n.node.platform} · {n.node.version} · {n.node.listen}</span>
          </div>
          <p className="mt-1 font-mono text-xs text-slate-500">id {n.node.id} · datos en {n.node.data_dir}</p>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            {n.providers.map((p) => (
              <div key={p.id} className="flex items-center gap-2 rounded-md border border-slate-200 px-3 py-2 text-sm dark:border-border">
                <ProviderIcon driver={p.id} />
                <span className="font-medium">{p.name}</span>
                <span className="ml-auto font-mono text-xs text-slate-500">{p.installed ? p.version || "instalado" : "no instalado"}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
      {(n?.peers ?? []).map((p) => {
        const Icon = osIcon(p.os);
        return (
          <Card key={p.id}>
            <div className="flex flex-wrap items-center gap-2">
              <Icon className="size-5" />
              <span className="font-semibold">{p.name}</span>
              <StatusPill tone={p.connected ? "green" : "slate"}>{p.connected ? "conectado" : "desconectado"}</StatusPill>
              <span className="font-mono text-xs text-slate-500">{p.address}{p.version ? ` · ${p.version}` : ""}</span>
              {!p.connected && p.last_seen && <span className="text-xs text-slate-500">visto {new Date(p.last_seen).toLocaleString()}</span>}
            </div>
            {!!p.providers?.length && <p className="mt-2 flex flex-wrap gap-2 text-xs">{p.providers.filter((x) => x.installed).map((x) => <span key={x.id} className="flex items-center gap-1"><ProviderIcon driver={x.id} />{x.name} {x.version.split(" ")[0]}</span>)}</p>}
          </Card>
        );
      })}
    </div>
  );
}
