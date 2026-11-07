import { expectedRemaining, paceTitle } from "../lib/usage-pace";
import { useEffect } from "react";
import { UsageHistory } from "./usage-history";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type T3InstanceUsage } from "../api/client";
import { Button, Card, ErrorText } from "../components/ui";

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

export function Limit({ w }: { w: T3InstanceUsage["providers"][number]["windows"][number] }) {
  const pace = expectedRemaining(w);
  const tone = w.used_percent >= alertAt ? "bg-red-600" : w.used_percent >= warnAt ? "bg-amber-500" : "bg-emerald-600";
  return (
    <div className="space-y-1 text-sm" title={(w.resets_at ? `Se reinicia el ${date(w.resets_at)}` : "Sin fecha de reinicio") + paceTitle(pace)}>
      <div className="flex flex-wrap justify-between gap-x-3">
        <span>{w.label}</span>
        <span>Queda {Math.round(100 - w.used_percent)} %</span>
      </div>
      <div className="h-2 relative rounded bg-slate-200 dark:bg-hover"><div className={`h-full rounded ${tone}`} style={{ width: `${Math.max(0, 100 - w.used_percent)}%` }} />{pace !== null && <div aria-hidden="true" className="absolute -top-0.5 -bottom-0.5 w-0.5 -translate-x-1/2 bg-slate-900 dark:bg-white" style={{ left: `${pace}%` }} />}</div>
      <p className="text-xs text-slate-500 dark:text-muted">
        {w.resets_at ? `Se reinicia el ${date(w.resets_at)}` : "Sin fecha de reinicio"}
        {w.used_percent >= alertAt ? " · Límite casi agotado" : w.used_percent >= warnAt ? " · Uso alto" : ""}
      </p>
    </div>
  );
}

export function Models({ usage }: { usage: T3InstanceUsage }) {
  if (usage.models_error === "missing_scope") return <p className="text-sm text-slate-500 dark:text-muted">Consumo por modelo sin datos: el token de esta instancia no tiene el permiso diagnostics:read.</p>;
  if (usage.models_error) return <p className="text-sm text-slate-500 dark:text-muted">Consumo por modelo sin datos: la instancia no responde.</p>;
  if (usage.models.length === 0) return <p className="text-sm text-slate-500 dark:text-muted">Sin consumo en los últimos 7 días.</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs text-slate-500 dark:text-muted"><tr><th className="pr-3">Proveedor</th><th className="pr-3">Modelo</th><th className="pr-3">Entrada</th><th className="pr-3">Caché</th><th className="pr-3">Salida</th><th>Coste</th></tr></thead>
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

// LimitRow is the compact form of Limit: one line with label, bar, remaining and reset.
function LimitRow({ w }: { w: T3InstanceUsage["providers"][number]["windows"][number] }) {
  const pace = expectedRemaining(w);
  const tone = w.used_percent >= alertAt ? "bg-red-600" : w.used_percent >= warnAt ? "bg-amber-500" : "bg-emerald-600";
  const left = Math.round(100 - w.used_percent);
  return (
    <div className="grid grid-cols-[minmax(0,7rem)_1fr_3rem] items-center gap-2 text-xs" title={(w.resets_at ? `Se reinicia el ${date(w.resets_at)}` : "Sin fecha de reinicio") + paceTitle(pace)}>
      <span className="truncate text-slate-600 dark:text-text-secondary">{w.label}</span>
      <div className="h-1.5 relative rounded bg-slate-200 dark:bg-hover"><div className={`h-full rounded ${tone}`} style={{ width: `${Math.max(0, 100 - w.used_percent)}%` }} />{pace !== null && <div aria-hidden="true" className="absolute -top-0.5 -bottom-0.5 w-0.5 -translate-x-1/2 bg-slate-900 dark:bg-white" style={{ left: `${pace}%` }} />}</div>
      <span className={`text-right tabular-nums ${w.used_percent >= alertAt ? "font-semibold text-red-600 dark:text-red-400" : w.used_percent >= warnAt ? "text-amber-700 dark:text-amber-400" : ""}`}>{left} %</span>
    </div>
  );
}

// Usage shows one compact card per instance, always in the same order (by name).
export function Usage() {
  const usage = useQuery({ queryKey: ["t3-usage"], queryFn: api.t3Usage, refetchInterval: 30_000 });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const qc = useQueryClient();
  const refresh = useMutation({ mutationFn: api.t3UsageRefresh, onSuccess: (data) => qc.setQueryData(["t3-usage"], data) });
  const { mutate } = refresh;
  useEffect(() => { mutate(); }, [mutate]);
  const sorted = [...(usage.data ?? [])].sort((a, b) => envName(a.environment_id).localeCompare(envName(b.environment_id)) || a.instance_id.localeCompare(b.instance_id));
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-bold">Uso</h1>
        <Button variant="ghost" disabled={refresh.isPending} onClick={() => mutate()}>{refresh.isPending ? "Actualizando…" : "Actualizar"}</Button>
      </div>
      <ErrorText error={usage.error ?? envs.error ?? refresh.error} />
      {usage.data?.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Todavía no hay lecturas de uso.</p>}
      <div className="grid gap-3 lg:grid-cols-2 2xl:grid-cols-3">
        {sorted.map((u) => {
          const providers = [...u.providers].sort((a, b) => a.name.localeCompare(b.name));
          const spent = u.models.reduce((t, m) => t + m.cost_usd, 0);
          const used = u.models.reduce((t, m) => t + m.input_tokens + m.output_tokens, 0);
          return (
            <Card key={u.instance_id} className="space-y-3">
              <div className="flex items-baseline justify-between gap-2">
                <h2 className="truncate font-semibold">{envName(u.environment_id) || u.instance_id}</h2>
                <span className="shrink-0 text-xs text-slate-500 dark:text-muted">Leído {age(u.fetched_at)}</span>
              </div>
              {u.limits_error ? <p className="text-sm text-slate-500 dark:text-muted">Sin datos: la instancia no responde.</p> : (
                <div className="space-y-2.5">
                  {providers.map((p) => (
                    <div key={p.provider} className="space-y-1">
                      <div className="text-sm font-medium">{p.name}</div>
                      {p.windows.length > 0 ? p.windows.map((w) => <LimitRow key={w.label} w={w} />) : <p className="text-xs text-slate-500 dark:text-muted">Sin datos de plan.</p>}
                    </div>
                  ))}
                </div>
              )}
              {!u.limits_error && (
                <details className="group border-t border-slate-200 pt-2 dark:border-border">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-2 text-xs text-slate-500 dark:text-muted">
                    <span>7 días{u.models.length > 0 && ` · ${tokens(used)} tokens · ${spent.toFixed(2)} USD`}</span>
                    <span aria-hidden="true" className="transition-transform group-open:rotate-90 motion-reduce:transition-none">›</span>
                  </summary>
                  <div className="mt-2"><Models usage={u} /></div>
                </details>
              )}
            </Card>
          );
        })}
      </div>
      <UsageHistory envName={envName} />
    </div>
  );
}
