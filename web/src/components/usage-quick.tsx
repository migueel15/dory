import { PaceMark } from "./pace-mark";
import { Fragment, useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { ArrowClockwise, PushPin } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type UsageWindow } from "../api/client";
import { expectedRemaining, paceTitle, windowLabel, windowMinutes, windowReset } from "../lib/usage-pace";
import { ProviderIcon } from "./launch-picker";
import { HoverCard } from "./tooltip";
import { Arrow } from "./ui";
import { qk } from "../api/queries/keys";

const pinKey = "dory-usage-provider";
const readPin = () => { try { return localStorage.getItem(pinKey) ?? ""; } catch { return ""; } };
const time = (iso: string) => new Date(iso).toLocaleString("es-ES", { weekday: "short", hour: "2-digit", minute: "2-digit" });
const usedOf = (w: UsageWindow) => Math.round(w.utilization * 100);

// Dial draws remaining allowance on one split circle: the top 40 % is the
// shortest window and the bottom 60 % the weekly one. Empty tracks mean no data.
function Dial({ short, long }: { short?: number; long?: number }) {
  const r = 9, gap = 26;
  const pt = (deg: number) => { const t = (deg - 90) * Math.PI / 180; return `${(12 + r * Math.cos(t)).toFixed(2)} ${(12 + r * Math.sin(t)).toFixed(2)}`; };
  const path = (from: number, to: number) => `M ${pt(from)} A ${r} ${r} 0 ${to - from > 180 ? 1 : 0} 1 ${pt(to)}`;
  const top = [-72 + gap / 2, 72 - gap / 2], bottom = [72 + gap / 2, 288 - gap / 2];
  const seg = ([from, to]: number[], used: number | undefined, fromStart: boolean) => {
    const left = used === undefined ? 0 : Math.max(0, Math.min(100, 100 - used)) / 100;
    const span = (to - from) * left;
    return (
      <>
        <path d={path(from, to)} fill="none" strokeWidth="3" className="stroke-current opacity-30" />
        {span > 0.5 && <path d={fromStart ? path(from, from + span) : path(to - span, to)} fill="none" strokeWidth="3" className={left < 0.25 ? "stroke-red-600 dark:stroke-red-400" : "stroke-current"} />}
      </>
    );
  };
  return <svg viewBox="0 0 24 24" className="size-5" aria-hidden="true">{seg(top, short, true)}{seg(bottom, long, false)}</svg>;
}

// until formats the time to reset: minutes for short windows, days and hours for long ones.
function until(iso: string | null, short: boolean) {
  if (!iso) return "";
  const ms = Date.parse(iso) - Date.now();
  if (!Number.isFinite(ms)) return "";
  const min = Math.max(0, Math.round(ms / 60_000));
  if (short) return min >= 60 ? `${Math.floor(min / 60)} h ${min % 60} min` : `${min} min`;
  const d = Math.floor(min / 1440), h = Math.floor((min % 1440) / 60);
  return d ? `${d} d ${h} h` : `${h} h ${min % 60} min`;
}

// UsageQuick is a sidebar button with the remaining limits of the pinned
// provider; it opens the limits of every provider of this node.
export function UsageQuick() {
  const [open, setOpen] = useState(false);
  const [hover, setHover] = useState(false);
  const [pos, setPos] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const node = useQuery({ queryKey: qk.node(), queryFn: api.node, refetchInterval: open ? 30_000 : 5 * 60_000 });
  const names = new Map((node.data?.providers ?? []).map((p) => [p.id, p.name]));
  const list = Object.entries(node.data?.usage ?? {}).filter(([, ws]) => ws.length > 0).sort(([a], [b]) => a.localeCompare(b));
  const [pin, setPin] = useState(readPin);
  const shown = list.find(([id]) => id === pin) ?? list[0];
  const byLength = [...(shown?.[1] ?? [])].sort((a, b) => windowMinutes(a) - windowMinutes(b) || Number(b.kind === "seven_day") - Number(a.kind === "seven_day"));
  const inner = byLength[0];
  const outer = byLength.find((w) => w.kind === "seven_day" && w !== inner)
    ?? (inner?.window_minutes ? byLength.find((w) => windowMinutes(w) === 7 * 24 * 60 && w !== inner) : undefined)
    ?? (byLength.length > 1 ? byLength[byLength.length - 1] : undefined);
  const rows = byLength.map((w) => ({ label: windowLabel(w), left: `queda ${100 - usedOf(w)} %`, reset: until(windowReset(w), windowMinutes(w) <= 300) }));
  const summary = shown ? `${names.get(shown[0]) ?? shown[0]}. ${rows.map((r) => `${r.label}: ${r.left}${r.reset ? `, se reinicia en ${r.reset}` : ""}`).join(". ")}` : "Límites restantes";
  const pinProvider = (id: string) => { setPin(id); try { localStorage.setItem(pinKey, id); } catch { /* per-viewer only */ } };

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
      <button ref={trigger} type="button" aria-haspopup="dialog" aria-expanded={open} aria-label={summary}
        onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)} onFocus={() => setHover(true)} onBlur={() => setHover(false)}
        onClick={() => { setHover(false); setOpen(!open); }}
        className={`grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 sm:px-3 sm:py-1.5 dark:hover:bg-raised ${open ? "bg-slate-200 dark:bg-raised" : ""}`}>
        <Dial short={inner && usedOf(inner)} long={outer && usedOf(outer)} />
      </button>
      {hover && !open && (
        <HoverCard anchor={trigger.current}>
          {shown ? <><div className="font-medium">{names.get(shown[0]) ?? shown[0]}</div><div className="grid grid-cols-[auto_auto_auto] gap-x-3 tabular-nums">{rows.map((r) => <Fragment key={r.label}><span>{r.label}</span><span>{r.left}</span><span className="text-right opacity-70">{r.reset && `↻ ${r.reset}`}</span></Fragment>)}</div></> : "Sin datos de límites"}
        </HoverCard>
      )}
      {open && createPortal(
        <div ref={panel} role="dialog" aria-label="Límites restantes" style={pos} className="notif-in fixed z-[100] space-y-3 rounded-lg border border-slate-200 bg-white p-3 shadow-xl dark:border-border dark:bg-surface">
          {node.isPending && <p className="text-sm text-slate-400">Cargando…</p>}
          {node.data && list.length === 0 && <p className="text-sm text-slate-400">Sin datos de límites: aparecen tras el primer turno de un agente.</p>}
          {list.map(([id, windows]) => (
            <div key={id} className="space-y-1">
              <div className="flex items-center gap-1.5 text-sm font-medium"><ProviderIcon driver={id} />{names.get(id) ?? id}
                {list.length > 1 && <button type="button" onClick={() => pinProvider(id)} aria-pressed={id === shown?.[0]} title={id === shown?.[0] ? "Se muestra en el icono" : "Mostrar en el icono"}
                  className={`ml-auto grid size-6 place-items-center rounded hover:bg-slate-100 dark:hover:bg-raised ${id === shown?.[0] ? "text-slate-900 dark:text-white" : "text-slate-300 dark:text-muted"}`}><PushPin weight={id === shown?.[0] ? "fill" : "regular"} className="size-3.5" /></button>}
              </div>
              {windows.map((w) => {
                const used = usedOf(w);
                const reset = windowReset(w);
                const pace = expectedRemaining({ resets_at: reset, window_minutes: windowMinutes(w) || null });
                const bar = used >= 95 ? "bg-red-600" : used >= 80 ? "bg-amber-500" : "bg-emerald-600";
                return (
                  <div key={w.kind} className="grid grid-cols-[minmax(0,6rem)_1fr_2.5rem] items-center gap-2 text-xs" title={reset ? `Se reinicia ${time(reset)}${paceTitle(pace)}` : windowLabel(w)}>
                    <span className="truncate text-slate-600 dark:text-text-secondary">{windowLabel(w)}</span>
                    <div className="relative h-1.5 rounded bg-slate-200 dark:bg-hover"><div className={`h-full rounded ${bar}`} style={{ width: `${Math.max(0, 100 - used)}%` }} />{pace !== null && <PaceMark pace={pace} />}</div>
                    <span className="text-right tabular-nums">{100 - used} %</span>
                  </div>
                );
              })}
            </div>
          ))}
          <div className="flex items-center border-t border-slate-200 pt-2 dark:border-border">
            <Link to="/usage" onClick={() => setOpen(false)} className="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-text">Ver uso completo <Arrow /></Link>
            <button type="button" onClick={() => node.refetch()} disabled={node.isFetching} title="Actualizar" aria-label="Actualizar"
              className="ml-auto grid size-6 place-items-center rounded text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:hover:bg-transparent dark:hover:bg-raised dark:hover:text-white">
              <ArrowClockwise weight="regular" className={`size-3.5 ${node.isFetching ? "animate-spin" : ""}`} />
            </button>
          </div>
        </div>,
        // Inside the phone drawer (a modal dialog in the top layer) the panel must live in it to show above.
        trigger.current?.closest("dialog") ?? document.body,
      )}
    </>
  );
}
