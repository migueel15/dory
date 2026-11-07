import { useState, type ReactNode } from "react";
import { Info } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { api, type T3DayUsage } from "../api/client";
import { Card, ErrorText, Select } from "../components/ui";

// Validated categorical slots (light / dark), assigned to providers by name, never by rank.
const SLOTS = [
  "bg-[#2a78d6] dark:bg-[#3987e5]", "bg-[#eb6834] dark:bg-[#d95926]", "bg-[#1baf7a] dark:bg-[#199e70]", "bg-[#eda100] dark:bg-[#c98500]",
  "bg-[#e87ba4] dark:bg-[#d55181]", "bg-[#008300] dark:bg-[#008300]", "bg-[#6250d6] dark:bg-[#9085e9]", "bg-[#e34948] dark:bg-[#e66767]",
];
const RANGES = [7, 30, 90] as const;
const MAX_DAYS = 90;
type Metric = "cost" | "tokens";

const usd = (n: number) => n.toLocaleString("es-ES", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 2 });
const short = (n: number) => n.toLocaleString("es-ES", { notation: "compact", maximumFractionDigits: 2 });
const processed = (d: T3DayUsage) => d.uncached_tokens + d.cached_tokens + d.cache_write_tokens + d.output_tokens;
const pct = (part: number, whole: number) => (whole ? (part / whole) * 100 : 0);
const share = (p: number) => (p > 0 && p < 0.1 ? "<0,1 %" : `${p.toLocaleString("es-ES", { maximumFractionDigits: 1 })} %`);

function Segmented<T extends string | number>({ value, options, onChange, label }: { value: T; options: { value: T; label: string }[]; onChange: (v: T) => void; label: string }) {
  return (
    <div role="group" aria-label={label} className="inline-flex rounded-md border border-slate-200 bg-slate-100 p-0.5 dark:border-border dark:bg-raised">
      {options.map((o) => (
        <button key={String(o.value)} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}
          className={`rounded px-2.5 py-1 text-xs font-medium ${o.value === value ? "bg-white shadow-sm dark:bg-hover dark:text-white" : "text-slate-600 hover:text-slate-900 dark:text-text-secondary dark:hover:text-white"}`}>{o.label}</button>
      ))}
    </div>
  );
}

const sum = (rows: T3DayUsage[], f: (d: T3DayUsage) => number) => rows.reduce((t, d) => t + f(d), 0);

// UsageHistory mirrors T3 Code's usage view: range, cost or tokens, by provider, per day and per model.
export function UsageHistory({ envName }: { envName: (id: string) => string }) {
  const [days, setDays] = useState<number>(30);
  const [metric, setMetric] = useState<Metric>("cost");
  const [env, setEnv] = useState("");
  const [by, setBy] = useState<"model" | "day">("model");
  const [hover, setHover] = useState<string>();
  // One read of the longest range feeds every range and both measures, so switching never waits.
  const history = useQuery({ queryKey: ["t3-usage-history", MAX_DAYS], queryFn: () => api.t3UsageHistory(MAX_DAYS), staleTime: 5 * 60_000, gcTime: 30 * 60_000 });

  const instances = history.data ?? [];
  // One slot per calendar day of the range, so empty days still show.
  const today = new Date();
  const calendar = Array.from({ length: days }, (_, i) => new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - (days - 1 - i))).toISOString().slice(0, 10));
  const rows = instances.filter((h) => !env || h.environment_id === env).flatMap((h) => h.days).filter((d) => d.day >= calendar[0]);
  const failed = instances.filter((h) => h.error && (!env || h.environment_id === env));
  const value = (d: T3DayUsage) => (metric === "cost" ? d.cost_usd : processed(d));
  const fmt = (n: number) => (metric === "cost" ? usd(n) : short(n));
  const total = sum(rows, value);
  const estimated = rows.some((d) => d.cost_source === "modelPriced");

  // Colors follow the provider name across every filter.
  const allProviders = [...new Set(instances.flatMap((h) => h.days.map((d) => d.provider)))].sort();
  const color = (p: string) => SLOTS[allProviders.indexOf(p) % SLOTS.length];
  const providers = [...new Set(rows.map((d) => d.provider))]
    .map((p) => { const r = rows.filter((d) => d.provider === p); return { p, value: sum(r, value), tokens: sum(r, processed) }; })
    .sort((a, b) => b.value - a.value);

  const perDay = calendar.map((day) => {
    const r = rows.filter((d) => d.day === day);
    return { day, total: sum(r, value), parts: allProviders.map((p) => ({ p, v: sum(r.filter((d) => d.provider === p), value) })).filter((x) => x.v > 0), rows: r };
  });
  const peak = Math.max(...perDay.map((d) => d.total), 0) || 1;
  const dayLabel = (day: string, long = false) => new Date(`${day}T00:00:00Z`).toLocaleDateString("es-ES", { day: "numeric", month: long ? "long" : "short", timeZone: "UTC" });

  const models = [...new Set(rows.map((d) => `${d.provider}\u0000${d.model}`))]
    .map((k) => { const [p, m] = k.split("\u0000"); const r = rows.filter((d) => d.provider === p && d.model === m); return { p, m, cost: sum(r, (d) => d.cost_usd), tokens: sum(r, processed), value: sum(r, value) }; })
    .sort((a, b) => b.value - a.value);
  const envs = [...new Set(instances.map((h) => h.environment_id))];

  return (
    <Card className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="mr-auto font-semibold">Consumo</h2>
        <Select className="w-44" value={env} onChange={setEnv} options={[{ value: "", label: "Todos los entornos" }, ...envs.map((id) => ({ value: id, label: envName(id) || "Instancia" }))]} />
        <Segmented label="Rango" value={days} onChange={setDays} options={RANGES.map((d) => ({ value: d, label: `${d} días` }))} />
        <Segmented label="Medida" value={metric} onChange={setMetric} options={[{ value: "cost", label: "Coste" }, { value: "tokens", label: "Tokens" }]} />
      </div>
      <ErrorText error={history.error} />
      {history.isPending ? <UsageSkeleton /> : (
        <>
          <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
            <div className="space-y-4">
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-3xl font-bold tabular-nums">{fmt(total)}</span>
                  {failed.length > 0 && (
                    <InfoTip title="Datos incompletos">
                      <p>Sin datos de:</p>
                      <ul className="mt-1 space-y-0.5">{failed.map((h) => <li key={h.instance_id} className="flex items-center gap-1.5"><span className="size-1.5 shrink-0 rounded-full bg-amber-500" />{envName(h.environment_id) || "Instancia"}<span className="text-slate-400 dark:text-muted">· {h.error === "missing_scope" ? "falta el permiso diagnostics:read" : "no responde"}</span></li>)}</ul>
                      <p className="mt-1.5 text-slate-500 dark:text-muted">El total no incluye su consumo.</p>
                    </InfoTip>
                  )}
                </div>
                <div className="text-xs text-slate-500 dark:text-muted">Últimos {days} días{metric === "cost" && estimated ? " · parte estimada por precio de modelo" : ""}</div>
              </div>
              <ul className="space-y-2.5">
                {providers.map((x) => (
                  <li key={x.p} className="text-sm">
                    <div className="flex items-center gap-2"><span className={`size-2.5 rounded-sm ${color(x.p)}`} /><span className="flex-1 font-medium">{x.p}</span><span className="tabular-nums">{fmt(x.value)}</span></div>
                    <div className="pl-4.5 text-xs text-slate-500 dark:text-muted">{share(pct(x.value, total))} del total · {short(x.tokens)} tokens</div>
                  </li>
                ))}
                {providers.length === 0 && <li className="text-sm text-slate-500 dark:text-muted">Sin consumo en este rango.</li>}
              </ul>
            </div>
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">{metric === "cost" ? "Coste diario" : "Tokens diarios"}</h3>
              <div className="relative">
                <div className="pointer-events-none absolute inset-x-0 top-0 flex h-48 flex-col justify-between">
                  {[1, 0.5, 0].map((f) => <div key={f} className="flex items-center gap-2 border-t border-slate-100 dark:border-border/60"><span className="-mt-3 bg-white pr-1 text-[10px] text-slate-400 dark:bg-surface dark:text-muted">{fmt(peak * f)}</span></div>)}
                </div>
                <div className="relative flex h-48 items-end gap-px pl-14">
                  {perDay.map((d) => (
                    <div key={d.day} onMouseEnter={() => setHover(d.day)} onMouseLeave={() => setHover(undefined)} className="relative flex h-full min-w-0 flex-1 flex-col justify-end hover:bg-slate-100/70 dark:hover:bg-raised/60">
                      <div className="flex flex-col-reverse gap-px" style={{ height: `${pct(d.total, peak)}%` }}>
                        {d.parts.map((x, i) => <div key={x.p} className={`${color(x.p)} ${i === d.parts.length - 1 ? "rounded-t-sm" : ""}`} style={{ height: `${pct(x.v, d.total)}%`, minHeight: 1 }} />)}
                      </div>
                      {hover === d.day && (
                        <div className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-1 w-48 -translate-x-1/2 rounded-lg border border-slate-200 bg-white p-2 text-xs shadow-lg dark:border-border dark:bg-surface">
                          <div className="mb-1 font-medium">{dayLabel(d.day, true)}</div>
                          {d.parts.length ? d.parts.map((x) => <div key={x.p} className="flex justify-between gap-2"><span className="flex items-center gap-1.5"><span className={`size-2 rounded-sm ${color(x.p)}`} />{x.p}</span><span className="tabular-nums">{fmt(x.v)}</span></div>) : <div className="text-slate-500 dark:text-muted">Sin consumo</div>}
                          {d.parts.length > 1 && <div className="mt-1 flex justify-between border-t border-slate-200 pt-1 dark:border-border"><span>Total</span><span className="tabular-nums">{fmt(d.total)}</span></div>}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                <div className="mt-1 flex justify-between pl-14 text-[10px] uppercase text-slate-400 dark:text-muted">
                  <span>{dayLabel(calendar[0])}</span><span>{dayLabel(calendar[Math.floor(days / 2)])}</span><span>{dayLabel(calendar[days - 1])}</span>
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-2">
            <h3 className="text-sm font-medium">Totales</h3>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
              {[
                ["Tokens procesados", short(sum(rows, processed))],
                ["Entrada en caché", short(sum(rows, (d) => d.cached_tokens))],
                ["Entrada sin caché", short(sum(rows, (d) => d.uncached_tokens + d.cache_write_tokens))],
                ["Salida", short(sum(rows, (d) => d.output_tokens))],
                ["Ahorro por caché", usd(sum(rows, (d) => d.cache_savings_usd))],
              ].map(([k, v]) => <div key={k}><div className="text-xs text-slate-500 dark:text-muted">{k}</div><div className="text-lg font-semibold tabular-nums">{v}</div></div>)}
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <h3 className="text-sm font-medium">Desglose</h3>
              <Segmented label="Desglose por" value={by} onChange={setBy} options={[{ value: "model", label: "Modelo" }, { value: "day", label: "Día" }]} />
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-slate-200 text-xs text-slate-500 dark:border-border dark:text-muted">
                  <tr><th className="w-8 py-1.5">#</th><th className="py-1.5">{by === "model" ? "Modelo" : "Día"}</th><th className="py-1.5 text-right">Coste</th><th className="py-1.5 text-right">Cuota</th><th className="py-1.5 text-right">Tokens</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-border/60">
                  {(by === "model"
                    ? models.map((x) => ({ key: `${x.p}/${x.m}`, name: <span className="flex items-center gap-2"><span className={`size-2 shrink-0 rounded-sm ${color(x.p)}`} /><span className="break-all">{x.m}</span></span>, cost: x.cost, tokens: x.tokens, value: x.value }))
                    : [...perDay].reverse().filter((d) => d.rows.length).map((d) => ({ key: d.day, name: <span>{dayLabel(d.day, true)}</span>, cost: sum(d.rows, (r) => r.cost_usd), tokens: sum(d.rows, processed), value: d.total }))
                  ).map((r, i) => (
                    <tr key={r.key}>
                      <td className="py-2 text-xs text-slate-400 dark:text-muted">{i + 1}</td>
                      <td className="py-2">{r.name}</td>
                      <td className="py-2 text-right tabular-nums">{usd(r.cost)}</td>
                      <td className="py-2 text-right tabular-nums text-slate-500 dark:text-muted">{share(pct(r.value, total))}</td>
                      <td className="py-2 text-right tabular-nums">{short(r.tokens)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </Card>
  );
}

// UsageSkeleton holds the final layout while T3 scans, with bars that rise and fall in gray.
function UsageSkeleton() {
  const bar = "rounded bg-slate-200 motion-safe:animate-pulse dark:bg-raised";
  return (
    <div role="status" aria-label="Cargando consumo" className="space-y-5">
      <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
        <div className="space-y-4">
          <div className="space-y-2"><div className={`h-8 w-36 ${bar}`} /><div className={`h-3 w-44 ${bar}`} /></div>
          {[0, 1, 2].map((i) => (
            <div key={i} className="space-y-1.5">
              <div className="flex items-center gap-2"><div className={`size-2.5 ${bar}`} /><div className={`h-3.5 flex-1 ${bar}`} style={{ maxWidth: `${60 - i * 12}%` }} /><div className={`h-3.5 w-14 ${bar}`} /></div>
              <div className={`ml-4.5 h-3 w-32 ${bar}`} />
            </div>
          ))}
        </div>
        <div className="space-y-2">
          <div className={`h-3.5 w-24 ${bar}`} />
          <div className="flex h-48 items-end gap-px border-b border-slate-100 pl-14 dark:border-border/60">
            {Array.from({ length: 30 }, (_, i) => (
              <div key={i} className="skel-bar flex-1 origin-bottom rounded-t-sm bg-slate-200 dark:bg-raised"
                style={{ height: `${30 + ((i * 37) % 60)}%`, animationDelay: `${(i % 10) * 0.12}s` }} />
            ))}
          </div>
          <div className="flex justify-between pl-14">{[0, 1, 2].map((i) => <div key={i} className={`h-2.5 w-10 ${bar}`} />)}</div>
        </div>
      </div>
      <div className="space-y-2">
        <div className={`h-3.5 w-16 ${bar}`} />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">{[0, 1, 2, 3, 4].map((i) => <div key={i} className="space-y-1.5"><div className={`h-3 w-20 ${bar}`} /><div className={`h-5 w-14 ${bar}`} /></div>)}</div>
      </div>
      <div className="space-y-2">
        <div className={`h-3.5 w-20 ${bar}`} />
        {[0, 1, 2, 3].map((i) => <div key={i} className="flex items-center gap-3 border-b border-slate-100 py-2 dark:border-border/60"><div className={`h-3 w-4 ${bar}`} /><div className={`h-3.5 flex-1 ${bar}`} style={{ maxWidth: `${45 - i * 6}%` }} /><div className={`ml-auto h-3.5 w-14 ${bar}`} /><div className={`h-3.5 w-10 ${bar}`} /><div className={`h-3.5 w-12 ${bar}`} /></div>)}
      </div>
    </div>
  );
}

// InfoTip is a yellow info icon that opens a small card on hover, focus or tap.
function InfoTip({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <span className="relative inline-flex" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button type="button" aria-label={title} aria-expanded={open} onClick={() => setOpen(!open)} onFocus={() => setOpen(true)} onBlur={() => setOpen(false)}
        className="grid size-6 place-items-center rounded-full text-amber-500 hover:bg-amber-50 dark:hover:bg-amber-950/40">
        <Info weight="fill" aria-hidden="true" className="size-5" />
      </button>
      {open && (
        <div role="tooltip" className="notif-in absolute left-0 top-full z-30 mt-1.5 w-72 rounded-lg border border-slate-200 bg-white p-3 text-xs shadow-lg dark:border-border dark:bg-surface">
          <div className="mb-1 text-sm font-semibold">{title}</div>
          {children}
        </div>
      )}
    </span>
  );
}
