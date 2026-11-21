import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type NodeSettings } from "../api/client";
import { ErrorText } from "./ui";

// TaskSettings holds what this node does when a task is done or cancelled and
// when a thread is archived.
export function TaskSettings() {
  const qc = useQueryClient();
  const current = useQuery({ queryKey: ["node-settings"], queryFn: () => api.nodeSettings() });
  const save = useMutation({ mutationFn: (s: NodeSettings) => api.setNodeSettings(s), onSuccess: (data) => qc.setQueryData(["node-settings"], data) });
  const s = current.data;
  const toggle = (key: keyof NodeSettings, title: string, help: string) => (
    <label className="flex items-start gap-3 text-sm">
      <input type="checkbox" className="mt-1" checked={s?.[key] ?? false} disabled={!s || save.isPending} onChange={(e) => s && save.mutate({ ...s, [key]: e.target.checked })} />
      <span><strong className="block font-medium">{title}</strong><span className="text-slate-600 dark:text-muted">{help}</span></span>
    </label>
  );
  return (
    <section aria-labelledby="tasks-title" className="space-y-4">
      <h2 id="tasks-title" className="text-lg font-semibold">Tareas e hilos</h2>
      {toggle("archive_threads_on_finish", "Archivar hilos al terminar la tarea", "Cuando una tarea pasa a hecha o cancelada, a mano o porque su pull request se mergea o se cierra, se archivan sus hilos y se detienen los que estén en marcha.")}
      {toggle("remove_worktree_on_archive", "Eliminar worktrees al archivar", "Al archivar un hilo se borra su worktree si está limpio y ningún otro hilo lo usa. Con cambios sin commitear se conserva y el hilo archivado lo avisa.")}
      <h2 className="pt-2 text-lg font-semibold">Dispositivos</h2>
      {toggle("agents_use_devices", "Los agentes pueden usar dispositivos", "Los agentes de este nodo ven y controlan los móviles y emuladores Android (pantalla, toques, texto, apps). Un agente puede enviar mensajes o tocar datos reales del móvil: sus acciones se ven en directo en el panel Dispositivo y quedan en el hilo. Desactivado por defecto.")}
      <ErrorText error={current.error ?? save.error} />
    </section>
  );
}
