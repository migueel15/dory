import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useDiffBase } from "../lib/diff-base";
import { Copy, Database, Folder, GitBranch, GitDiff, Globe, Info } from "@phosphor-icons/react";
import { api, type ThreadService, type ThreadView, type UsageWindow } from "../api/client";
import { expectedRemaining, windowLabel, windowMinutes, windowReset } from "../lib/usage-pace";
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

// headerButton is the look of the thread header buttons that open a card: details and task.
export const headerButton = (active: boolean) => `grid size-8 place-items-center rounded-md text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised ${active ? "bg-slate-100 dark:bg-raised" : ""}`;

// ThreadInfo is a button that opens a card with where the thread runs, its
// branch, its changes and the remaining usage of its provider.
// It stays open while the chat is used; only its button closes it. While the
// side panel is open (popup) it hides and works as a pop-up that closes on any
// click or key outside it, without touching the remembered state, which comes
// back when the side panel closes.
// Only one card shows at a time: hidden is true while the task card is open,
// and onOpen tells the header that this card is opening instead.
export function ThreadInfo({ t, diff: showingDiff, toggleDiff, popup = false, hidden = false, onOpen }: { t: ThreadView; diff: boolean; toggleDiff: () => void; popup?: boolean; hidden?: boolean; onOpen?: () => void }) {
  // Each thread remembers whether its panel was left open, in this browser.
  const key = `dory-thread-info:${t.id}`;
  const [open, setOpenState] = useState(() => readOpen(key));
  useEffect(() => setOpenState(readOpen(key)), [key]);
  const [pop, setPop] = useState(false);
  useEffect(() => setPop(false), [popup, key]);
  const wrap = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!popup || !pop) return;
    const close = (e: Event) => { if (!wrap.current?.contains(e.target as Node)) setPop(false); };
    document.addEventListener("pointerdown", close, true);
    document.addEventListener("keydown", close, true);
    return () => { document.removeEventListener("pointerdown", close, true); document.removeEventListener("keydown", close, true); };
  }, [popup, pop]);
  const shown = !hidden && (popup ? pop : open);
  const qc = useQueryClient();
  const services = useQuery({ queryKey: ["thread-services", t.id], queryFn: () => api.threadServices(t.id), enabled: shown, refetchInterval: 3000 });
  const [serviceRows, setServiceRows] = useState<{ service: ThreadService; leaving: boolean }[]>([]);
  useEffect(() => {
    const next = services.data ?? [];
    const key = (s: ThreadService) => `${s.pid}:${s.address}:${s.port}`;
    const present = new Set(next.map(key));
    setServiceRows((rows) => [...next.map((service) => ({ service, leaving: false })), ...rows.filter((row) => !present.has(key(row.service))).map((row) => ({ ...row, leaving: true }))]);
    const timer = setTimeout(() => setServiceRows((rows) => rows.filter((row) => !row.leaving)), 180);
    return () => clearTimeout(timer);
  }, [services.data]);
  const stopService = useMutation({ mutationFn: (pid: number) => api.stopThreadService(t.id, pid), onSettled: () => qc.invalidateQueries({ queryKey: ["thread-services", t.id] }) });
  const toggle = () => {
    if (hidden) { onOpen?.(); return popup ? setPop(true) : setOpen(true); }
    return popup ? setPop(!pop) : setOpen(!open);
  };
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
  const diff = useQuery({ queryKey: ["thread-diff", t.id, base], queryFn: () => api.threadDiff(t.id, base), enabled: shown && t.local, refetchInterval: shown ? 5000 : false });
  // The environment the thread runs in, which may be a peer of this node.
  const mesh = useQuery({ queryKey: ["mesh"], queryFn: api.mesh, staleTime: 60_000, enabled: shown });
  const self = node.data?.node;
  const env = self && t.node_id === self.id ? self : mesh.data?.peers.find((p) => p.id === t.node_id)?.info.node;
  const where = env?.name ?? mesh.data?.peers.find((p) => p.id === t.node_id)?.name ?? `entorno ${t.node_id.slice(0, 8)}`;
  const stat = diff.data ? diffStat(diff.data.diff) : undefined;
  const usage = node.data?.usage[t.provider] ?? [];
  // Not positioned: the panel anchors to the thread header, so it opens at its far right edge.
  return (
    <div ref={wrap}>
      <button type="button" onClick={toggle} title="Información del hilo" aria-label="Información del hilo" aria-expanded={shown}
        className={headerButton(shown)}>
        <Info weight="regular" className="size-4" />
      </button>
      {shown && (
        <div ref={panel} role="region" aria-label="Información del hilo" className="notif-in absolute right-1 top-11 z-30 w-80 divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white py-1 shadow-xl dark:divide-border dark:border-border dark:bg-surface">
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
          {!!serviceRows.length && <div className="px-2.5 py-2">
            <div className="px-1.5 pb-1 text-xs text-slate-500">Servidores</div>
            {serviceRows.map(({ service: s, leaving }) => {
              const names: Record<string, string> = { postgresql: "postgres", redis: "redis", mysql: "mysql", mongodb: "mongodb", vite: "vite", next: "next", docker: "docker" };
              const local = ["127.0.0.1", "::1", "0.0.0.0", "::"].includes(s.address) || s.address.startsWith("127.");
              const address = `${local ? "localhost" : s.address.includes(":") ? `[${s.address}]` : s.address}:${s.port}`;
              const name = names[s.kind] ?? s.command.split(" ")[0]?.split("/").pop()?.replace(/\d+(\.\d+)*$/, "") ?? "proceso";
              const Icon = ["postgresql", "redis", "mysql", "mongodb"].includes(s.kind) ? Database : Globe;
              const link = local && self?.id === t.node_id && !["postgresql", "redis", "mysql", "mongodb"].includes(s.kind);
              return <div key={`${s.pid}:${s.address}:${s.port}`} className={`${leaving ? "service-out" : "notif-in"} group flex items-center gap-2.5 rounded-lg px-1.5 py-1.5 hover:bg-slate-100 dark:hover:bg-raised`}>
                <Icon className="size-4 shrink-0 text-slate-500" />
                <div className="min-w-0 flex-1" title={s.command}>
                  {link ? <a href={`http://${address}`} target="_blank" rel="noreferrer" className="block truncate text-sm font-semibold">{address}</a> : <div className="truncate text-sm font-semibold">{address}</div>}
                  <div className="truncate text-xs text-slate-500">Agente · {name}</div>
                </div>
                <button type="button" aria-label={`Parar ${address}`} title="Parar" onClick={() => stopService.mutate(s.pid)} disabled={stopService.isPending} className="grid size-6 shrink-0 place-items-center rounded [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100 focus:opacity-100"><span className="size-2.5 rounded-[2px] bg-red-500" /></button>
              </div>;
            })}
            {stopService.error && <p className="px-1.5 text-xs text-red-600">{stopService.error.message}</p>}
          </div>}
          {usage.length > 0 && <div className="space-y-1 px-2.5 py-2">
            {usage.map((w) => {
              const left = Math.max(0, 100 - Math.round(w.utilization * 100));
              return (
                <div key={w.kind} className={`grid grid-cols-[5.5rem_1fr_2.5rem] items-center gap-3 rounded-md px-1.5 py-1.5 text-xs transition-colors hover:bg-slate-100 dark:hover:bg-raised`}
                  onMouseEnter={(e) => setHover({ el: e.currentTarget, w })} onMouseLeave={() => setHover(null)}>
                  <span className="truncate text-slate-500 dark:text-muted">{windowLabel(w)}</span>
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

// until formats the time to reset: hours and minutes, or days and hours.
function until(iso: string | null) {
  if (!iso) return "";
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
  const pace = expectedRemaining({ resets_at: windowReset(w), window_minutes: windowMinutes(w) || null });
  const reset = until(windowReset(w));
  const date = new Date(w.resets_at);
  const at = windowMinutes(w) > 0 && windowMinutes(w) <= 1440
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
      <div className="font-medium">{windowLabel(w)}: queda {left} %</div>
      {reset && <div className="opacity-70">Se reinicia en {reset} · {at}</div>}
      {status && pace !== null && <div><span className={status.tone}>{status.text}</span><span className="opacity-70"> · debería quedar {Math.round(pace)} %</span></div>}
    </div>
  );
}
