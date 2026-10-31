import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api/client";
import { Badge, Card, ErrorText } from "../components/ui";

export const useT3 = (enabled = true) => useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 10_000, enabled });
export const useBlocked = (enabled = true) => useQuery({ queryKey: ["items", "blocked"], queryFn: () => api.items.list({ status: "blocked" }), enabled });

export function useAttentionCount(enabled: boolean) {
  const t3 = useT3(enabled);
  const blocked = useBlocked(enabled);
  const threads = t3.data?.reduce((n, s) => n + s.threads.filter((t) => t.attention).length, 0) ?? 0;
  return threads + (blocked.data?.length ?? 0);
}

export function Attention() {
  const t3 = useT3();
  const blocked = useBlocked();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const threads = t3.data?.flatMap((s) => s.threads.filter((t) => t.attention).map((t) => ({ ...t, status_: s }))) ?? [];
  const down = t3.data?.filter((s) => s.state !== "available") ?? [];
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  return (
    <div className="space-y-3">
      <h1 className="text-xl font-bold">Atención</h1>
      <ErrorText error={t3.error ?? blocked.error} />
      {down.map((s) => <p key={s.instance_id} className="text-sm text-slate-500 dark:text-slate-400">Sin datos de {envName(s.environment_id)}: instancia no disponible.</p>)}
      {threads.map((t) => (
        <Card key={t.id}>
          <a href={t.url} target="_blank" rel="noreferrer" className="block space-y-1">
            <div className="font-medium break-words">{t.title}</div>
            <div className="flex flex-wrap gap-1"><Badge>{envName(t.status_.environment_id)}</Badge><Badge>{t.status}</Badge><Badge>{t.model}</Badge></div>
          </a>
        </Card>
      ))}
      {blocked.data?.map((i) => (
        <Card key={i.id}>
          {i.project_id ? <Link to="/projects/$id" params={{ id: i.project_id }} className="font-medium break-words">{i.title}</Link> : <span className="font-medium break-words">{i.title}</span>}
          <p className="text-xs text-red-600 dark:text-red-400">{i.blocked_reason}</p>
        </Card>
      ))}
      {t3.isSuccess && blocked.isSuccess && threads.length + (blocked.data?.length ?? 0) === 0 && <p className="text-sm text-slate-500 dark:text-slate-400">Nada pendiente.</p>}
    </div>
  );
}
