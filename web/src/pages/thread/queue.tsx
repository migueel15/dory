import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Lightning, PencilSimple, Trash } from "@phosphor-icons/react";
import { api, type T3QueuedRun, type T3ThreadView } from "../../api/client";
import { Button, ErrorText, Input } from "../../components/ui";

// Queue shows the messages waiting for the active turn as outlined bubbles
// at the end of the conversation, each with its actions underneath.
export function Queue({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey });
  const act = useMutation({ mutationFn: (fn: () => Promise<void>) => fn(), onSuccess: done });
  if (!v.queue.length) return null;
  const q = v.queue;
  return (
    <section aria-label="Mensajes en cola" className="space-y-3 border-t border-dashed border-slate-300 pt-3 dark:border-border">
      {q.map((r) => (
        <QueuedRow key={r.run_id} r={r} canSteer={!!v.active_run_id}
          onSteer={() => act.mutate(() => api.t3QueueSteer(v.id, r.run_id))}
          onCancel={() => act.mutate(() => api.t3QueueCancel(v.id, r.run_id))}
          onEdit={(text) => act.mutate(() => api.t3QueueEdit(v.id, r.run_id, text))} />
      ))}
      <ErrorText error={act.error} />
    </section>
  );
}

export const iconBtn = "grid size-7 place-items-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

export function QueuedRow({ r, canSteer, onSteer, onCancel, onEdit }: { r: T3QueuedRun; canSteer: boolean; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  return (
    <div className="msg-in flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <p className="whitespace-pre-wrap break-words text-sm">{r.text}</p> : (
          <form className="flex gap-1" onSubmit={(e) => { e.preventDefault(); onEdit(draft); setDraft(null); }}>
            <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setDraft(null); }} />
            <Button>Guardar</Button>
          </form>
        )}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={iconBtn} title="Mandar ya (steer: inyectar en el turno en curso)" disabled={!canSteer} onClick={onSteer}><Lightning /></button>
        <button type="button" className={iconBtn} title="Editar" onClick={() => setDraft(r.text)}><PencilSimple /></button>
        <button type="button" className={iconBtn} title="Eliminar de la cola" onClick={onCancel}><Trash /></button>
      </div>
    </div>
  );
}
