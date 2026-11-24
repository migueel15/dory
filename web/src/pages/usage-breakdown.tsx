import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { api, type UsageBreakdownRow, type UsageRange, type UsageTurnCall } from "../api/client";
import { Card, ErrorText, Select } from "../components/ui";
import { compact, percent, usd } from "../components/usage-charts";
import { qk } from "../api/queries/keys";
import { type Kind, KINDS, TYPE_LABELS, date, PROVIDER_NAMES, cx } from "./usage-shared";

export const CATEGORY_LABELS: Record<string, string> = {
  base: "Contexto sin atribuir", system: "Instrucciones de Dory", user: "Mensajes del usuario", history: "Conversación",
  response: "Respuestas del agente", reasoning: "Razonamiento", tool: "Otras herramientas", subagent: "Subagentes",
  mcp: "Llamadas MCP", skill: "Skills", command: "Comandos", read: "Lectura de archivos",
  unattributed_output: "Salida sin atribuir",
};

export function Breakdown({ rows, range, provider }: { rows: (UsageBreakdownRow & { node: string })[]; range: UsageRange; provider: string }) {
  const [thread, setThread] = useState("");
  const threads = new Map(rows.map((r) => [r.thread, r.title || r.thread.slice(0, 8)]));
  const selected = threads.has(thread) ? thread : "";
  const visible = rows.filter((r) => !selected || r.thread === selected);
  const node = visible[0]?.node;
  const totalOf = (r: Pick<UsageBreakdownRow, Kind>) => KINDS.reduce((sum, kind) => sum + r[kind], 0);
  const sumOf = (group: UsageBreakdownRow[], kind: Kind) => group.reduce((sum, r) => sum + r[kind], 0);
  const total = visible.reduce((sum, row) => sum + totalOf(row), 0);
  const costOfGroup = (group: UsageBreakdownRow[]) => group.reduce((sum, row) => sum + row.cost, 0);
  const priced = visible.reduce((sum, row) => sum + row.priced_tokens, 0);
  const cost = costOfGroup(visible);
  const groups = new Map<string, UsageBreakdownRow[]>();
  for (const row of visible) {
    const key = row.category === "tool" ? row.label.split(":", 1)[0] : row.category;
    const category = CATEGORY_LABELS[key] ? key : "tool";
    const group = groups.get(category) ?? [];
    group.push(row);
    groups.set(category, group);
  }
  const summary = [...groups].map(([key, group]) => ({ key, group, tokens: group.reduce((sum, row) => sum + totalOf(row), 0) }))
    .sort((a, b) => b.tokens - a.tokens);
  if (!rows.length) return <Card><p className="text-sm">Sin turnos registrados en este periodo.</p></Card>;
  return (
    <div className="space-y-3">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <p className="text-lg font-semibold tabular-nums">{compact(total)} tokens registrados</p>
            <p className="text-sm text-slate-500 dark:text-muted">Reparto estimado de los turnos que Dory ha registrado.</p>
          </div>
          <div className="flex items-center gap-2">
            <Select value={selected} onChange={setThread} options={[{ value: "", label: "Todos los hilos" },
              ...[...threads].sort((a, b) => a[1].localeCompare(b[1])).map(([value, label]) => ({ value, label }))]} />
            {selected && <Link className="text-sm underline" to="/threads/$threadId" params={{ threadId: selected }}>Abrir hilo</Link>}
          </div>
        </div>
        <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          {KINDS.map((kind) => <div key={kind}><dt className="text-slate-500 dark:text-muted">{TYPE_LABELS[kind]}</dt>
            <dd className="tabular-nums">{compact(sumOf(visible, kind))} <span className="text-xs text-slate-500 dark:text-muted">{total > 0 && percent(sumOf(visible, kind) / total)}</span></dd>
          </div>)}
        </dl>
        {priced > 0 && <p className="mt-3 text-sm tabular-nums">{usd(cost)} a tarifa API · Cobertura {percent(Math.min(1, priced / total))} de los tokens</p>}
      </Card>
      {selected && node && <ThreadUsage thread={selected} range={range} node={node} provider={provider} />}
      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-slate-500 dark:text-muted">
              <tr><th scope="col" className="py-2 pr-4">Categoría</th><th scope="col" className="px-2 text-right">Entrada</th>
                <th scope="col" className="px-2 text-right">Caché leída</th><th scope="col" className="px-2 text-right">Caché escrita</th>
                <th scope="col" className="px-2 text-right">Salida</th><th scope="col" className="pl-2 text-right">% de tokens</th>
                {priced > 0 && <th scope="col" className="px-2 text-right">Coste API</th>}</tr>
            </thead>
            <tbody>
              {summary.map(({ key, group, tokens }) => {
                const labels = new Map<string, number>();
                for (const row of group) {
                  if (row.label && key !== "subagent") {
                    const name = row.label.replace(/^(mcp|skill):/, "");
                    if (name !== key) labels.set(name, (labels.get(name) ?? 0) + totalOf(row));
                  }
                }
                return <tr key={key} className="border-t border-slate-200 align-top dark:border-border">
                  <th scope="row" className="py-2 pr-4 font-medium">
                    {labels.size ? <details><summary className="cursor-pointer whitespace-nowrap">{CATEGORY_LABELS[key]}</summary>
                      <ul className="mt-2 space-y-1 pl-3 text-xs font-normal text-slate-500 dark:text-muted">
                        {[...labels].sort((a, b) => b[1] - a[1]).map(([name, value]) =>
                          <li key={name} className="break-all">{name} · {compact(value)} · {percent(total > 0 ? value / total : 0)}</li>)}
                      </ul>
                    </details> : CATEGORY_LABELS[key]}
                  </th>
                  {KINDS.map((kind) => <td key={kind} className="px-2 py-2 text-right tabular-nums">{compact(sumOf(group, kind))}</td>)}
                  <td className="whitespace-nowrap py-2 pl-2 text-right tabular-nums">{percent(total > 0 ? tokens / total : 0)}</td>
                  {priced > 0 && <td className="whitespace-nowrap px-2 py-2 text-right tabular-nums">
                    {group.some((row) => row.priced_tokens > 0) ? usd(costOfGroup(group)) : "—"}
                    {cost > 0 && costOfGroup(group) > 0 && <span className="ml-1 text-xs text-slate-500 dark:text-muted">{percent(costOfGroup(group) / cost)}</span>}
                  </td>}
                </tr>;
              })}
            </tbody>
          </table>
        </div>
        <p className="mt-3 text-xs text-slate-500 dark:text-muted">El contexto sin atribuir puede incluir historial previo, instrucciones del proveedor y esquemas de herramientas. El coste usa tarifas API de referencia y escrituras de caché de corta duración.</p>
      </Card>
    </div>
  );
}

export const callInput = (call: UsageTurnCall) => call.input + call.cache_read + call.cache_write;
export const exact = new Intl.NumberFormat("es-ES");
export const signed = new Intl.NumberFormat("es-ES", { signDisplay: "exceptZero" });
export const CALL_COLUMNS = ["Llamada", "Modelo", "Entrada total", "Cambio de entrada", "Sin caché", "Caché leída", "Caché escrita", "Salida", "Razonamiento"];

export function ThreadUsage({ thread, range, node, provider }: { thread: string; range: UsageRange; node: string; provider: string }) {
  const { data, error, isPending } = useQuery({
    queryKey: qk.usageTurns(node, thread, range),
    queryFn: () => api.threadUsageTurns(thread, range, node),
  });
  const turns = (data ?? []).filter((turn) => !provider || turn.provider === provider);

  return <Card>
    <h2 className="font-semibold">Uso por turno</h2>
    <p className="mb-2 text-sm text-slate-500 dark:text-muted">Contadores del proveedor. La diferencia compara la entrada total con la llamada anterior del mismo turno. El razonamiento registrado forma parte de la salida.</p>
    <ErrorText error={error} />
    {isPending && <p className="text-sm">Leyendo los turnos…</p>}
    {data && !turns.length && <p className="text-sm">Sin turnos registrados en este periodo.</p>}
    {turns.map((turn) => <details key={turn.id} className="border-t border-slate-200 py-2 dark:border-border">
      <summary className="cursor-pointer text-sm">
        <span className="font-medium">{turn.preview || date(turn.created_at)}</span>
        <span className="ml-2 text-slate-500 dark:text-muted">
          {turn.preview && `${date(turn.created_at)} · `}{PROVIDER_NAMES[turn.provider] ?? turn.provider} · {turn.calls.length} {turn.calls.length === 1 ? "llamada" : "llamadas"} · {exact.format(turn.calls.reduce((sum, call) => sum + callInput(call), 0))} tokens de entrada
        </span>
      </summary>
      <div className="mt-2 overflow-x-auto">
        <table className="w-full whitespace-nowrap text-sm">
          <thead className="text-xs text-slate-500 dark:text-muted">
            <tr>{CALL_COLUMNS.map((label, i) => <th key={label} scope="col" className={`px-2 py-1 ${i < 2 ? "text-left" : "text-right"}`}>{label}</th>)}</tr>
          </thead>
          <tbody>
            {turn.calls.map((call, i) => {
              const previous = turn.calls[i - 1];
              const delta = previous && !call.subagent && !previous.subagent && call.model === previous.model ? callInput(call) - callInput(previous) : null;
              return <tr key={call.ordinal} className="border-t border-slate-200 tabular-nums dark:border-border">
                <th scope="row" className="px-2 py-1 text-left font-medium">{call.ordinal + 1}{call.subagent && " · Subagente"}</th>
                <td className="px-2 py-1">{call.model || "—"}</td>
                <td className="px-2 py-1 text-right">{exact.format(callInput(call))}</td>
                <td className="px-2 py-1 text-right">{delta === null ? "—" : signed.format(delta)}</td>
                {[...KINDS.map((kind) => call[kind]), call.reasoning].map((n, index) => <td key={CALL_COLUMNS[index + 4]} className="px-2 py-1 text-right">{exact.format(n)}</td>)}
              </tr>;
            })}
          </tbody>
        </table>
      </div>
    </details>)}
  </Card>;
}

