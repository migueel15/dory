import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Item } from "../api/client";
import { Button, ErrorText, Modal } from "../components/ui";
import { useLaunchForm } from "./launch-form";

// Starts one thread per item on the chosen instance. Runs on an explicit click only.
export function LaunchDialog({ items, role, onClose }: { items: Item[]; role: "triage" | "implementation"; onClose: () => void }) {
  const qc = useQueryClient();
  // Analysis never touches code, so only implementation offers a worktree.
  const form = useLaunchForm({ doryProject: items[0]?.project_id, worktree: role === "implementation" ? true : undefined });
  const launch = useMutation({
    mutationFn: async () => {
      for (const item of items) await api.t3Launch(item.id, { ...form.value, role, worktree: !!form.value.worktree });
    },
    onSuccess: () => {
      form.remember();
      qc.invalidateQueries({ queryKey: ["items"] });
      qc.invalidateQueries({ queryKey: ["item-threads"] });
      onClose();
    },
  });
  return (
    <Modal title={role === "triage" ? `Analizar ${items.length} con un agente` : "Enviar a implementar"} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">El agente se lanza con acceso completo.</p>
      <ErrorText error={form.error ?? launch.error} />
      <Button disabled={!form.ready || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
