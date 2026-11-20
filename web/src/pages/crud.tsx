import { DictationField } from "../components/dictation-field";
import { useDictationInProgress } from "../components/local-dictation";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowsClockwise, Desktop, Plus } from "@phosphor-icons/react";
import { EnvIconPicker } from "../components/env-icon";
import { useMeshUpdates } from "../components/mesh-update";
import { api, type Memory, type MemoryInput, type NodeUpdate } from "../api/client";
import { Badge, Button, Card, ErrorText, Input, Label, Md, Modal, PageHeader, Select, StatusPill, csv } from "../components/ui";
import { useProject } from "../project";
import { askConfirm } from "../components/confirm";

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
      <PageHeader title="Memorias" subtitle="Lo que los agentes recuerdan entre sesiones. Se replica entre nodos."><Button onClick={() => setEditing("new")}><Plus weight="regular" className="size-4" />Nueva</Button></PageHeader>
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
  const dictating = useDictationInProgress();
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
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (!dictating && m.content.trim()) save.mutate(); }}>
        <Label text="Título"><Input value={m.title ?? ""} onChange={(e) => setM({ ...m, title: e.target.value })} /></Label>
        <DictationField label="Contenido" rows={10} value={m.content} onValue={(content) => setM((current) => ({ ...current, content }))} />
        <div className="grid gap-2 sm:grid-cols-3">
          <Label text="Tipo"><Select value={m.kind ?? "context"} onChange={(v) => setM({ ...m, kind: v as Memory["kind"] })} options={Object.entries(KINDS).map(([value, label]) => ({ value, label }))} /></Label>
          <Label text="Ámbito"><Select value={m.scope ?? "global"} onChange={(v) => setM({ ...m, scope: v as Memory["scope"] })} options={[{ value: "global", label: "Global" }, { value: "project", label: "Proyecto" }]} /></Label>
          {m.scope === "project" && <Label text="Proyecto"><Select value={m.scope_id ?? ""} onChange={(v) => setM({ ...m, scope_id: v })} options={(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))} /></Label>}
        </div>
        <Label text="Labels (separadas por comas)"><Input value={labels} onChange={(e) => setLabels(e.target.value)} /></Label>
        <ErrorText error={save.error ?? archive.error} />
        <div className="flex gap-2">
          <Button disabled={dictating || !m.content.trim() || save.isPending}>Guardar</Button>
          {memory && <Button type="button" variant="ghost" className="ml-auto" onClick={() => askConfirm("¿Archivar la memoria?", "Archivar").then((ok) => ok && archive.mutate())}>Archivar</Button>}
        </div>
      </form>
    </Modal>
  );
}


// Environments lists the nodes of the mesh: this one and its peers, with
// their harnesses (requirements RF-06, RF-07), and pairs new ones (RF-05).
export function Environments() {
  const qc = useQueryClient();
  const node = useQuery({ queryKey: ["node"], queryFn: api.node, refetchInterval: 15_000 });
  const mesh = useQuery({ queryKey: ["mesh"], queryFn: api.mesh, refetchInterval: 5_000 });
  const [code, setCode] = useState<{ code: string; expires_at: string; fingerprint: string; port: number }>();
  const [address, setAddress] = useState("");
  const [typed, setTyped] = useState("");
  const open = useMutation({ mutationFn: api.pairCode, onSuccess: setCode });
  const pair = useMutation({ mutationFn: () => api.pair(address.trim(), typed.trim()), onSuccess: () => { setAddress(""); setTyped(""); qc.invalidateQueries({ queryKey: ["mesh"] }); } });
  const revoke = useMutation({ mutationFn: api.revokePeer, onSuccess: () => qc.invalidateQueries({ queryKey: ["mesh"] }) });
  const devices = useQuery({ queryKey: ["devices"], queryFn: api.devices, refetchInterval: 15_000 });
  const revokeDevice = useMutation({ mutationFn: api.revokeDevice, onSuccess: () => qc.invalidateQueries({ queryKey: ["devices"] }) });
  const n = node.data;
  const search = useSearch({ strict: false }) as { node?: string };
  const targetReady = !!search.node && (n?.node.id === search.node || !!mesh.data?.peers.some((p) => !p.revoked && p.id === search.node));
  useEffect(() => {
    if (search.node && targetReady) document.getElementById(`environment-${search.node}`)?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [search.node, targetReady]);
  const highlight = (id: string) => search.node === id ? "rounded-lg ring-2 ring-blue-500" : "";
  return (
    <div className="space-y-3">
      <PageHeader title="Entornos" subtitle="Los nodos de Dory en tus máquinas. Se sincronizan cuando se ven." />
      <ErrorText error={node.error ?? mesh.error} />
      <MeshUpdate />
      {n && (
        <div id={`environment-${n.node.id}`} className={highlight(n.node.id)}><Card>
          <div className="flex flex-wrap items-center gap-2">
            <EnvIconPicker node="local" icon={n.node.icon} os={n.node.os} name={n.node.name} />
            <span className="font-semibold">{n.node.name}</span>
            <span className="font-mono text-xs text-slate-500">{n.node.platform} · {n.node.version} · huella {mesh.data?.fingerprint}</span>
          </div>
          <p className="mt-1 font-mono text-xs text-slate-500">id {n.node.id} · datos en {n.node.data_dir}</p>
        </Card></div>
      )}
      {(mesh.data?.peers ?? []).filter((p) => !p.revoked).map((p) => {
        return (
          <div key={p.id} id={`environment-${p.id}`} className={highlight(p.id)}><Card>
            <div className="flex flex-wrap items-center gap-2">
              <EnvIconPicker node={p.id} icon={p.info.node?.icon} os={p.info.node?.os} name={p.name} />
              <span className="font-semibold">{p.name}</span>
              <StatusPill tone={p.connected ? "green" : "slate"}>{p.connected ? "conectado" : "desconectado"}</StatusPill>
              <span className="font-mono text-xs text-slate-500">{p.address}{p.info.node?.version ? ` · ${p.info.node.version}` : ""} · huella {p.fingerprint}</span>
              {!p.connected && p.last_seen && <span className="text-xs text-slate-500">visto {new Date(p.last_seen).toLocaleString()}</span>}
              <Button variant="ghost" className="ml-auto text-xs" onClick={() => askConfirm(`¿Revocar ${p.name}? Dejará de sincronizarse con este nodo.`, "Revocar").then((ok) => ok && revoke.mutate(p.id))}>Revocar</Button>
            </div>
          </Card></div>
        );
      })}
      <Card>
        <h2 className="mb-2 font-semibold">Emparejar otro nodo o un móvil</h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2 text-sm">
            <p className="text-slate-600 dark:text-muted">En este nodo: abre un código y escríbelo en el otro nodo o en la app del móvil.</p>
            {code ? (
              <div className="rounded-md bg-slate-100 p-3 dark:bg-raised">
                <p className="font-mono text-2xl tracking-widest">{code.code}</p>
                <p className="mt-1 text-xs text-slate-500">Puerto {code.port} · huella {code.fingerprint} · caduca a las {new Date(code.expires_at).toLocaleTimeString()}</p>
              </div>
            ) : <Button onClick={() => open.mutate()} disabled={open.isPending}>Mostrar código</Button>}
            <ErrorText error={open.error} />
          </div>
          <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); if (address.trim() && typed.trim()) pair.mutate(); }}>
            <p className="text-sm text-slate-600 dark:text-muted">O en el otro sentido: la dirección del nodo y su código.</p>
            <Input value={address} onChange={(e) => setAddress(e.target.value)} placeholder="100.64.0.2 o servidor:7710" />
            <Input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="Código, p. ej. 1234-5678" className="font-mono" />
            <Button disabled={!address.trim() || !typed.trim() || pair.isPending}>{pair.isPending ? "Emparejando…" : "Emparejar"}</Button>
            <ErrorText error={pair.error} />
          </form>
        </div>
        {!!mesh.data?.discovered.length && (
          <div className="mt-3 border-t border-slate-200 pt-3 dark:border-border">
            <p className="mb-1 text-sm font-medium">Nodos encontrados en la red</p>
            {mesh.data.discovered.map((d) => (
              <button key={d.id} type="button" onClick={() => setAddress(d.address)} className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                <Desktop className="size-4" /><span>{d.name}</span><span className="ml-auto font-mono text-xs text-slate-500">{d.address}</span>
              </button>
            ))}
          </div>
        )}
      </Card>
      {!!devices.data?.length && (
        <Card>
          <h2 className="mb-2 font-semibold">Móviles emparejados</h2>
          {devices.data.map((d) => (
            <div key={d.id} className="flex items-center gap-2 py-1 text-sm">
              <span>{d.name}</span>
              <span className="text-xs text-slate-500">desde {new Date(d.created_at).toLocaleDateString()}</span>
              <Button variant="ghost" className="ml-auto text-xs" onClick={() => confirm(`¿Revocar ${d.name}? Tendrá que emparejarse de nuevo.`) && revokeDevice.mutate(d.id)}>Revocar</Button>
            </div>
          ))}
          <ErrorText error={revokeDevice.error} />
        </Card>
      )}
    </div>
  );
}

// MeshUpdate moves every connected node to the latest release of its channel,
// with its desktop app and, if asked, its harnesses (ADR-016).
function MeshUpdate() {
  const qc = useQueryClient();
  const [providers, setProviders] = useState(false);
  const [force, setForce] = useState(false);
  const list = useMeshUpdates();
  const run = useMutation({ mutationFn: () => api.updateMesh({ providers, force }), onSuccess: (data) => qc.setQueryData(["mesh-update"], data) });
  // Checks the releases now instead of waiting for the next poll.
  const recheck = useMutation({ mutationFn: () => api.meshUpdates(true), onSuccess: (data) => qc.setQueryData(["mesh-update"], data) });
  const nodes = list.data ?? [];
  const pending = nodes.some((n) => n.status?.available || n.status?.providers.some((p) => p.available)) || providers;
  const label = (n: NodeUpdate) => {
    const st = n.status;
    if (n.error) return <StatusPill tone="slate">{n.error}</StatusPill>;
    if (!st) return null;
    if (st.state === "updating") return <StatusPill tone="amber">actualizando…</StatusPill>;
    if (st.state === "failed") return <StatusPill tone="red">falló: {st.error}</StatusPill>;
    if (st.channel === "dev") return <StatusPill tone="slate">compilación local</StatusPill>;
    if (st.available) return <StatusPill tone="amber">{st.current} → {st.latest}</StatusPill>;
    return <StatusPill tone="green">al día</StatusPill>;
  };
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-semibold">Actualizar la malla</h2>
        <span className="text-xs text-slate-500">Cada nodo conectado instala la última versión de su canal y, si tiene la app de escritorio, la reinstala.</span>
        <button type="button" aria-label="Buscar actualizaciones" title="Buscar actualizaciones" disabled={recheck.isPending} onClick={() => recheck.mutate()}
          className="ml-auto grid size-8 shrink-0 place-items-center rounded-md hover:bg-slate-200 disabled:opacity-70 dark:hover:bg-raised">
          <ArrowsClockwise className={`size-5 ${recheck.isPending ? "animate-spin" : ""}`} aria-hidden="true" />
        </button>
      </div>
      <ErrorText error={list.error ?? recheck.error ?? run.error} />
      <div className="mt-2 space-y-1">
        {nodes.map((n) => (
          <div key={n.node} className="flex flex-wrap items-center gap-2 text-sm">
            <span>{n.name}</span>
            {n.status && <span className="font-mono text-xs text-slate-500">{n.status.channel}{n.status.desktop ? " · escritorio" : ""}{n.status.active_turns ? ` · ${n.status.active_turns} turnos en marcha` : ""}</span>}
            <span className="ml-auto">{label(n)}</span>
            {n.status?.providers.filter((p) => p.available).map((p) => (
              <span key={p.id} className="basis-full pl-4 text-xs text-slate-500">{p.name} {p.current} → {p.latest}</span>
            ))}
          </div>
        ))}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2"><input type="checkbox" checked={providers} onChange={(e) => setProviders(e.target.checked)} />Actualizar también los proveedores</label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />Aunque haya turnos en marcha</label>
        <Button className="ml-auto" disabled={!pending || run.isPending} onClick={() => run.mutate()}>{run.isPending ? "Lanzando…" : "Actualizar"}</Button>
      </div>
    </Card>
  );
}
