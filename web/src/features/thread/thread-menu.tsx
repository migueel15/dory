import { useQueryClient } from "@tanstack/react-query";
import { useNavigate,useParams } from "@tanstack/react-router";
import { useState,type MouseEvent } from "react";
import { type Thread } from "../../api/client";
import { qk } from "../../api/queries/keys";
import { useThreadAction } from "../../api/queries/threads";
import { askConfirm } from "../../components/confirm";
import { ContextMenu } from "../../components/context-menu";

// useThreadMenu opens, on right click, the actions of a thread: pin or unpin
// and archive. The thread header and the side list share it.
export function useThreadMenu(t: Thread | undefined) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { threadId } = useParams({ strict: false }) as { threadId?: string };
  const [at, setAt] = useState<{ x: number; y: number } | null>(null);
  const act = useThreadAction(t?.id ?? "", {
    onSuccess: (_, action) => {
      qc.invalidateQueries({ queryKey: qk.thread(t!.id) });
      qc.invalidateQueries({ queryKey: qk.threads() });
      if (action.action === "archive" && threadId === t!.id) navigate({ to: "/threads" });
    },
  });
  const open = (e: MouseEvent) => { if (!t) return; e.preventDefault(); setAt({ x: e.clientX, y: e.clientY }); };
  const menu = at && t ? (
    <ContextMenu x={at.x} y={at.y} label="Acciones del hilo" onClose={() => setAt(null)} actions={[
      { label: t.pinned_at ? "Desfijar" : "Fijar", run: () => act.mutate({ action: t.pinned_at ? "unpin" : "pin" }) },
      { label: "Archivar (borra su worktree)", run: () => askConfirm("¿Archivar el hilo? Se borra su worktree.", "Archivar").then((ok) => ok && act.mutate({ action: "archive" })) },
    ]} />
  ) : null;
  return { open, menu, error: act.error };
}
