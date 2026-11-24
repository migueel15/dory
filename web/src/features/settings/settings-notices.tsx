import { useQueryClient } from "@tanstack/react-query";
import { qk } from "../../api/queries/keys";
import { useNoticeSettings,useSaveNoticeSetting,type NoticeSetting } from "../../api/queries/settings";
import { ErrorText,Select } from "../../components/ui";

type Setting = NoticeSetting;

const MODES = [
  { value: "auto", label: "Automático" },
  { value: "push", label: "Siempre push" },
  { value: "app", label: "Solo en la app" },
  { value: "off", label: "Desactivado" },
];

// NoticesSettings chooses, for each kind of event, how it reaches you. The
// choice is shared by every device of the mesh.
export function NoticesSettings() {
  const qc = useQueryClient();
  const list = useNoticeSettings();
  const save = useSaveNoticeSetting({
    onSettled: () => qc.invalidateQueries({ queryKey: qk.noticeSettings() }),
  });
  return (
    <section aria-labelledby="notices-title" className="space-y-3">
      <h2 id="notices-title" className="text-lg font-semibold">Avisos</h2>
      <p className="text-sm text-slate-600 dark:text-muted">
        Automático: con Dory en primer plano el aviso sale dentro de la app; si no, llega como push al móvil o, sin móvil, como notificación del sistema.
        «Siempre push» lo envía aunque haya una interfaz con foco, «Solo en la app» nunca lo envía como push y «Desactivado» lo silencia. Se aplica a todos tus dispositivos.
      </p>
      <ul className="space-y-2">
        {(list.data ?? []).map((s) => (
          <li key={s.kind} className="flex items-center justify-between gap-3">
            <span className="min-w-0 text-sm">{s.title}</span>
            <Select className="w-44 shrink-0" value={s.mode} disabled={save.isPending} onChange={(mode) => save.mutate({ kind: s.kind, mode })} options={MODES} />
          </li>
        ))}
      </ul>
      <ErrorText error={list.error ?? save.error} />
    </section>
  );
}
