import { expectedRemaining, paceTitle } from "../lib/usage-pace";
import { Fragment, useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { ArrowClockwise, PushPin } from "@phosphor-icons/react";
import { Arrow } from "./ui";
import { HoverCard } from "./tooltip";
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

const keyOf = (p: Provider) => `${p.provider}/${p.account_key}`;
const pinKey = "dory-usage-account";
const readPin = () => { try { return localStorage.getItem(pinKey) ?? ""; } catch { return ""; } };

// Dial draws remaining allowance on one split circle: the top 40 % is the 5 h
// window and the bottom 60 % the weekly one. Both are anchored on the left, so
// the top shrinks anticlockwise and the bottom clockwise. Empty tracks mean no data.
function Dial({ short, long }: { short?: number; long?: number }) {
  const r = 9, gap = 26;
  const pt = (deg: number) => { const t = (deg - 90) * Math.PI / 180; return `${(12 + r * Math.cos(t)).toFixed(2)} ${(12 + r * Math.sin(t)).toFixed(2)}`; };
  const path = (from: number, to: number) => `M ${pt(from)} A ${r} ${r} 0 ${to - from > 180 ? 1 : 0} 1 ${pt(to)}`;
  // Degrees clockwise from twelve o'clock.
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

// UsageQuick is a sidebar button that opens the remaining plan limits above it.
export function UsageQuick() {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const usage = useQuery({ queryKey: ["t3-usage"], queryFn: api.t3Usage, refetchInterval: open ? 30_000 : 5 * 60_000 });
  // Each opening asks the instances for fresh limits; the cached ones show meanwhile.
  const qc = useQueryClient();
  const refresh = useMutation({ mutationFn: api.t3UsageRefresh, onSuccess: (data) => qc.setQueryData(["t3-usage"], data) });
  useEffect(() => { if (open) refresh.mutate(); }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  // The dial also asks for fresh limits on load and every five minutes, panel open or not.
  useEffect(() => { const id = setInterval(() => refresh.mutate(), 5 * 60_000); refresh.mutate(); return () => clearInterval(id); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const list = accounts(usage.data ?? []);
  // The icon follows one account: the pinned one, else the first.
  const [pin, setPin] = useState(readPin);
  const shown = list.find((p) => keyOf(p) === pin) ?? list[0];
  const generalWeekly = (w: Provider["windows"][number]) => w.kind === "weekly" && w.label.trim().toLowerCase() === "weekly";
  const byLength = [...(shown?.windows ?? [])].sort((a, b) => (a.window_minutes ?? 0) - (b.window_minutes ?? 0) || Number(generalWeekly(b)) - Number(generalWeekly(a)));
  const inner = byLength[0], outer = byLength.length > 1
    ? byLength.find(generalWeekly)
      ?? byLength.find((w) => w.kind === "weekly") ?? byLength[byLength.length - 1]
    : undefined;
  const pinAccount = (k: string) => { setPin(k); try { localStorage.setItem(pinKey, k); } catch { /* per-viewer only */ } };
  // until formats the time to reset: minutes for the short window, days and hours for the weekly one.
  const until = (iso: string | null, short: boolean) => {
    const ms = iso ? Date.parse(iso) - Date.now() : NaN;
    if (!Number.isFinite(ms)) return "";
    const min = Math.max(0, Math.round(ms / 60_000));
    if (short) return min >= 60 ? `${Math.floor(min / 60)} h ${min % 60} min` : `${min} min`;
    const d = Math.floor(min / 1440), h = Math.floor((min % 1440) / 60);
    return d ? `${d} d ${h} h` : `${h} h ${min % 60} min`;
  };
  const rows = byLength.map((w) => ({ label: w.label, left: `queda ${Math.round(100 - w.used_percent)} %`, reset: until(w.resets_at, (w.window_minutes ?? 0) <= 300) }));
  const lines = rows.map((r) => `${r.label}: ${r.left}${r.reset ? `, se reinicia en ${r.reset}` : ""}`);
  const summary = shown ? `${shown.name}. ${lines.join(". ")}` : "Límites restantes";
  // Hover shows the dial's numbers at once (a native title takes a second and is easy to miss).
  const [hover, setHover] = useState(false);
  const showHover = () => setHover(true);

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
      <button ref={trigger} type="button" aria-haspopup="dialog" aria-expanded={open} aria-label={summary} onMouseEnter={showHover} onMouseLeave={() => setHover(false)} onFocus={showHover} onBlur={() => setHover(false)} onClick={() => { setHover(false); setOpen(!open); }}
        className={`grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 sm:px-3 sm:py-1.5 dark:hover:bg-raised ${open ? "bg-slate-200 dark:bg-raised" : ""}`}>
        <Dial short={inner?.used_percent} long={outer?.used_percent} />
      </button>
      {hover && !open && (
        <HoverCard anchor={trigger.current}>
          {shown ? <><div className="font-medium">{shown.name}</div><div className="grid grid-cols-[auto_auto_auto] gap-x-3 tabular-nums">{rows.map((r) => <Fragment key={r.label}><span>{r.label}</span><span>{r.left}</span><span className="text-right opacity-70">{r.reset && `↻ ${r.reset}`}</span></Fragment>)}</div></> : "Sin datos de límites"}
        </HoverCard>
      )}
      {open && createPortal(
        <div ref={panel} role="dialog" aria-label="Límites restantes" style={pos} className="notif-in fixed z-[100] space-y-3 rounded-lg border border-slate-200 bg-white p-3 shadow-xl dark:border-border dark:bg-surface">
          {usage.isPending && <p className="text-sm text-slate-400">Cargando…</p>}
          {usage.data && list.length === 0 && <p className="text-sm text-slate-400">Sin datos de límites.</p>}
          {list.map((p) => (
            <div key={keyOf(p)} className="space-y-1">
              <div className="flex items-center gap-1 text-sm font-medium">{p.name}
                {list.length > 1 && <button type="button" onClick={() => pinAccount(keyOf(p))} aria-pressed={p === shown} title={p === shown ? "Se muestra en el icono" : "Mostrar en el icono"}
                  className={`ml-auto grid size-6 place-items-center rounded hover:bg-slate-100 dark:hover:bg-raised ${p === shown ? "text-slate-900 dark:text-white" : "text-slate-300 dark:text-muted"}`}><PushPin weight={p === shown ? "fill" : "regular"} className="size-3.5" /></button>}
              </div>
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
