import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { req } from "../api/client";
import { ErrorText, Select } from "./ui";

type Setting = { kind: string; title: string; mode: string };

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
  const list = useQuery({ queryKey: ["notice-settings"], queryFn: () => req<Setting[]>("GET", "/notice-settings") });
  const save = useMutation({
    mutationFn: (s: { kind: string; mode: string }) => req("PUT", "/notice-settings", s),
    onSettled: () => qc.invalidateQueries({ queryKey: ["notice-settings"] }),
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
