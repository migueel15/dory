import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowsClockwise, DownloadSimple, Warning } from "@phosphor-icons/react";
import { api, type T3InstanceUpdates, type T3UpdateResult } from "../api/client";
import { Arrow, Button, ErrorText, Modal } from "./ui";
import { HoverCard } from "./tooltip";

// Versions shows a change from one version to the next, with the app's arrow icon.
function Versions({ from, to }: { from: string; to: string }) {
  return <span className="inline-flex items-center gap-1.5 font-mono tabular-nums"><span className="opacity-60">{from || "?"}</span><Arrow />{to}</span>;
}

// Instances with something to update: the server, a connector, or both.
export const pending = (u: T3InstanceUpdates) => u.server.available || u.providers.some((p) => p.available);

// A line of an instance's updates: the server or one connector, with its progress.
export type UpdateRow = { k: string; from: string; to: string; busy: boolean; error?: string };
const working = (p: T3InstanceUpdates["providers"][number]) => p.state === "queued" || p.state === "running";
export const rowsOf = (o: T3InstanceUpdates): UpdateRow[] => [
  ...(o.server.available ? [{ k: "T3 Code", from: o.server.current, to: o.server.latest, busy: o.server.state === "updating" }] : []),
  ...o.providers.filter((p) => p.available || working(p)).map((p) => ({ k: p.name, from: p.current, to: p.latest, busy: working(p), error: p.state === "failed" ? p.message : undefined })),
];
const isBusy = (o: T3InstanceUpdates) => !!o.updating || rowsOf(o).some((r) => r.busy);

// RowState says what is happening to a line: updating now, failed, or from → to.
function RowState({ r }: { r: UpdateRow }) {
  if (r.busy) return <span className="inline-flex items-center gap-1.5"><ArrowsClockwise className="size-3.5 animate-spin" aria-hidden="true" />Actualizando…</span>;
  return <Versions from={r.from} to={r.to} />;
}

// useUpdates reads what is outdated and runs the updates. The answer of an
// update lists the instances it left for confirmation (running threads would be
// cut by the restart) and the ones that failed; the dialog below shows them.
export function useUpdates(enabled = true) {
  const qc = useQueryClient();
  const updates = useQuery({ queryKey: ["t3-updates"], queryFn: () => api.t3Updates(), enabled, staleTime: 2_000,
    // Reads what T3 reports as it happens: every 2 s while something updates, otherwise now and then
    // (the monitor's live events refresh it sooner).
    refetchInterval: (q) => (q.state.data?.some(isBusy) ? 2_000 : 15_000) });
  const [results, setResults] = useState<T3UpdateResult[]>([]);
  // The review dialog lists everything that will change before it is confirmed.
  const [reviewing, setReviewing] = useState(false);
  const run = useMutation({
    // One request per instance, so a slow install does not hold the others back.
    mutationFn: async ({ ids, restart }: { ids: string[]; restart: boolean }) => (await Promise.all(ids.map((id) => api.t3Update(id, restart)))).flat(),
    onSuccess: (res) => {
      setResults(res.filter((r) => r.status === "needs_confirmation" || r.status === "failed" || r.status === "missing_scope"));
      qc.invalidateQueries({ queryKey: ["t3-updates"] });
      qc.invalidateQueries({ queryKey: ["t3-status"] });
      qc.invalidateQueries({ queryKey: ["t3-catalog"] });
    },
  });
  const outdated = (updates.data ?? []).filter(pending);
  // Anything updating, whoever started it (Dory, T3 Code or this page a moment ago).
  const working = run.isPending || outdated.some(isBusy);
  return { updates, outdated, working, run, results, close: () => setResults([]), reviewing, review: setReviewing };
}

export type Updates = ReturnType<typeof useUpdates>;

// UpdateDialog explains what an update left undone and asks before restarting
// an instance that has running threads.
export function UpdateDialog({ u, names }: { u: Updates; names: Record<string, string> }) {
  if (u.results.length === 0) return null;
  const name = (id: string) => names[id] ?? "Entorno";
  const confirm = u.results.filter((r) => r.status === "needs_confirmation");
  return (
    <Modal title="Actualización" onClose={u.close}>
      <ul className="space-y-2 text-sm">
        {u.results.map((r) => (
          <li key={r.instance_id} className="flex items-start gap-2"><Warning weight="fill" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-amber-500" />
            <span>
              <span className="font-medium">{name(r.instance_id)}</span>:{" "}
              {r.status === "needs_confirmation" && `tiene ${r.active_threads} ${r.active_threads === 1 ? "hilo en ejecución" : "hilos en ejecución"}; reiniciar T3 los cortaría.`}
              {r.status === "missing_scope" && "el token de Dory no tiene permiso para actualizar (environment:maintain y providers:manage); hay que ampliarlo."}
              {r.status === "failed" && (r.error || "no se pudo actualizar.")}
            </span>
          </li>
        ))}
      </ul>
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="ghost" onClick={u.close}>Cerrar</Button>
        {confirm.length > 0 && <Button disabled={u.run.isPending} onClick={() => { const ids = confirm.map((r) => r.instance_id); u.close(); u.run.mutate({ ids, restart: true }); }}>Reiniciar y actualizar</Button>}
      </div>
    </Modal>
  );
}

// UpdateReview is the confirmation dialog of "update everything": each
// environment with its server and connectors, from version to version.
export function UpdateReview({ u, names }: { u: Updates; names: Record<string, string> }) {
  if (!u.reviewing) return null;
  const close = () => u.review(false);
  const muted = "text-slate-500 dark:text-muted";
  return (
    <Modal title="Actualizar entornos" onClose={close}>
      {u.outdated.length === 0 ? <p className={`text-sm ${muted}`}>Todo está al día.</p> : (
        <div className="space-y-4">
          {u.outdated.map((o) => (
            <section key={o.instance_id} className="space-y-1.5">
              <h3 className="text-sm font-semibold">{names[o.instance_id] ?? "Entorno"}</h3>
              <ul className="divide-y divide-slate-100 rounded-lg border border-slate-200 text-sm dark:divide-border dark:border-border">
                {rowsOf(o).map((r) => <li key={r.k} className="flex items-center justify-between gap-3 px-3 py-1.5"><span>{r.k}{r.k !== "T3 Code" && <span className={`text-xs ${muted}`}> conector</span>}</span><span className="text-xs"><RowState r={r} /></span></li>)}
              </ul>
              {o.server.available && o.active_threads > 0 && <p className="flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-400"><Warning weight="fill" aria-hidden="true" className="mt-px size-3.5 shrink-0" />Tiene {o.active_threads} {o.active_threads === 1 ? "hilo en ejecución" : "hilos en ejecución"}: al reiniciar T3 se cortar{o.active_threads === 1 ? "á" : "án"} si confirmas.</p>}
            </section>
          ))}
        </div>
      )}
      <div className="mt-5 flex justify-end gap-2">
        <Button variant="ghost" onClick={close}>Cancelar</Button>
        <Button disabled={u.outdated.length === 0 || u.run.isPending} onClick={() => { close(); u.run.mutate({ ids: u.outdated.map((o) => o.instance_id), restart: true }); }}>Confirmar y actualizar</Button>
      </div>
    </Modal>
  );
}

// UpdatesButton sits in the sidebar footer: a round download icon, shown only
// when something is outdated, that updates every environment and its connectors.
export function UpdatesButton({ u, names }: { u: Updates; names: Record<string, string> }) {
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [hover, setHover] = useState(false);
  const rows = u.outdated.map((o) => ({ id: o.instance_id, items: rowsOf(o) }));
  return (
    <>
      <button ref={setAnchor} type="button" disabled={u.working} aria-label="Actualizar entornos y conectores"
        onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)} onFocus={() => setHover(true)} onBlur={() => setHover(false)} onClick={() => { setHover(false); u.review(true); }}
        className="grid size-8 shrink-0 place-items-center rounded-md hover:bg-slate-200 disabled:opacity-70 dark:hover:bg-raised">
        {u.working ? <ArrowsClockwise className="size-5 animate-spin" aria-hidden="true" /> : <DownloadSimple weight="regular" className="size-5" aria-hidden="true" />}
      </button>
      {/* What can be updated shows at once on hover: it is information, not a label. */}
      {hover && (
        <HoverCard anchor={anchor}>
          <div className="mb-1 font-medium">{u.working ? "Actualizando…" : "Actualizar todo"}</div>
          <div className="space-y-1">{rows.map((r) => <div key={r.id}><div className="font-medium opacity-90">{names[r.id] ?? "Entorno"}</div>{r.items.map((i) => <div key={i.k} className="flex items-center gap-2 opacity-70"><span>{i.k}</span><span className="text-[11px]"><RowState r={i} /></span></div>)}</div>)}</div>
        </HoverCard>
      )}
    </>
  );
}

// MachineUpdate is the update row of one environment's card: versions and its own button.
export function MachineUpdate({ instanceId, name }: { instanceId: string; name: string }) {
  const u = useUpdates();
  const mine = u.updates.data?.find((x) => x.instance_id === instanceId);
  if (!mine || mine.error) return null;
  const items = rowsOf(mine);
  const updating = isBusy(mine) || u.run.isPending;
  const muted = "text-xs text-slate-500 dark:text-muted";
  return (
    <section className="space-y-1.5">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-muted">Actualizaciones</h3>
      {items.length === 0 ? <p className={muted}>Todo al día.</p> : (
        <div className="flex items-start gap-2">
          <ul className="min-w-0 flex-1 space-y-0.5 text-sm">{items.map((i) => <li key={i.k} className="min-w-0"><div className="truncate"><span className="font-medium">{i.k}</span> <span className="text-xs text-slate-500 dark:text-muted"><RowState r={i} /></span></div>{i.error && !i.busy && <div className="text-xs text-red-600 dark:text-red-400">{i.error}</div>}</li>)}</ul>
          <Button disabled={updating} onClick={() => u.run.mutate({ ids: [instanceId], restart: false })} className="shrink-0 !px-2.5 !py-1 text-xs">{updating ? "Actualizando…" : "Actualizar"}</Button>
        </div>
      )}
      <ErrorText error={u.run.error} />
      <UpdateDialog u={u} names={{ [instanceId]: name }} />
    </section>
  );
}
