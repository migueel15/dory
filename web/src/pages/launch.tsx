import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Item } from "../api/client";
import { Button, ErrorText, Modal } from "../components/ui";
import { useLaunchForm } from "./launch-form";

// Starts one thread per item on the chosen instance. Runs on an explicit click
// only: refine opens the definition thread of each item, implementation builds
// a ready one.
export function LaunchDialog({ items, role, onClose, onSuccess }: { items: Item[]; role: "refine" | "implementation"; onClose: () => void; onSuccess?: () => void }) {
  const qc = useQueryClient();
  // Definition never touches code, so only implementation offers a worktree.
  const form = useLaunchForm({ doryProject: items[0]?.project_id, worktree: role === "implementation" ? true : undefined });
  const launch = useMutation({
    mutationFn: async () => {
      for (const item of items) {
        if (role === "refine") await api.refine(item.id, form.value);
        else await api.t3Launch(item.id, { ...form.value, role, worktree: !!form.value.worktree });
      }
    },
    onSuccess: () => {
      form.remember();
      ["items", "item-threads", "refinement"].forEach((k) => qc.invalidateQueries({ queryKey: [k] }));
      onClose();
      onSuccess?.();
    },
  });
  return (
    <Modal title={role === "refine" ? `Definir ${items.length} con un agente` : "Enviar a implementar"} onClose={onClose}>
      {role === "refine" && <p className="text-sm">Abre un hilo de definición por item. Después sigue cada uno en su vista «Definir».</p>}
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">El agente se lanza con acceso completo.</p>
      <ErrorText error={form.error ?? launch.error} />
      <Button disabled={!form.ready || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
