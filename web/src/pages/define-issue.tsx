import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api/client";
import { Button, ErrorText, Modal } from "../components/ui";
import { useLaunchForm } from "./launch-form";

// Shared dialog for issue definition and pull request conflict resolution.
export function DefineIssueDialog({ repo, number, onClose, resolve = false }: { repo: string; number: number; onClose: () => void; resolve?: boolean }) {
  const qc = useQueryClient();
  const form = useLaunchForm({ section: resolve ? "resolve-conflicts" : "issue-define", repo });
  const launch = useMutation({
    mutationFn: () => resolve ? api.resolveConflicts(repo, number, form.value) : api.issueDefine(repo, number, form.value),
    onSuccess: () => {
      form.remember();
      qc.invalidateQueries({ queryKey: ["issue-threads"] });
    },
  });
  return (
    <Modal title={`${resolve ? "Resolver conflictos de" : "Definir tareas de"} #${number}`} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">{resolve ? "El agente resuelve los conflictos en un worktree, valida y empuja la rama de la PR. Si hay dudas, las plantea en el hilo." : "El agente analiza la issue y el repositorio y crea los items en Dory; no implementa nada."}</p>
      <ErrorText error={form.error ?? launch.error} />
      {launch.isSuccess ? <Link to="/jobs" onClick={onClose} className="text-blue-600 underline">Ver hilo en Trabajos</Link> : <Button disabled={!form.ready || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>}
    </Modal>
  );
}
