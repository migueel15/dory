import { useState } from "react";
import { ArrowCircleUp, ArrowsClockwise } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type NodeUpdate } from "../api/client";
import { askConfirm } from "./confirm";
import { HoverCard } from "./tooltip";

// What a node has to update: its core and the harnesses with a newer release.
function pendingOf(n: NodeUpdate) {
  const st = n.status;
  if (!st) return { core: false, providers: [] as string[] };
  return { core: st.available, providers: st.providers.filter((p) => p.available).map((p) => `${p.name} ${p.current} → ${p.latest}`) };
}

// useMeshUpdates reads the update state of every connected node; it polls
// faster while one of them is updating.
export function useMeshUpdates(enabled = true) {
  return useQuery({
    queryKey: ["mesh-update"], queryFn: () => api.meshUpdates(), enabled,
    refetchInterval: (q) => (q.state.data?.some((n) => n.status?.state === "updating") ? 3_000 : 5 * 60_000),
  });
}

// MeshUpdateButton sits right of the commit in the sidebar. It shows only when
// a connected node, or one of its harnesses, has a newer release (ADR-016).
export function MeshUpdateButton() {
  const qc = useQueryClient();
  const list = useMeshUpdates();
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [hover, setHover] = useState(false);
  const nodes = list.data ?? [];
  const rows = nodes.map((n) => ({ n, p: pendingOf(n) })).filter((r) => r.p.core || r.p.providers.length);
  const working = nodes.some((n) => n.status?.state === "updating");
  const providers = rows.some((r) => r.p.providers.length > 0);
  const run = useMutation({
    mutationFn: () => api.updateMesh({ providers, force: false }),
    onSuccess: (data) => qc.setQueryData(["mesh-update"], data),
  });
  if (!rows.length && !working) return null;
  const busy = working || run.isPending;
  const start = async () => {
    setHover(false);
    const turns = nodes.reduce((a, n) => a + (n.status?.active_turns ?? 0), 0);
    const note = turns ? ` Los nodos con turnos en marcha (${turns}) se saltarán.` : "";
    if (await askConfirm(`¿Actualizar ${rows.length === 1 ? "1 nodo" : `${rows.length} nodos`}? Cada nodo se reinicia al terminar.${note}`, "Actualizar")) run.mutate();
  };
  return (
    <>
      <button ref={setAnchor} type="button" disabled={busy} aria-label="Actualizar la malla"
        onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)} onFocus={() => setHover(true)} onBlur={() => setHover(false)} onClick={start}
        className="relative grid size-8 shrink-0 place-items-center rounded-md hover:bg-slate-200 disabled:opacity-70 dark:hover:bg-raised">
        {busy ? <ArrowsClockwise className="size-5 animate-spin" aria-hidden="true" /> : <>
          <ArrowCircleUp weight="regular" className="size-5" aria-hidden="true" />
          <span className="absolute right-1 top-1 size-2 rounded-full bg-blue-500 ring-2 ring-slate-100 dark:ring-surface" aria-hidden="true" />
        </>}
      </button>
      {hover && (
        <HoverCard anchor={anchor}>
          <div className="mb-1 font-medium">{busy ? "Actualizando…" : "Actualizar la malla"}</div>
          <div className="space-y-1">
            {rows.map(({ n, p }) => (
              <div key={n.node}>
                <div className="font-medium opacity-90">{n.name}</div>
                {p.core && <div className="opacity-70">Dory {n.status?.current} → {n.status?.latest}</div>}
                {p.providers.map((t) => <div key={t} className="opacity-70">{t}</div>)}
              </div>
            ))}
          </div>
          {run.error && <div className="mt-1 text-red-500">{(run.error as Error).message}</div>}
        </HoverCard>
      )}
    </>
  );
}
