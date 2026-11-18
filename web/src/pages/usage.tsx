import { useQuery } from "@tanstack/react-query";
import { api, usable, type UsageWindow } from "../api/client";
import { Card, ErrorText, PageHeader } from "../components/ui";
import { ProviderIcon } from "../components/launch-picker";
import { expectedRemaining, paceTitle, WINDOWS } from "../lib/usage-pace";


const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" });

// Limit shows what is left of one usage window and the expected pace.
export function Limit({ w }: { w: UsageWindow }) {
  const [label, minutes] = WINDOWS[w.kind] ?? [w.kind, 0];
  const used = Math.round(w.utilization * 100);
  const pace = expectedRemaining({ resets_at: w.resets_at, window_minutes: minutes || null });
  const tone = used >= 95 ? "bg-red-600" : used >= 80 ? "bg-amber-500" : "bg-emerald-600";
  return (
    <div className="space-y-1 text-sm" title={`Se reinicia el ${date(w.resets_at)}${paceTitle(pace)}`}>
      <div className="flex justify-between gap-3"><span>{label}</span><span>Queda {100 - used} %</span></div>
      <div className="relative h-2 rounded bg-slate-200 dark:bg-hover">
        <div className={`h-full rounded ${tone}`} style={{ width: `${Math.max(0, 100 - used)}%` }} />
        {pace !== null && <div aria-hidden="true" className="absolute -bottom-0.5 -top-0.5 w-0.5 -translate-x-1/2 bg-slate-900 dark:bg-white" style={{ left: `${pace}%` }} />}
      </div>
      <p className="text-xs text-slate-500 dark:text-muted">Se reinicia el {date(w.resets_at)}</p>
    </div>
  );
}

// Usage shows the remaining allowance of each provider (requirement 4.7). The
// harness reports it while a turn runs, so it updates as the agents work.
export function Usage() {
  const node = useQuery({ queryKey: ["node"], queryFn: api.node, refetchInterval: 30_000 });
  const usage = node.data?.usage ?? {};
  return (
    <div className="mx-auto max-w-3xl space-y-3">
      <PageHeader title="Uso" subtitle="Lo que te queda de cada proveedor, según lo último que informó." />
      <ErrorText error={node.error} />
      {(node.data?.providers ?? []).filter(usable).map((p) => (
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
