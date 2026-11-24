import { useQueryClient } from "@tanstack/react-query";
import { qk } from "../../api/queries/keys";
import { useKeptWorktrees,useRemoveThreadWorktree } from "../../api/queries/threads";
import { askConfirm } from "../../components/confirm";
import { Button,ErrorText } from "../../components/ui";

// KeptWorktrees warns about the worktrees of archived threads that were not
// deleted because they have uncommitted changes, and deletes them on request.
export function KeptWorktrees() {
  const qc = useQueryClient();
  const kept = useKeptWorktrees({ refetchInterval: 30_000 });
  const remove = useRemoveThreadWorktree({
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.keptWorktrees() }),
  });
  const dirty = (kept.data ?? []).filter((k) => k.dirty);
  if (!dirty.length) return null;
  return (
    <div role="status" className="mx-2 mb-2 space-y-2 rounded-lg border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
      <p className="font-medium">Worktrees conservados con cambios sin commitear</p>
      {dirty.map((k) => (
        <div key={k.thread_id} className="flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-mono" title={k.path}>{k.path}</span>
          <Button variant="danger" disabled={remove.isPending}
            onClick={() => askConfirm("¿Eliminar el worktree? Se perderán los cambios sin commitear.", "Eliminar").then((ok) => ok && remove.mutate(k.thread_id))}>Eliminar worktree</Button>
        </div>
      ))}
      <ErrorText error={remove.error} />
    </div>
  );
}
