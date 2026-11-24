import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowCircleUp, ArrowsClockwise, DeviceMobile, DotsThree, Plus, WifiHigh } from "@phosphor-icons/react";
import { api, type NodeUpdate, type Provider } from "../../api/client";
import { EnvIconPicker } from "../../components/env-icon";
import { ProviderIcon } from "../../components/launch-picker";
import { useMeshUpdates } from "../../components/mesh-update";
import { askConfirm } from "../../components/confirm";
import { Button, ErrorText, Input } from "../../components/ui";
import type { SectionProps } from "./settings-node";
import { qk } from "../../api/queries/keys";
import { useMesh, useNode, usePairedDevices, useNodeProviders } from "../../api/queries/hooks";

type Row = { id: string; name: string; icon?: string; os?: string; version?: string; local: boolean; state: "on" | "off" | "disabled"; seen?: string; incompatible?: boolean };

// ConnectionsSettings is the "Conexiones" section: the nodes of the mesh with
// what matters about each one (state, version, updates, providers), and how to
// add a new one. It is global, so it ignores the node selector.
export function ConnectionsSettings(_: SectionProps) {
  const qc = useQueryClient();
  const node = useNode({ refetchInterval: 15_000 });
  const mesh = useMesh({ refetchInterval: 5_000 });
  const updates = useMeshUpdates();
  const recheck = useMutation({ mutationFn: () => api.meshUpdates(true), onSuccess: (d) => qc.setQueryData(["mesh-update"], d) });
  const all = useMutation({ mutationFn: () => api.updateMesh({ providers: true, force: false }), onSuccess: (d) => qc.setQueryData(["mesh-update"], d) });
  const rows: Row[] = [];
  if (node.data) rows.push({ id: node.data.node.id, name: node.data.node.name, icon: node.data.node.icon, os: node.data.node.os, version: node.data.node.version, local: true, state: "on" });
  for (const p of mesh.data?.peers ?? []) if (!p.revoked) rows.push({ id: p.id, name: p.name, icon: p.info.node?.icon, os: p.info.node?.os, version: p.info.node?.version, local: false, state: p.disabled ? "disabled" : p.connected ? "on" : "off", seen: p.last_seen, incompatible: p.incompatible });
  const byNode = new Map((updates.data ?? []).map((u) => [u.node, u]));
  const pending = (updates.data ?? []).filter((u) => u.status?.available || u.status?.providers.some((p) => p.available)).length;
  return (
    <section aria-labelledby="connections-title" className="space-y-6">
      <div className="flex items-center gap-2">
        <h2 id="connections-title" className="text-lg font-semibold">Conexiones</h2>
        <button type="button" title="Buscar actualizaciones" aria-label="Buscar actualizaciones" disabled={recheck.isPending} onClick={() => recheck.mutate()}
          className="ml-auto grid size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
          <ArrowsClockwise className={`size-4 ${recheck.isPending ? "animate-spin" : ""}`} aria-hidden="true" />
        </button>
        {pending > 0 && (
          <Button disabled={all.isPending} onClick={async () => (await askConfirm(`¿Actualizar ${pending === 1 ? "1 entorno" : `${pending} entornos`} y sus proveedores? Cada uno se reinicia al terminar.`, "Actualizar")) && all.mutate()}>
            <ArrowCircleUp className="mr-1.5 size-4" aria-hidden="true" />Actualizar todo ({pending})
          </Button>
        )}
      </div>
      <ErrorText error={node.error ?? mesh.error ?? all.error ?? recheck.error} />
      <div className="space-y-2">
        {rows.map((r) => <NodeCard key={r.id} row={r} update={byNode.get(r.id)} />)}
      </div>
      <AddConnection discovered={mesh.data?.discovered ?? []} />
      <Phones />
    </section>
  );
}

const STATE = {
  on: { dot: "bg-emerald-500", text: "Conectado" },
  off: { dot: "bg-slate-400", text: "Desconectado" },
  disabled: { dot: "bg-amber-500", text: "Desactivado" },
};

// NodeCard is one environment: name and state, what can be updated with one
// click, and its providers.
function NodeCard({ row: r, update }: { row: Row; update?: NodeUpdate }) {
  const qc = useQueryClient();
  const [menu, setMenu] = useState(false);
  const providers = useNodeProviders(r.local ? "local" : r.id, { enabled: r.state === "on", staleTime: 60_000 });
  const st = update?.status;
  const core = !!st?.available;
  const provUpdates = st?.providers.filter((p) => p.available) ?? [];
  const run = useMutation({
    mutationFn: (force: boolean) => api.updateNode(r.local ? "local" : r.id, { providers: provUpdates.length > 0, force }),
    onSettled: () => qc.invalidateQueries({ queryKey: qk.meshUpdate() }),
  });
  const toggle = useMutation({ mutationFn: () => api.setPeerEnabled(r.id, r.state === "disabled"), onSuccess: () => qc.invalidateQueries({ queryKey: qk.mesh() }) });
  const remove = useMutation({ mutationFn: () => api.revokePeer(r.id), onSuccess: () => qc.invalidateQueries({ queryKey: qk.mesh() }) });
  const failure = (run.error as Error | null)?.message || update?.error;
  const updating = st?.state === "updating" || run.isPending;
  const s = STATE[r.state];
  return (
    <div className="rounded-xl border border-slate-200 p-4 dark:border-border">
      <div className="flex flex-wrap items-center gap-3">
        <EnvIconPicker node={r.local ? "local" : r.id} icon={r.icon} os={r.os} name={r.name} />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{r.name}</span>
            {r.local && <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[11px] text-slate-600 dark:bg-raised dark:text-muted">Este equipo</span>}
          </div>
          <div className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted">
            <span className={`size-2 rounded-full ${s.dot}`} aria-hidden="true" />{s.text}
            {r.state === "off" && r.seen && <span>· visto {new Date(r.seen).toLocaleDateString()}</span>}
            {r.version && <span>· Dory {r.version}</span>}
          </div>
        </div>
        <div className="ml-auto flex items-center gap-1">
          {updating ? (
            <span className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted"><ArrowsClockwise className="size-4 animate-spin" aria-hidden="true" />Actualizando…</span>
          ) : (core || provUpdates.length > 0) ? (
            <Button onClick={() => run.mutate(false)}><ArrowCircleUp className="mr-1.5 size-4" aria-hidden="true" />Actualizar</Button>
          ) : st && st.channel !== "dev" ? <span className="text-xs text-emerald-600 dark:text-emerald-400">Al día</span> : null}
          {!r.local && (
            <div className="relative">
              <button type="button" aria-label="Más acciones" onClick={() => setMenu(!menu)} onBlur={() => setTimeout(() => setMenu(false), 150)}
                className="grid size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
                <DotsThree weight="bold" className="size-5" aria-hidden="true" />
              </button>
              {menu && (
                <div className="absolute right-0 z-10 mt-1 w-40 rounded-lg border border-slate-200 bg-white p-1 text-sm shadow-lg dark:border-border dark:bg-raised">
                  <button type="button" className="w-full rounded px-2 py-1.5 text-left hover:bg-slate-100 dark:hover:bg-hover" onMouseDown={() => toggle.mutate()}>{r.state === "disabled" ? "Activar" : "Desactivar"}</button>
                  <button type="button" className="w-full rounded px-2 py-1.5 text-left text-red-600 hover:bg-slate-100 dark:text-red-400 dark:hover:bg-hover"
                    onMouseDown={() => askConfirm(`¿Eliminar ${r.name}? Habrá que emparejarlo de nuevo.`, "Eliminar").then((ok) => ok && remove.mutate())}>Eliminar</button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
      {!updating && (core || provUpdates.length > 0) && (
        <div className="mt-3 flex flex-wrap gap-1.5 text-xs">
          {core && <span className="rounded-full bg-blue-50 px-2 py-0.5 text-blue-700 dark:bg-accent-subtle dark:text-accent-text">Dory {st?.current} → {st?.latest}</span>}
          {provUpdates.map((p) => <span key={p.id} className="rounded-full bg-blue-50 px-2 py-0.5 text-blue-700 dark:bg-accent-subtle dark:text-accent-text">{p.name} {p.current} → {p.latest}</span>)}
        </div>
      )}
      {r.incompatible && <p className="mt-2 text-xs text-amber-700 dark:text-amber-400">Versión de malla incompatible. Actualiza Dory en {r.name}.</p>}
      {st?.state === "failed" && <p className="mt-2 text-xs text-red-600 dark:text-red-400">La actualización falló: {st.error}</p>}
      {failure && !updating && (
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-red-600 dark:text-red-400">
          <span>No se pudo actualizar: {failure}</span>
          {r.state === "on" && (
            <button type="button" className="font-medium underline"
              onClick={async () => (await askConfirm(`¿Actualizar ${r.name} igualmente? Se cortarán los turnos en marcha.`, "Actualizar")) && run.mutate(true)}>Actualizar igualmente</button>
          )}
        </div>
      )}
      {(st?.active_turns ?? 0) > 0 && (core || provUpdates.length > 0) && <p className="mt-2 text-xs text-slate-500 dark:text-muted">Tiene {st!.active_turns} turnos en marcha: se actualizará cuando terminen.</p>}
      <ErrorText error={toggle.error ?? remove.error} />
      {!!providers.data?.length && <ProviderChips list={providers.data} />}
    </div>
  );
}

function ProviderChips({ list }: { list: Provider[] }) {
  const shown = list.filter((p) => p.installed && p.enabled !== false);
  if (!shown.length) return null;
  return (
    <div className="mt-3 flex flex-wrap gap-1.5 border-t border-slate-100 pt-3 dark:border-border">
      {shown.map((p) => (
        <span key={p.id} className="flex items-center gap-1.5 rounded-md bg-slate-50 px-2 py-1 text-xs dark:bg-raised" title={p.logged_in === false ? "Sin sesión" : undefined}>
          <ProviderIcon driver={p.driver ?? p.id} />{p.name}
          {p.logged_in === false && <span className="size-1.5 rounded-full bg-amber-500" aria-label="Sin sesión" />}
        </span>
      ))}
    </div>
  );
}

// AddConnection pairs a new node: one found on the network, by showing this
// node's code, or by address.
function AddConnection({ discovered }: { discovered: { id: string; name: string; address: string }[] }) {
  const qc = useQueryClient();
  const [target, setTarget] = useState<{ address: string; name?: string } | null>(null);
  const [manual, setManual] = useState(false);
  const [typed, setTyped] = useState("");
  const codeInput = useRef<HTMLInputElement>(null);
  const [code, setCode] = useState<{ code: string; expires_at: string }>();
  const show = useMutation({ mutationFn: api.pairCode, onSuccess: setCode });
  const pair = useMutation({
    mutationFn: () => api.pair(target!.address.trim(), typed.trim()),
    onSuccess: () => { setTarget(null); setTyped(""); setManual(false); qc.invalidateQueries({ queryKey: qk.mesh() }); },
  });
  const pick = (address: string, name?: string) => { setTarget({ address, name }); setTimeout(() => codeInput.current?.focus(), 0); };
  return (
    <div className="space-y-3">
      <h3 className="flex items-center gap-2 font-semibold"><Plus className="size-4" aria-hidden="true" />Añadir conexión</h3>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="rounded-xl border border-slate-200 p-4 dark:border-border">
          <p className="mb-2 flex items-center gap-1.5 text-sm font-medium"><WifiHigh className="size-4" aria-hidden="true" />En tu red</p>
          {discovered.length ? discovered.map((d) => (
            <button key={d.id} type="button" onClick={() => pick(d.address, d.name)}
              className={`flex w-full items-center justify-between rounded-md px-2 py-2 text-left text-sm hover:bg-slate-50 dark:hover:bg-raised ${target?.address === d.address ? "bg-slate-50 dark:bg-raised" : ""}`}>
              {d.name}<span className="text-xs text-accent dark:text-accent-text">Conectar</span>
            </button>
          )) : <p className="text-sm text-slate-500 dark:text-muted">Buscando equipos con Dory…</p>}
          {!manual && !target && <button type="button" className="mt-2 text-xs text-slate-500 underline-offset-2 hover:underline dark:text-muted" onClick={() => { setManual(true); setTarget({ address: "" }); }}>Conectar por dirección</button>}
        </div>
        <div className="rounded-xl border border-slate-200 p-4 dark:border-border">
          <p className="mb-2 text-sm font-medium">Conectar otro equipo o un móvil a este</p>
          {code ? (
            <div>
              <p className="font-mono text-3xl tracking-[0.2em]">{code.code}</p>
              <p className="mt-1 text-xs text-slate-500 dark:text-muted">Escríbelo en el otro equipo. Caduca a las {new Date(code.expires_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}.</p>
            </div>
          ) : <Button variant="ghost" onClick={() => show.mutate()} disabled={show.isPending}>Mostrar código</Button>}
          <ErrorText error={show.error} />
        </div>
      </div>
      {target && (
        <form className="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 p-4 dark:border-border"
          onSubmit={(e) => { e.preventDefault(); if (target.address.trim() && typed.trim()) pair.mutate(); }}>
          <span className="w-full text-sm">{target.name ? <>Conectar con <strong>{target.name}</strong>: escribe el código que muestra ese equipo.</> : "Dirección del equipo y el código que muestra."}</span>
          {!target.name && <Input className="min-w-48 flex-1" value={target.address} onChange={(e) => setTarget({ address: e.target.value })} placeholder="servidor:7710" />}
          <Input ref={codeInput} className="w-40 font-mono" value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="1234-5678" />
          <Button disabled={!target.address.trim() || !typed.trim() || pair.isPending}>{pair.isPending ? "Conectando…" : "Conectar"}</Button>
          <Button type="button" variant="ghost" onClick={() => { setTarget(null); setManual(false); setTyped(""); }}>Cancelar</Button>
          <div className="w-full"><ErrorText error={pair.error} /></div>
        </form>
      )}
    </div>
  );
}

// Phones lists the paired mobile apps.
function Phones() {
  const qc = useQueryClient();
  const devices = usePairedDevices({ refetchInterval: 15_000 });
  const revoke = useMutation({ mutationFn: api.revokeDevice, onSuccess: () => qc.invalidateQueries({ queryKey: qk.devices() }) });
  if (!devices.data?.length) return null;
  return (
    <div className="space-y-2">
      <h3 className="font-semibold">Móviles</h3>
      {devices.data.map((d) => (
        <div key={d.id} className="flex items-center gap-2 rounded-xl border border-slate-200 px-4 py-3 text-sm dark:border-border">
          <DeviceMobile className="size-4" aria-hidden="true" />{d.name}
          <Button variant="ghost" className="ml-auto text-xs" onClick={() => askConfirm(`¿Desconectar ${d.name}? Tendrá que emparejarse de nuevo.`, "Desconectar").then((ok) => ok && revoke.mutate(d.id))}>Desconectar</Button>
        </div>
      ))}
      <ErrorText error={revoke.error} />
    </div>
  );
}
