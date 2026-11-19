import { DictationField } from "../components/dictation-field";
import { useDictationInProgress } from "../components/local-dictation";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "@phosphor-icons/react";
import { api, type Skill, type SkillState } from "../api/client";
import { Button, Card, ErrorText, Input, Label, Modal, PageHeader, StatusPill, type Tone } from "../components/ui";
import { askConfirm } from "../components/confirm";

const HARNESSES = [["claude", "Claude Code"], ["codex", "Codex"], ["opencode", "OpenCode"], ["antigravity", "Antigravity"]] as const;
const STATE: Record<SkillState["status"], [string, Tone]> = {
  synced: ["sincronizada", "green"], pending: ["pendiente", "amber"], error: ["error", "red"], conflict: ["conflicto", "red"], removed: ["no instalada", "slate"],
};

// Skills is the single catalog of skills with, per node, whether each one is
// installed, pending or in conflict (requirements RF-37 to RF-41).
export function Skills() {
  const skills = useQuery({ queryKey: ["skills"], queryFn: api.skills });
  const states = useQuery({ queryKey: ["skills", "states"], queryFn: api.skillStates, refetchInterval: 10_000 });
  const mesh = useQuery({ queryKey: ["mesh"], queryFn: api.mesh });
  const node = useQuery({ queryKey: ["node"], queryFn: api.node });
  const [editing, setEditing] = useState<Skill | "new">();
  const nodes = [...(node.data ? [{ id: node.data.node.id, name: node.data.node.name }] : []), ...(mesh.data?.peers ?? []).filter((p) => !p.revoked).map((p) => ({ id: p.id, name: p.name }))];
  const name = (id: string) => nodes.find((n) => n.id === id)?.name ?? id.slice(0, 6);
  return (
    <div className="space-y-3">
      <PageHeader title="Skills" subtitle="Un catálogo para todos los nodos. Cada nodo instala las suyas en las carpetas de sus agentes."><Button onClick={() => setEditing("new")}><Plus weight="regular" className="size-4" />Nueva skill</Button></PageHeader>
      <ErrorText error={skills.error} />
      {(skills.data ?? []).map((s) => (
        <Card key={s.id}>
          <button type="button" className="block w-full text-left" onClick={() => setEditing(s)}>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono font-medium">{s.name}</span>
              <span className="text-xs text-slate-500">v{s.version}</span>
              <span className="text-xs text-slate-500">{s.targets.harnesses?.length ? s.targets.harnesses.join(", ") : "todos los agentes"} · {s.targets.nodes?.length ? s.targets.nodes.map(name).join(", ") : "todos los nodos"}</span>
            </div>
            {s.description && <p className="mt-1 text-sm text-slate-600 dark:text-muted">{s.description}</p>}
          </button>
          <div className="mt-2 flex flex-wrap gap-2">
            {(states.data ?? []).filter((st) => st.skill_id === s.id).map((st) => (
              <span key={st.node_id} title={st.message || undefined} className="flex items-center gap-1 text-xs">
                {name(st.node_id)}: <StatusPill tone={st.status === "synced" && st.version < s.version ? "amber" : STATE[st.status][1]}>{st.status === "synced" && st.version < s.version ? "pendiente" : STATE[st.status][0]}</StatusPill>
              </span>
            ))}
          </div>
        </Card>
      ))}
      {skills.isSuccess && skills.data.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">El catálogo está vacío.</p>}
      {editing && <SkillForm skill={editing === "new" ? undefined : editing} nodes={nodes} onClose={() => setEditing(undefined)} />}
    </div>
  );
}

function SkillForm({ skill, nodes, onClose }: { skill?: Skill; nodes: { id: string; name: string }[]; onClose: () => void }) {
  const qc = useQueryClient();
  const dictating = useDictationInProgress();
  const [s, setS] = useState<Partial<Skill>>(skill ?? { name: "", description: "", body_md: "", files: {}, targets: { nodes: [], harnesses: [] } });
  const toggle = (key: "nodes" | "harnesses", v: string) => {
    const list = s.targets?.[key] ?? [];
    setS({ ...s, targets: { nodes: s.targets?.nodes ?? [], harnesses: s.targets?.harnesses ?? [], [key]: list.includes(v) ? list.filter((x) => x !== v) : [...list, v] } });
  };
  const save = useMutation({ mutationFn: () => api.saveSkill(s), onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }); onClose(); } });
  const archive = useMutation({ mutationFn: () => api.archiveSkill(skill!.id), onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }); onClose(); } });
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
          <div className="flex flex-wrap gap-3">{HARNESSES.map(([id, label]) => <label key={id} className="flex items-center gap-1.5"><input type="checkbox" checked={!!s.targets?.harnesses?.includes(id)} onChange={() => toggle("harnesses", id)} />{label}</label>)}</div>
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
