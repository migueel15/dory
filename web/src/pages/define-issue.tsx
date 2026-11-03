import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, ErrorText, Modal } from "../components/ui";
import { useLaunchForm } from "./launch-form";

// DefineIssueDialog starts one thread that turns the issue into Dory items.
// Runs on an explicit click only; the agent never implements, so no worktree.
export function DefineIssueDialog({ repo, number, onClose }: { repo: string; number: number; onClose: () => void }) {
  const qc = useQueryClient();
  const form = useLaunchForm({ repo });
  const launch = useMutation({
    mutationFn: () => api.issueDefine(repo, number, form.value),
    onSuccess: () => {
      form.remember();
      qc.invalidateQueries({ queryKey: ["issue-threads"] });
      onClose();
    },
  });
  return (
    <Modal title={`Definir tareas de #${number}`} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-slate-400">El agente analiza la issue y el repositorio y crea los items en Dory; no implementa nada.</p>
      <ErrorText error={form.error ?? launch.error} />
      <Button disabled={!form.ready || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
