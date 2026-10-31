import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Item } from "../api/client";
import { Button, ErrorText, Label, Modal, field } from "../components/ui";
import { useT3 } from "./attention";

// Starts one thread per item on the chosen instance. Runs on an explicit click only.
export function LaunchDialog({ items, role, onClose }: { items: Item[]; role: "triage" | "implementation"; onClose: () => void }) {
  const qc = useQueryClient();
  const t3 = useT3();
  const instances = t3.data?.filter((s) => s.state === "available") ?? [];
  const [instance, setInstance] = useState("");
  const instanceId = instance || instances[0]?.instance_id || "";
  const catalog = useQuery({ queryKey: ["t3-catalog", instanceId], queryFn: () => api.t3Catalog(instanceId), enabled: !!instanceId });
  const [project, setProject] = useState("");
  const [choice, setChoice] = useState("");
  const [worktree, setWorktree] = useState(role === "implementation");
  const firstProvider = catalog.data?.providers[0];
  const selected = choice || (firstProvider ? `${firstProvider.instance_id}|${firstProvider.models[0].slug}` : "");
  const projectId = project || catalog.data?.projects[0]?.id || "";
  const launch = useMutation({
    mutationFn: async () => {
      const [provider, model] = selected.split("|");
      for (const item of items) await api.t3Launch(item.id, { instance_id: instanceId, project_id: projectId, provider, model, role, worktree });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["items"] });
      qc.invalidateQueries({ queryKey: ["item-threads"] });
      onClose();
    },
  });
  return (
    <Modal title={role === "triage" ? `Analizar ${items.length} con un agente` : "Enviar a implementar"} onClose={onClose}>
      <Label text="Máquina">
        <select className={field} value={instanceId} onChange={(e) => { setInstance(e.target.value); setProject(""); setChoice(""); }}>
          {instances.map((s) => <option key={s.instance_id} value={s.instance_id}>{s.base_url}</option>)}
        </select>
      </Label>
      <Label text="Proyecto de T3">
        <select className={field} value={projectId} onChange={(e) => setProject(e.target.value)}>
          {catalog.data?.projects.map((p) => <option key={p.id} value={p.id}>{p.title}</option>)}
        </select>
      </Label>
      <Label text="Modelo">
        <select className={field} value={selected} onChange={(e) => setChoice(e.target.value)}>
          {catalog.data?.providers.map((p) => (
            <optgroup key={p.instance_id} label={p.name}>
              {p.models.map((m) => <option key={m.slug} value={`${p.instance_id}|${m.slug}`}>{m.name}</option>)}
            </optgroup>
          ))}
        </select>
      </Label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={worktree} onChange={(e) => setWorktree(e.target.checked)} />Crear un worktree nuevo</label>
      <p className="text-xs text-slate-500 dark:text-slate-400">El agente se lanza con acceso completo.</p>
      <ErrorText error={catalog.error ?? launch.error} />
      <Button disabled={!selected || !projectId || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
