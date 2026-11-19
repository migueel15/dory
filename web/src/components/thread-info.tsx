import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useDiffBase } from "../lib/diff-base";
import { Copy, Folder, GitBranch, GitDiff, Info } from "@phosphor-icons/react";
import { api, type ThreadView, type UsageWindow } from "../api/client";
import { expectedRemaining, WINDOWS } from "../lib/usage-pace";
import { HoverCard } from "./tooltip";
import { EnvIcon } from "./env-icon";
import { copyText } from "./ui";

// diffStat counts added and removed lines of a unified diff.
function diffStat(diff: string) {
  let add = 0, del = 0;
  for (const line of diff.split("\n")) {
    if (line.startsWith("+++") || line.startsWith("---")) continue;
    if (line.startsWith("+")) add++;
    else if (line.startsWith("-")) del++;
  }
  return { add, del };
}

const readOpen = (key: string) => { try { return localStorage.getItem(key) === "1"; } catch { return false; } };

const row = "flex items-center gap-2.5 px-4 py-2 text-sm";

// ThreadInfo is a button that opens a card with where the thread runs, its
// branch, its changes and the remaining usage of its provider.
// It stays open while the chat is used; only its button closes it.
export function ThreadInfo({ t, diff: showingDiff, toggleDiff }: { t: ThreadView; diff: boolean; toggleDiff: () => void }) {
  // Each thread remembers whether its panel was left open, in this browser.
  const key = `dory-thread-info:${t.id}`;
  const [open, setOpenState] = useState(() => readOpen(key));
  useEffect(() => setOpenState(readOpen(key)), [key]);
  const setOpen = (v: boolean) => { setOpenState(v); try { if (v) localStorage.setItem(key, "1"); else localStorage.removeItem(key); } catch { /* per-viewer only */ } };
  const panel = useRef<HTMLDivElement>(null);
  const [hover, setHoverNow] = useState<{ el: HTMLElement; w: UsageWindow } | null>(null);
  // The card waits for the pointer to rest on a row, so passing over does not flash it.
  const wait = useRef<ReturnType<typeof setTimeout>>(undefined);
  const setHover = (h: { el: HTMLElement; w: UsageWindow } | null) => { clearTimeout(wait.current); if (h) wait.current = setTimeout(() => setHoverNow(h), 500); else setHoverNow(null); };
  useEffect(() => () => clearTimeout(wait.current), []);
  const node = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000 });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const projectName = t.project_id ? projects.data?.find((p) => p.id === t.project_id)?.name : undefined;
  const base = useDiffBase(t.id);
  const diff = useQuery({ queryKey: ["thread-diff", t.id, base], queryFn: () => api.threadDiff(t.id, base), enabled: open && t.local, refetchInterval: open ? 5000 : false });
  // The environment the thread runs in, which may be a peer of this node.
  const mesh = useQuery({ queryKey: ["mesh"], queryFn: api.mesh, staleTime: 60_000, enabled: open });
  const self = node.data?.node;
  const env = self && t.node_id === self.id ? self : mesh.data?.peers.find((p) => p.id === t.node_id)?.info.node;
  const where = env?.name ?? mesh.data?.peers.find((p) => p.id === t.node_id)?.name ?? `entorno ${t.node_id.slice(0, 8)}`;
  const stat = diff.data ? diffStat(diff.data.diff) : undefined;
  const usage = node.data?.usage[t.provider] ?? [];
  return (
    <div className="relative">
      <button type="button" onClick={() => setOpen(!open)} title="Información del hilo" aria-label="Información del hilo" aria-expanded={open}
        className={`grid size-8 place-items-center rounded-md text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised ${open ? "bg-slate-100 dark:bg-raised" : ""}`}>
        <Info weight="regular" className="size-4" />
      </button>
      {open && (
        <div ref={panel} role="region" aria-label="Información del hilo" className="notif-in absolute right-0 top-10 z-30 w-80 divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white py-1 shadow-xl dark:divide-border dark:border-border dark:bg-surface">
          <div>
            <div className={row}><span className="shrink-0 text-slate-500"><EnvIcon icon={env?.icon} os={env?.os} name={where} className="size-4" /></span><span className="min-w-0 flex-1 truncate">{where}</span>
              {t.worktree && <button type="button" onClick={() => copyText(t.worktree)} title={`Copiar la carpeta: ${t.worktree}`} aria-label="Copiar la carpeta"
                className="grid size-6 place-items-center rounded text-slate-500 hover:bg-slate-100 dark:hover:bg-raised"><Copy weight="regular" className="size-3.5" /></button>}
            </div>
            {projectName && <div className={row}><Folder weight="regular" className="size-4 shrink-0 text-slate-500" /><span className="min-w-0 flex-1 truncate">{projectName}</span></div>}
          </div>
          <div>
            <div className={row}><GitBranch weight="regular" className="size-4 shrink-0 text-slate-500" /><span className="min-w-0 flex-1 truncate font-medium">{t.branch || "Sin rama"}</span></div>
            {t.local && <button type="button" onClick={toggleDiff} aria-pressed={showingDiff} title={showingDiff ? "Cerrar los cambios" : "Ver los cambios al lado"}
              className={`${row} w-full text-left hover:bg-slate-50 dark:hover:bg-raised ${showingDiff ? "bg-slate-50 dark:bg-raised" : ""}`}><GitDiff weight="regular" className="size-4 shrink-0 text-slate-500" /><span className="font-medium">Cambios</span>
              {diff.data?.base && diff.data.base !== "HEAD" && <span className="min-w-0 truncate text-xs text-slate-500 dark:text-muted">contra {diff.data.base}</span>}
              <span className="flex-1" />
              {stat ? <span className="shrink-0 font-mono tabular-nums"><span className="text-emerald-600 dark:text-emerald-400">+{stat.add.toLocaleString("es-ES")}</span> <span className="text-red-600 dark:text-red-400">-{stat.del.toLocaleString("es-ES")}</span></span> : <span className="text-xs text-slate-400">…</span>}
            </button>}
          </div>
          {usage.length > 0 && <div className="space-y-1 px-2.5 py-2">
            {usage.map((w) => {
              const left = Math.max(0, 100 - Math.round(w.utilization * 100));
              return (
                <div key={w.kind} className={`grid grid-cols-[5.5rem_1fr_2.5rem] items-center gap-3 rounded-md px-1.5 py-1.5 text-xs transition-colors hover:bg-slate-100 dark:hover:bg-raised`}
                  onMouseEnter={(e) => setHover({ el: e.currentTarget, w })} onMouseLeave={() => setHover(null)}>
                  <span className="truncate text-slate-500 dark:text-muted">{WINDOWS[w.kind]?.[0] ?? w.kind}</span>
                  <div className="h-1 rounded bg-slate-200 dark:bg-hover"><div className="h-full rounded bg-slate-700 dark:bg-slate-200" style={{ width: `${left}%` }} /></div>
                  <span className="text-right tabular-nums">{left} %</span>
                </div>
              );
            })}
          </div>}
          {hover && <HoverCard anchor={hover.el} side="left" edge={panel.current} gap={12}><WindowCard w={hover.w} /></HoverCard>}
        </div>
      )}
    </div>
  );
}

const minutesOf = (w: UsageWindow) => WINDOWS[w.kind]?.[1] ?? 0;

// until formats the time to reset: hours and minutes, or days and hours.
function until(iso: string) {
  const ms = Date.parse(iso) - Date.now();
  if (!Number.isFinite(ms)) return "";
  const min = Math.max(0, Math.round(ms / 60_000));
  const d = Math.floor(min / 1440), h = Math.floor((min % 1440) / 60);
  return d ? `${d} d ${h} h` : `${h} h ${min % 60} min`;
}

// WindowCard details one limit on hover: what is left, when it resets and
// whether the spending keeps pace with the time left in the window.
function WindowCard({ w }: { w: UsageWindow }) {
  const left = Math.max(0, 100 - Math.round(w.utilization * 100));
  const pace = expectedRemaining({ resets_at: w.resets_at, window_minutes: minutesOf(w) || null });
  const reset = until(w.resets_at);
  const date = new Date(w.resets_at);
  const at = minutesOf(w) > 0 && minutesOf(w) <= 1440
    ? date.toLocaleTimeString("es-ES", { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleString("es-ES", { weekday: "short", hour: "2-digit", minute: "2-digit" });
  // pace is the share that should be left if usage were spread evenly over
  // the window: the share of its time still to go. Within ±5 points is on pace.
  const status = pace === null ? null
    : left > pace + 5 ? { text: "Buen ritmo", tone: "text-emerald-400" }
    : left < pace - 5 ? { text: "Sobreusando el modelo", tone: "text-red-400" }
    : { text: "Al ritmo", tone: "" };
  return (
    <div className="space-y-0.5 text-left">
      <div className="font-medium">{WINDOWS[w.kind]?.[0] ?? w.kind}: queda {left} %</div>
      {reset && <div className="opacity-70">Se reinicia en {reset} · {at}</div>}
      {status && pace !== null && <div><span className={status.tone}>{status.text}</span><span className="opacity-70"> · debería quedar {Math.round(pace)} %</span></div>}
    </div>
  );
}
