import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type T3InstanceUsage } from "../api/client";
import { Badge, Button, Card, ErrorText } from "../components/ui";

const warnAt = 80;
const alertAt = 95;

function age(iso: string | null | undefined) {
  if (!iso) return "fecha desconocida";
  const minutes = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60_000));
  if (minutes < 1) return "hace menos de 1 min";
  if (minutes < 60) return `hace ${minutes} min`;
  return `hace ${Math.round(minutes / 60)} h`;
}

const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" });
const tokens = (n: number) => n.toLocaleString("es-ES");

function cost(usd: number, source: string) {
  if (source === "providerReported") return `${usd.toFixed(2)} USD`;
  if (source === "modelPriced") return `~${usd.toFixed(2)} USD (estimado)`;
  return "Sin coste";
}

function Limit({ w }: { w: T3InstanceUsage["providers"][number]["windows"][number] }) {
  const tone = w.used_percent >= alertAt ? "bg-red-600" : w.used_percent >= warnAt ? "bg-amber-500" : "bg-emerald-600";
  return (
    <div className="space-y-1 text-sm">
      <div className="flex flex-wrap justify-between gap-x-3">
        <span>{w.label}</span>
        <span>Queda {Math.round(100 - w.used_percent)} %</span>
      </div>
      <div className="h-2 overflow-hidden rounded bg-slate-200 dark:bg-slate-700"><div className={`h-full ${tone}`} style={{ width: `${Math.min(100, w.used_percent)}%` }} /></div>
      <p className="text-xs text-slate-500 dark:text-slate-400">
        {w.resets_at ? `Se reinicia el ${date(w.resets_at)}` : "Sin fecha de reinicio"}
        {w.used_percent >= alertAt ? " · Límite casi agotado" : w.used_percent >= warnAt ? " · Uso alto" : ""}
      </p>
    </div>
  );
}

function Models({ usage }: { usage: T3InstanceUsage }) {
  if (usage.models_error === "missing_scope") return <p className="text-sm text-slate-500 dark:text-slate-400">Consumo por modelo sin datos: el token de esta instancia no tiene el permiso diagnostics:read.</p>;
  if (usage.models_error) return <p className="text-sm text-slate-500 dark:text-slate-400">Consumo por modelo sin datos: la instancia no responde.</p>;
  if (usage.models.length === 0) return <p className="text-sm text-slate-500 dark:text-slate-400">Sin consumo en los últimos 7 días.</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs text-slate-500 dark:text-slate-400"><tr><th className="pr-3">Proveedor</th><th className="pr-3">Modelo</th><th className="pr-3">Entrada</th><th className="pr-3">Caché</th><th className="pr-3">Salida</th><th>Coste</th></tr></thead>
        <tbody>
          {usage.models.map((m) => (
            <tr key={`${m.provider}/${m.model}/${m.cost_source}`}>
              <td className="pr-3">{m.provider}</td><td className="pr-3 break-all">{m.model}</td><td className="pr-3">{tokens(m.input_tokens)}</td><td className="pr-3">{tokens(m.cached_tokens)}</td><td className="pr-3">{tokens(m.output_tokens)}</td><td>{cost(m.cost_usd, m.cost_source)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Usage() {
  const usage = useQuery({ queryKey: ["t3-usage"], queryFn: api.t3Usage, refetchInterval: 30_000 });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const qc = useQueryClient();
  const refresh = useMutation({ mutationFn: api.t3UsageRefresh, onSuccess: (data) => qc.setQueryData(["t3-usage"], data) });
  const { mutate } = refresh;
  useEffect(() => mutate(), [mutate]);
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-bold">Uso</h1>
        <Button variant="ghost" disabled={refresh.isPending} onClick={() => mutate()}>{refresh.isPending ? "Actualizando…" : "Actualizar"}</Button>
      </div>
      <ErrorText error={usage.error ?? envs.error ?? refresh.error} />
      {usage.data?.length === 0 && <p className="text-sm text-slate-500 dark:text-slate-400">Todavía no hay lecturas de uso.</p>}
      {usage.data?.map((u) => (
        <section key={u.instance_id} className="space-y-2">
          <div className="flex flex-wrap items-baseline gap-2">
            <h2 className="font-semibold">{envName(u.environment_id)}</h2>
            <span className="text-xs text-slate-500 dark:text-slate-400">Leído {age(u.fetched_at)}</span>
          </div>
          {u.limits_error && <p className="text-sm text-slate-500 dark:text-slate-400">Sin datos: la instancia no responde.</p>}
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {u.providers.map((p) => (
              <Card key={p.provider}>
                <div className="mb-2 flex flex-wrap items-center gap-2"><span className="font-medium">{p.name}</span>{p.windows.length > 0 && <Badge>{age(p.checked_at)}</Badge>}</div>
                {p.windows.length > 0 ? <div className="space-y-3">{p.windows.map((w) => <Limit key={w.label} w={w} />)}</div> : <p className="text-sm text-slate-500 dark:text-slate-400">Sin datos de plan.</p>}
              </Card>
            ))}
          </div>
          {!u.limits_error && <Card><h3 className="mb-2 text-sm font-medium">Consumo de los últimos 7 días</h3><Models usage={u} /></Card>}
        </section>
      ))}
    </div>
  );
}
