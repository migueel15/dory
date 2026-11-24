import { DictationField } from "../features/composer/dictation-field";
import { useDictationInProgress } from "../features/composer/local-dictation";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "@phosphor-icons/react";
import { api, type Skill, type SkillConflict, type SkillState } from "../api/client";
import { Button, Card, ErrorText, Input, Label, Modal, PageHeader, StatusPill, type Tone } from "../components/ui";
import { askConfirm } from "../components/confirm";
import { qk } from "../api/queries/keys";
import { useMesh, useNode } from "../api/queries/hooks";

const STATE: Record<SkillState["status"], [string, Tone]> = {
  synced: ["sincronizada", "green"], pending: ["pendiente", "amber"], error: ["error", "red"], conflict: ["conflicto", "red"], removed: ["no instalada", "slate"],
};

// Skills is the single catalog of skills with, per node, whether each one is
// installed, pending or in conflict (requirements RF-37 to RF-41).
export function Skills() {
  const skills = useQuery({ queryKey: qk.skills(), queryFn: api.skills });
  const states = useQuery({ queryKey: qk.skillStates(), queryFn: api.skillStates, refetchInterval: 10_000 });
  const mesh = useMesh();
  const node = useNode();
  const harnesses = (node.data?.providers ?? [])
    .filter((p) => p.id === (p.driver ?? p.id) && p.capabilities.includes("skills"))
    .map((p) => [p.id, p.driver_name || p.name] as const);
  const [editing, setEditing] = useState<Skill | "new">();
  const [conflict, setConflict] = useState<{ skill: Skill; node: string }>();
  const nodes = [...(node.data ? [{ id: node.data.node.id, name: node.data.node.name, connected: true }] : []), ...(mesh.data?.peers ?? []).filter((p) => !p.revoked).map((p) => ({ id: p.id, name: p.name, connected: p.connected }))];
  const online = (id: string) => nodes.find((n) => n.id === id)?.connected ?? false;
  const name = (id: string) => nodes.find((n) => n.id === id)?.name ?? id.slice(0, 6);
  const qc = useQueryClient();
  const enable = useMutation({ mutationFn: ({ id, on }: { id: string; on: boolean }) => api.setSkillEnabled(id, on), onSuccess: () => qc.invalidateQueries({ queryKey: qk.skills() }) });
  const skillName = (id: string) => skills.data?.find((s) => s.id === id)?.name ?? id;
  // System skills first; each group keeps the catalog order.
  const list = [...(skills.data ?? [])].sort((a, b) => Number(b.source === "system") - Number(a.source === "system"));
  return (
    <div className="space-y-3">
      <PageHeader title="Skills" subtitle="Un catálogo para todos los nodos. Cada nodo instala las suyas en las carpetas de sus agentes."><Button onClick={() => setEditing("new")}><Plus weight="regular" className="size-4" />Nueva skill</Button></PageHeader>
      <ErrorText error={skills.error ?? enable.error} />
      {list.map((s) => (
        <Card key={s.id} className={s.disabled_at ? "opacity-60" : undefined}>
          <div className="flex items-start gap-2">
          <button type="button" className="block min-w-0 flex-1 text-left" onClick={() => setEditing(s)}>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono font-medium">{s.name}</span>
              {s.source === "system" && <StatusPill tone="slate">Sistema</StatusPill>}
              {s.forked_from && <span className="text-xs text-slate-500">fork de {skillName(s.forked_from)}</span>}
              {s.disabled_at && <StatusPill tone="amber">Desactivada</StatusPill>}
              <span className="text-xs text-slate-500">v{s.version}</span>
              <span className="text-xs text-slate-500">{s.targets.harnesses?.length ? s.targets.harnesses.join(", ") : "todos los agentes"} · {s.targets.nodes?.length ? s.targets.nodes.map(name).join(", ") : "todos los nodos"}</span>
            </div>
            {s.description && <p className="mt-1 text-sm text-slate-600 dark:text-muted">{s.description}</p>}
          </button>
          <Button type="button" variant="ghost" disabled={enable.isPending} onClick={() => enable.mutate({ id: s.id, on: !!s.disabled_at })}>{s.disabled_at ? "Activar" : "Desactivar"}</Button>
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            {(states.data ?? []).filter((st) => st.skill_id === s.id && (!s.disabled_at || st.status === "conflict")).map((st) => (
              <span key={st.node_id} title={online(st.node_id) ? st.message || undefined : "Último estado conocido: el nodo no está conectado"} className={`flex items-center gap-1 text-xs${online(st.node_id) ? "" : " opacity-60"}`}>
                {name(st.node_id)}{online(st.node_id) ? "" : " (desconectado)"}: {st.status === "conflict" && online(st.node_id) ? <button type="button" className="underline decoration-dotted" onClick={() => setConflict({ skill: s, node: st.node_id })}>resolver</button> : null} <StatusPill tone={st.status === "synced" && st.version < s.version ? "amber" : STATE[st.status][1]}>{st.status === "synced" && st.version < s.version ? "pendiente" : STATE[st.status][0]}</StatusPill>
              </span>
            ))}
          </div>
        </Card>
      ))}
      {skills.isSuccess && skills.data.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">El catálogo está vacío.</p>}
      {conflict && <ConflictDialog skill={conflict.skill} node={conflict.node} nodeName={name(conflict.node)} harnesses={harnesses} onClose={() => setConflict(undefined)} />}
      {editing && (editing !== "new" && editing.source === "system"
        ? <SystemSkill skill={editing} onClose={() => setEditing(undefined)} />
        : <SkillForm skill={editing === "new" ? undefined : editing} nodes={nodes} harnesses={harnesses} onClose={() => setEditing(undefined)} />)}
    </div>
  );
}

function SkillForm({ skill, nodes, harnesses, onClose }: { skill?: Skill; nodes: { id: string; name: string }[]; harnesses: (readonly [string, string])[]; onClose: () => void }) {
  const qc = useQueryClient();
  const dictating = useDictationInProgress();
  const [s, setS] = useState<Partial<Skill>>(skill ?? { name: "", description: "", body_md: "", files: {}, targets: { nodes: [], harnesses: [] } });
  const toggle = (key: "nodes" | "harnesses", v: string) => {
    const list = s.targets?.[key] ?? [];
    setS({ ...s, targets: { nodes: s.targets?.nodes ?? [], harnesses: s.targets?.harnesses ?? [], [key]: list.includes(v) ? list.filter((x) => x !== v) : [...list, v] } });
  };
  const save = useMutation({ mutationFn: () => api.saveSkill(s), onSuccess: () => { qc.invalidateQueries({ queryKey: qk.skills() }); onClose(); } });
  const archive = useMutation({ mutationFn: () => api.archiveSkill(skill!.id), onSuccess: () => { qc.invalidateQueries({ queryKey: qk.skills() }); onClose(); } });
  return (
    <Modal title={skill ? `Skill ${skill.name}` : "Nueva skill"} onClose={onClose} wide>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (!dictating) save.mutate(); }}>
        <div className="grid gap-2 sm:grid-cols-2">
          <Label text="Nombre (kebab-case)"><Input value={s.name ?? ""} onChange={(e) => setS({ ...s, name: e.target.value })} className="font-mono" /></Label>
          <DictationField label="Descripción" rows={1} value={s.description ?? ""} onValue={(description) => setS((current) => ({ ...current, description }))} />
        </div>
        <DictationField label="Instrucciones (SKILL.md, sin frontmatter)" rows={14} value={s.body_md ?? ""} onValue={(body_md) => setS((current) => ({ ...current, body_md }))} className="font-mono !text-xs" />
        <fieldset className="text-sm">
          <legend className="mb-1 font-medium">Agentes <span className="text-xs text-slate-500">(ninguno marcado = todos)</span></legend>
          <div className="flex flex-wrap gap-3">{harnesses.map(([id, label]) => <label key={id} className="flex items-center gap-1.5"><input type="checkbox" checked={!!s.targets?.harnesses?.includes(id)} onChange={() => toggle("harnesses", id)} />{label}</label>)}</div>
        </fieldset>
        <fieldset className="text-sm">
          <legend className="mb-1 font-medium">Nodos <span className="text-xs text-slate-500">(ninguno marcado = todos; se aplica aunque estén desconectados)</span></legend>
          <div className="flex flex-wrap gap-3">{nodes.map((n) => <label key={n.id} className="flex items-center gap-1.5"><input type="checkbox" checked={!!s.targets?.nodes?.includes(n.id)} onChange={() => toggle("nodes", n.id)} />{n.name}</label>)}</div>
        </fieldset>
        <ErrorText error={save.error ?? archive.error} />
        <div className="flex gap-2">
          <Button disabled={dictating || !s.name || save.isPending}>Guardar</Button>
          {skill && <Button type="button" variant="ghost" className="ml-auto" onClick={() => askConfirm("¿Archivar la skill? Se desinstala de todos los nodos.", "Archivar").then((ok) => ok && archive.mutate())}>Archivar</Button>}
        </div>
      </form>
    </Modal>
  );
}

// SystemSkill shows a skill Dory ships. It cannot be edited: a fork makes an
// editable copy and turns the original off on every node.
function SystemSkill({ skill, onClose }: { skill: Skill; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(`${skill.name}-custom`);
  const fork = useMutation({ mutationFn: () => api.forkSkill(skill.id, name), onSuccess: () => { qc.invalidateQueries({ queryKey: qk.skills() }); onClose(); } });
  return (
    <Modal title={`Skill ${skill.name}`} onClose={onClose} wide>
      <div className="space-y-3">
        <p className="text-sm text-slate-600 dark:text-muted">Skill del sistema, v{skill.version}. Dory la instala en todos los nodos y la actualiza con cada versión. No se puede editar: haz un fork para cambiarla; la original se desactiva y solo se puede reactivar con el fork desactivado.</p>
        {skill.description && <p className="text-sm">{skill.description}</p>}
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded border border-slate-200 p-3 font-mono text-xs dark:border-line">{skill.body_md}</pre>
        <ErrorText error={fork.error} />
        <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); fork.mutate(); }}>
          <Label text="Nombre del fork"><Input value={name} onChange={(e) => setName(e.target.value)} className="font-mono" /></Label>
          <Button disabled={!name || fork.isPending}>Hacer fork</Button>
        </form>
      </div>
    </Modal>
  );
}

const KIND: Record<SkillConflict["kind"], string> = {
  link: "la carpeta es un enlace a otro sitio",
  foreign: "Dory no escribió estos ficheros",
  edited: "se editó a mano después de instalarla",
};

// ConflictDialog settles the copies of a skill that block its install on one
// node: keep the catalog version, or take the node's copy into the catalog.
// Either way the node's copies go to a backup folder first.
function ConflictDialog({ skill, node, nodeName, harnesses, onClose }: { skill: Skill; node: string; nodeName: string; harnesses: (readonly [string, string])[]; onClose: () => void }) {
  const qc = useQueryClient();
  const conflicts = useQuery({ queryKey: qk.skillConflicts(node, skill.id), queryFn: () => api.skillConflicts(node, skill.id) });
  const [done, setDone] = useState<string>();
  const resolve = useMutation({
    mutationFn: ({ action, harness }: { action: "restore" | "adopt"; harness?: string }) => api.resolveSkill(node, skill.id, action, harness),
    onSuccess: (r) => { setDone(r.backup); qc.invalidateQueries({ queryKey: qk.skills() }); },
  });
  const system = skill.source === "system";
  const label = (h: string) => harnesses.find(([id]) => id === h)?.[1] ?? h;
  return (
    <Modal title={`Conflicto de ${skill.name} en ${nodeName}`} onClose={onClose} wide>
      <div className="space-y-3 text-sm">
        {done ? (
          <>
            <p>Resuelto. Las copias anteriores del nodo están en <span className="font-mono text-xs">{done}</span>.</p>
            <Button type="button" onClick={onClose}>Cerrar</Button>
          </>
        ) : (
          <>
            <p className="text-slate-600 dark:text-muted">Dory no instala esta skill en el nodo porque ya hay una copia que no es la suya. Elige cuál se queda; la del nodo se guarda antes en una copia de seguridad.</p>
            <ErrorText error={conflicts.error ?? resolve.error} />
            {(conflicts.data ?? []).map((c) => (
              <div key={c.harness} className="space-y-1 rounded border border-slate-200 p-2 dark:border-line">
                <div><span className="font-medium">{label(c.harness)}</span> · {KIND[c.kind]}</div>
                <div className="break-all font-mono text-xs text-slate-500">{c.path}</div>
                {c.skill_md && <pre className="max-h-48 overflow-auto whitespace-pre-wrap font-mono text-xs">{c.skill_md}</pre>}
                {c.orphan && <p className="text-xs text-slate-500">Esta skill ya no debe estar en el nodo (está desactivada o el nodo no es su destino), pero su copia se editó a mano.</p>}
                {c.skill_md && !c.orphan && <Button type="button" variant="ghost" disabled={resolve.isPending} onClick={() => askConfirm(system ? "Se crea un fork con esta versión y la skill del sistema se desactiva en toda la malla." : "Esta versión sustituye a la del catálogo en toda la malla.", "Quedarme con esta").then((ok) => ok && resolve.mutate({ action: "adopt", harness: c.harness }))}>{system ? "Hacer fork con esta versión" : "Quedarme con esta versión"}</Button>}
              </div>
            ))}
            {conflicts.isSuccess && conflicts.data.length === 0 && <p>Ya no hay conflictos en este nodo.</p>}
            {!!conflicts.data?.length && <Button type="button" disabled={resolve.isPending} onClick={() => askConfirm(conflicts.data.every((c) => c.orphan) ? "Las copias del nodo se mueven a una copia de seguridad y la skill deja de estar en el nodo." : "Las copias del nodo se mueven a una copia de seguridad y se instala la versión del catálogo.", conflicts.data.every((c) => c.orphan) ? "Retirar" : "Restaurar").then((ok) => ok && resolve.mutate({ action: "restore" }))}>{conflicts.data.every((c) => c.orphan) ? "Retirar la copia del nodo" : "Restaurar la versión de Dory"}</Button>}
          </>
        )}
      </div>
    </Modal>
  );
}
