import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { api, usable, type UsageRange, type UsageReport, type UsageRow, type UsageWindow } from "../api/client";
import { Card, ErrorText, Select, iconButton } from "../components/ui";
import { AreaChart, StackedBar, compact, percent, providerColor, usd } from "../components/usage-charts";
import { ProviderIcon, useNodes } from "../components/launch-picker";
import { expectedRemaining, paceTitle, windowLabel, windowMinutes, windowReset } from "../lib/usage-pace";


const cx = (...c: (string | false | undefined)[]) => c.filter(Boolean).join(" ");
const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" });

// Limit shows what is left of one usage window and the expected pace.
export function Limit({ w }: { w: UsageWindow }) {
  const reset = windowReset(w);
  const resetLabel = reset ? `Se reinicia el ${date(reset)}` : undefined;
  const used = Math.round(w.utilization * 100);
  const pace = expectedRemaining({ resets_at: reset, window_minutes: windowMinutes(w) || null });
  const tone = used >= 95 ? "bg-red-600" : used >= 80 ? "bg-amber-500" : "bg-emerald-600";
  return (
    <div className="space-y-1 text-sm" title={resetLabel && resetLabel + paceTitle(pace)}>
      <div className="flex justify-between gap-3"><span>{windowLabel(w)}</span><span>Queda {100 - used} %</span></div>
      <div className="relative h-2 rounded bg-slate-200 dark:bg-hover">
        <div className={`h-full rounded ${tone}`} style={{ width: `${Math.max(0, 100 - used)}%` }} />
        {pace !== null && <div aria-hidden="true" className="absolute -bottom-0.5 -top-0.5 w-0.5 -translate-x-1/2 bg-slate-900 dark:bg-white" style={{ left: `${pace}%` }} />}
      </div>
      {resetLabel && <p className="text-xs text-slate-500 dark:text-muted">{resetLabel}</p>}
    </div>
  );
}

// Limits shows the remaining allowance of each provider (requirement 4.7).
// The harness reports it while a turn runs, so it updates as the agents work.
function Limits({ only }: { only: string }) {
  const node = useQuery({ queryKey: ["node"], queryFn: api.node, refetchInterval: 30_000 });
  const usage = node.data?.usage ?? {};
  return (
    <div className="space-y-3">
      <ErrorText error={node.error} />
      {(node.data?.providers ?? []).filter(usable).filter((p) => !only || p.id === only).map((p) => (
        <Card key={p.id}>
          <p className="mb-2 flex items-center gap-2 font-semibold"><ProviderIcon driver={p.id} />{p.name}</p>
          {usage[p.id]?.length
            ? <div className="space-y-3">{usage[p.id].map((w) => <Limit key={w.kind} w={w} />)}</div>
            : <p className="text-sm text-slate-500 dark:text-muted">{p.capabilities.includes("usage") ? "Sin datos todavía: aparecen tras el primer turno de un agente." : "Este proveedor no informa de su uso."}</p>}
        </Card>
      ))}
    </div>
  );
}

type Tab = "cost" | "tokens" | "limits";
type Search = { tab?: Tab; range?: UsageRange; provider?: string; by?: "model" | "day"; envs?: string };
const RANGES: [UsageRange, string][] = [["24h", "24 h"], ["7d", "7 días"], ["30d", "30 días"], ["90d", "90 días"]];
const TABS: [Tab, string][] = [["cost", "Coste"], ["tokens", "Tokens"], ["limits", "Límites"]];
const PROVIDER_NAMES: Record<string, string> = { claude: "Claude Code", codex: "Codex", antigravity: "Antigravity" };
const TYPE_COLORS = { input: "#6366f1", cache_read: "#14b8a6", cache_write: "#f59e0b", output: "#ec4899" };
const TYPE_LABELS = { input: "Entrada", cache_read: "Lectura de caché", cache_write: "Escritura de caché", output: "Salida" };
type Kind = keyof typeof TYPE_COLORS;
const KINDS = Object.keys(TYPE_COLORS) as Kind[];

const tokensOf = (r: UsageRow) => r.tokens.input + r.tokens.output + r.tokens.cache_write + r.tokens.cache_read;
const costOf = (r: UsageRow) => r.cost.input + r.cost.output + r.cost.cache_write + r.cost.cache_read;

const segment = (active: boolean) => cx("min-h-11 rounded px-2.5 py-1 text-xs font-medium sm:min-h-0", active ? "bg-white text-slate-900 shadow-sm dark:bg-raised dark:text-white" : "text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white");
function Segmented<T extends string>({ value, options, onChange }: { value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div className="inline-flex rounded-md bg-slate-100 p-0.5 dark:bg-hover">
      {options.map(([v, l]) => <button key={v} type="button" className={segment(v === value)} onClick={() => onChange(v)}>{l}</button>)}
    </div>
  );
}

// buckets lists every hour or day of the range so empty ones still plot.
function buckets(report: UsageReport) {
  const out: Date[] = [];
  const now = Date.now();
  const d = new Date(report.from);
  if (report.bucket === "hour") d.setMinutes(0, 0, 0);
  while (d.getTime() <= now) {
    out.push(new Date(d));
    if (report.bucket === "hour") d.setHours(d.getHours() + 1); else d.setDate(d.getDate() + 1);
  }
  return out;
}

// merge adds up the reports of several environments; rows of the same
// bucket, provider and model are kept apart, the charts sum them anyway.
function merge(reports: UsageReport[]): UsageReport {
  const sessions: Record<string, number> = {};
  for (const r of reports) for (const [p, n] of Object.entries(r.sessions)) sessions[p] = (sessions[p] ?? 0) + n;
  const from = reports.map((r) => r.from).sort()[0];
  return { from, bucket: reports[0].bucket, rows: reports.flatMap((r) => r.rows), sessions };
}

// Usage shows the API-equivalent cost and the tokens read from the harness
// logs of this machine, and the current limits of each provider.
export function Usage() {
  const search = useSearch({ strict: false }) as Search;
  const navigate = useNavigate();
  const set = (patch: Search) => navigate({ to: "/usage", search: { ...search, ...patch } as never, replace: true });
  const tab = search.tab ?? "cost", range = search.range ?? "30d", provider = search.provider ?? "", by = search.by ?? "model";
  const nodes = useNodes();
  // Every active environment counts by default; envs narrows the selection.
  const picked = search.envs ? search.envs.split(",") : null;
  const selected = nodes.filter((n) => !picked || picked.includes(n.value));
  const toggle = (id: string) => {
    const current = selected.map((n) => n.value);
    const next = current.includes(id) ? current.filter((v) => v !== id) : [...current, id];
    set({ envs: next.length === nodes.length || !next.length ? undefined : next.join(",") });
  };
  const results = useQueries({ queries: selected.map((n) => ({ queryKey: ["usage-history", range, n.value], queryFn: () => api.usageHistory(range, n.value), enabled: tab !== "limits" })) });
  const loaded = results.filter((r) => r.data).map((r) => r.data!);
  const history = {
    isFetching: results.some((r) => r.isFetching),
    error: results.find((r) => r.error)?.error ?? null,
    data: results.length && results.every((r) => !r.isPending) && loaded.length ? merge(loaded) : undefined,
  };
  const qc = useQueryClient();
  const providers = Object.keys(PROVIDER_NAMES);
  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="flex items-center gap-1 text-lg font-semibold">
          <span>Uso</span><span className="text-slate-400">/</span>
          <Select plain value={provider} onChange={(v) => set({ provider: v || undefined })}
            options={[{ value: "", label: "Todos los proveedores" }, ...providers.map((p) => ({ value: p, label: PROVIDER_NAMES[p], icon: <ProviderIcon driver={p} /> }))]} />
        </h1>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {tab !== "limits" && <Segmented value={range} options={RANGES} onChange={(v) => set({ range: v })} />}
          <Segmented value={tab} options={TABS} onChange={(v) => set({ tab: v })} />
          <button type="button" className={iconButton} title="Recargar" aria-label="Recargar"
            onClick={() => qc.invalidateQueries({ queryKey: tab === "limits" ? ["node"] : ["usage-history"] })}>
            <svg viewBox="0 0 24 24" className={cx("size-4", history.isFetching && "animate-spin")} fill="none" stroke="currentColor" strokeWidth={2}><path d="M21 12a9 9 0 1 1-3-6.7L21 8M21 3v5h-5" /></svg>
          </button>
        </div>
      </div>
      {tab !== "limits" && nodes.length > 1 && (
        <div className="flex flex-wrap items-center gap-1.5 text-xs">
          <span className="text-slate-500 dark:text-muted">Entornos:</span>
          {nodes.map((n) => {
            const on = selected.some((s) => s.value === n.value);
            return <button key={n.value} type="button" aria-pressed={on} onClick={() => toggle(n.value)}
              className={cx("min-h-11 rounded-full border px-2.5 py-1 sm:min-h-0", on ? "border-slate-900 bg-slate-900 text-white dark:border-white dark:bg-white dark:text-slate-900" : "border-slate-300 text-slate-500 dark:border-border dark:text-muted")}>{n.label}</button>;
          })}
        </div>
      )}
      {tab === "limits" ? <Limits only={provider} /> : (
        <>
          <ErrorText error={history.error} />
          {history.data ? <History report={history.data} provider={provider} money={tab === "cost"} by={by} setBy={(v) => set({ by: v })} /> : !history.error && <p className="text-sm text-slate-500 dark:text-muted">Leyendo los registros…</p>}
        </>
      )}
    </div>
  );
}

function History({ report, provider, money, by, setBy }: { report: UsageReport; provider: string; money: boolean; by: "model" | "day"; setBy: (v: "model" | "day") => void }) {
  const rows = report.rows.filter((r) => !provider || r.provider === provider);
  const value = money ? costOf : tokensOf;
  const fmt = money ? usd : compact;
  const total = rows.reduce((a, r) => a + value(r), 0);
  const allTokens = rows.reduce((a, r) => a + tokensOf(r), 0);
  const allCost = rows.reduce((a, r) => a + costOf(r), 0);
  const unpriced = [...new Set(rows.filter((r) => !r.priced).map((r) => r.model))];
  const present = [...new Set(rows.map((r) => r.provider))].sort();
  const sessions = present.reduce((a, p) => a + (report.sessions[p] ?? 0), 0);
  const days = buckets(report);
  const hourly = report.bucket === "hour";
  const label = (d: Date) => hourly ? d.toLocaleTimeString("es-ES", { hour: "2-digit", minute: "2-digit" }) : d.toLocaleDateString("es-ES", { day: "numeric", month: "short" });
  const index = new Map(days.map((d, i) => [d.getTime(), i]));
  const series = present.map((p) => {
    const values = days.map(() => 0);
    for (const r of rows) if (r.provider === p) { const i = index.get(new Date(r.bucket).getTime()); if (i !== undefined) values[i] += value(r); }
    return { id: p, label: PROVIDER_NAMES[p] ?? p, color: providerColor(p), values };
  });
  const sum = (k: Kind) => rows.reduce((a, r) => a + (money ? r.cost[k] : r.tokens[k]), 0);
  const saving = rows.reduce((a, r) => a + r.cost.cache_saving, 0);
  const tokenSum = (k: Kind) => rows.reduce((a, r) => a + r.tokens[k], 0);
  const groups = new Map<string, { key: string; label: string; provider?: string; value: number; tokens: number; cost: number }>();
  for (const r of rows) {
    const key = by === "model" ? `${r.provider}/${r.model}` : r.bucket;
    const g = groups.get(key) ?? { key, label: by === "model" ? r.model : label(new Date(r.bucket)), provider: by === "model" ? r.provider : undefined, value: 0, tokens: 0, cost: 0 };
    g.value += value(r); g.tokens += tokensOf(r); g.cost += costOf(r);
    groups.set(key, g);
  }
  const table = [...groups.values()].sort((a, b) => by === "model" ? b.value - a.value : b.key.localeCompare(a.key));
  const top = Math.max(0, ...table.map((g) => g.value));
  const heading = "text-sm font-semibold";
  return (
    <div className="space-y-4">
      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <Card>
          <p className="text-3xl font-semibold tabular-nums">{fmt(total)}</p>
          <p className="text-sm text-slate-500 dark:text-muted">{sessions} sesiones{money ? " · estimación API" : " · tokens procesados"}</p>
          <div className="mt-4 divide-y divide-slate-100 dark:divide-border">
            {present.map((p) => {
              const pr = rows.filter((r) => r.provider === p);
              const v = pr.reduce((a, r) => a + value(r), 0);
              return (
                <div key={p} className="flex items-center gap-2 py-2 text-sm">
                  <span className="size-2 shrink-0 rounded-full" style={{ background: providerColor(p) }} />
                  <ProviderIcon driver={p} />
                  <div className="min-w-0">
                    <p className="truncate font-medium">{PROVIDER_NAMES[p] ?? p}</p>
                    <p className="text-xs text-slate-500 dark:text-muted">{report.sessions[p] ?? 0} sesiones</p>
                  </div>
                  <div className="ml-auto text-right tabular-nums">
                    <p>{fmt(v)} <span className="text-xs text-slate-500 dark:text-muted">{total ? percent(v / total) : ""}</span></p>
                    <p className="text-xs text-slate-500 dark:text-muted">{money ? `${compact(pr.reduce((a, r) => a + tokensOf(r), 0))} tokens` : usd(pr.reduce((a, r) => a + costOf(r), 0))}</p>
                  </div>
                </div>
              );
            })}
            {!present.length && <p className="py-2 text-sm text-slate-500 dark:text-muted">Sin uso en este periodo.</p>}
          </div>
          {money && unpriced.length > 0 && <p className="mt-2 text-xs text-slate-500 dark:text-muted">Sin precio (cuentan tokens, no coste): {unpriced.join(", ")}</p>}
        </Card>
        <Card>
          <p className={cx(heading, "mb-2")}>{money ? (hourly ? "Coste por hora" : "Coste diario") : (hourly ? "Tokens por hora" : "Tokens por día")}</p>
          <AreaChart labels={days.map(label)} series={series} format={money ? (n) => usd(n) : compact} />
        </Card>
      </div>
      <Card>
        <p className={cx(heading, "mb-3")}>Totales</p>
        <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-5">
          {([["Tokens procesados", compact(allTokens)], ["Entrada en caché", compact(tokenSum("cache_read"))], ["Entrada sin caché", compact(tokenSum("input") + tokenSum("cache_write"))], ["Salida", compact(tokenSum("output"))], ["Ahorro por caché", usd(saving)]] as const).map(([l, v]) => (
            <div key={l}><p className="text-xs text-slate-500 dark:text-muted">{l}</p><p className="text-lg font-semibold tabular-nums">{v}</p></div>
          ))}
        </div>
        {!money && <p className="mt-2 text-xs text-slate-500 dark:text-muted">Coste API equivalente del periodo: {usd(allCost)}</p>}
      </Card>
      <Card>
        <p className={cx(heading, "mb-3")}>{money ? "Coste por tipo" : "Tokens por tipo"}</p>
        <StackedBar format={fmt} parts={KINDS.map((k) => ({ label: TYPE_LABELS[k], color: TYPE_COLORS[k], value: sum(k) }))} />
      </Card>
      <Card>
        <div className="mb-3 flex items-center gap-2">
          <p className={heading}>Desglose</p>
          <div className="ml-auto"><Segmented value={by} options={[["model", "Modelo"], ["day", hourly ? "Hora" : "Día"]]} onChange={setBy} /></div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-slate-500 dark:text-muted">
              <tr><th className="w-8 py-1 font-normal">#</th><th className="py-1 font-normal">{by === "model" ? "Modelo" : hourly ? "Hora" : "Día"}</th><th className="py-1 text-right font-normal">{money ? "Coste" : "Tokens"}</th><th className="py-1 text-right font-normal">%</th><th className="py-1 text-right font-normal">{money ? "Tokens" : "Coste"}</th></tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-border">
              {table.map((g, i) => (
                <tr key={g.key}>
                  <td className="py-1.5 text-slate-500 dark:text-muted">{i + 1}</td>
                  <td className="py-1.5">
                    <div className="flex min-w-40 items-center gap-2">
                      {g.provider && <ProviderIcon driver={g.provider} model={g.label} />}
                      <span className="truncate">{g.label}</span>
                    </div>
                    <div className="mt-1 h-1 rounded-full" style={{ width: `${top ? (g.value / top) * 100 : 0}%`, background: g.provider ? providerColor(g.provider) : "#94a3b8" }} />
                  </td>
                  <td className="py-1.5 text-right tabular-nums">{fmt(g.value)}</td>
                  <td className="py-1.5 text-right tabular-nums text-slate-500 dark:text-muted">{total ? percent(g.value / total) : "—"}</td>
                  <td className="py-1.5 text-right tabular-nums text-slate-500 dark:text-muted">{money ? compact(g.tokens) : usd(g.cost)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
