import { expectedRemaining, paceTitle } from "../lib/usage-pace";
import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { ArrowClockwise, Gauge } from "@phosphor-icons/react";
import { Arrow } from "./ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type T3InstanceUsage } from "../api/client";

type Provider = T3InstanceUsage["providers"][number];

const warnAt = 80;
const alertAt = 95;
const time = (iso: string) => new Date(iso).toLocaleString("es-ES", { weekday: "short", hour: "2-digit", minute: "2-digit" });

// accounts keeps one reading per provider account: instances signed in to the same
// account share limits, so the newest check wins. Providers without windows are skipped.
function accounts(usage: T3InstanceUsage[]): Provider[] {
  const best = new Map<string, Provider>();
  for (const u of usage) {
    if (u.limits_error) continue;
    for (const p of u.providers) {
      if (p.windows.length === 0) continue;
      const key = p.account_key ? `${p.provider}/${p.account_key}` : `${u.instance_id}/${p.provider}`;
      const old = best.get(key);
      if (!old || (p.checked_at ?? "") > (old.checked_at ?? "")) best.set(key, p);
    }
  }
  return [...best.values()].sort((a, b) => a.name.localeCompare(b.name));
}

// UsageQuick is a sidebar button that opens the remaining plan limits above it.
export function UsageQuick() {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const usage = useQuery({ queryKey: ["t3-usage"], queryFn: api.t3Usage, refetchInterval: open ? 30_000 : false });
  // Each opening asks the instances for fresh limits; the cached ones show meanwhile.
  const qc = useQueryClient();
  const refresh = useMutation({ mutationFn: api.t3UsageRefresh, onSuccess: (data) => qc.setQueryData(["t3-usage"], data) });
  useEffect(() => { if (open) refresh.mutate(); }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  const list = accounts(usage.data ?? []);
  const worst = Math.max(0, ...list.flatMap((p) => p.windows.map((w) => w.used_percent)));
  const tone = worst >= alertAt ? "text-red-600 dark:text-red-400" : worst >= warnAt ? "text-amber-600 dark:text-amber-400" : "";

  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const t = trigger.current.getBoundingClientRect();
    const width = Math.min(288, window.innerWidth - 16);
    setPos({ width, left: Math.max(8, Math.min(t.left, window.innerWidth - width - 8)), bottom: window.innerHeight - t.top + 6 });
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !trigger.current?.contains(e.target as Node) && !panel.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [open]);

  return (
    <>
      <button ref={trigger} type="button" aria-haspopup="dialog" aria-expanded={open} aria-label="Límites restantes" title="Límites restantes" onClick={() => setOpen(!open)}
        className={`grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 sm:px-3 sm:py-1.5 dark:hover:bg-raised ${open ? "bg-slate-200 dark:bg-raised" : ""} ${tone}`}>
        <Gauge weight="duotone" className="size-5" aria-hidden="true" />
      </button>
      {open && createPortal(
        <div ref={panel} role="dialog" aria-label="Límites restantes" style={pos} className="notif-in fixed z-[100] space-y-3 rounded-lg border border-slate-200 bg-white p-3 shadow-xl dark:border-border dark:bg-surface">
          {usage.isPending && <p className="text-sm text-slate-400">Cargando…</p>}
          {usage.data && list.length === 0 && <p className="text-sm text-slate-400">Sin datos de límites.</p>}
          {list.map((p) => (
            <div key={`${p.provider}/${p.account_key}`} className="space-y-1">
              <div className="text-sm font-medium">{p.name}</div>
              {p.windows.map((w) => {
                const pace = expectedRemaining(w);
                const bar = w.used_percent >= alertAt ? "bg-red-600" : w.used_percent >= warnAt ? "bg-amber-500" : "bg-emerald-600";
                return (
                  <div key={w.label} className="grid grid-cols-[minmax(0,6rem)_1fr_2.5rem] items-center gap-2 text-xs" title={(w.resets_at ? `Se reinicia ${time(w.resets_at)}` : "Sin fecha de reinicio") + paceTitle(pace)}>
                    <span className="truncate text-slate-600 dark:text-text-secondary">{w.label}</span>
                    <div className="h-1.5 relative rounded bg-slate-200 dark:bg-hover"><div className={`h-full rounded ${bar}`} style={{ width: `${Math.max(0, 100 - w.used_percent)}%` }} />{pace !== null && <div aria-hidden="true" className="absolute -top-0.5 -bottom-0.5 w-0.5 -translate-x-1/2 bg-slate-900 dark:bg-white" style={{ left: `${pace}%` }} />}</div>
                    <span className="text-right tabular-nums">{Math.round(100 - w.used_percent)} %</span>
                  </div>
                );
              })}
            </div>
          ))}
          <div className="flex items-center border-t border-slate-200 pt-2 dark:border-border">
            <Link to="/usage" onClick={() => setOpen(false)} className="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-text">Ver uso completo <Arrow /></Link>
            <button type="button" onClick={() => refresh.mutate()} disabled={refresh.isPending} title="Actualizar" aria-label="Actualizar"
              className="ml-auto grid size-6 place-items-center rounded text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:hover:bg-transparent dark:hover:bg-raised dark:hover:text-white">
              <ArrowClockwise weight="regular" className={`size-3.5 ${refresh.isPending ? "animate-spin" : ""}`} />
            </button>
          </div>
        </div>,
        // Inside the phone drawer (a modal dialog in the top layer) the panel must live in it to show above.
        trigger.current?.closest("dialog") ?? document.body,
      )}
    </>
  );
}
