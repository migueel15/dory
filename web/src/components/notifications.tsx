import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent as RPointerEvent } from "react";
import { ArrowLeft, Bell, Check, GearSix, Trash, X } from "@phosphor-icons/react";
import { createPortal } from "react-dom";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "./ui";

const iconBtn = "grid size-8 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-40 dark:text-muted dark:hover:bg-raised dark:hover:text-white";
const channelLabel: Record<string, string> = { enabled: "Activo", bell: "Lista", toast: "Aviso", push: "Push" };
import { ItemForm } from "../pages/items";
import { api } from "../api/client";

type Notice = { id: number; title: string; body: string; url: string; bell: boolean; toast: boolean; read: boolean; created_at: string };
// external opens links outside Dory (T3 Code threads) in a new window, so the app keeps its state.
// href opens a thread notice in Dory's thread view; older notices still carry
// the T3 Code link (…/threads/<id>), so it is rewritten.
const href = (url: string) => {
  try {
    const u = new URL(url, location.href);
    const id = u.origin !== location.origin && /\/threads\//.test(u.pathname) ? u.pathname.replace(/\/+$/, "").split("/").pop() : "";
    return id ? `/threads/${encodeURIComponent(id)}` : url;
  } catch { return url; }
};

const external = (url: string) => {
  try { return new URL(url, location.href).origin !== location.origin ? { target: "_blank", rel: "noreferrer" } : {}; } catch { return {}; }
};
type Channel = "enabled" | "bell" | "toast" | "push";
type Settings = Record<string, Partial<Record<Channel, boolean>>>;
const events = {
  thread_finished: "Hilo finalizado", thread_attention: "Hilo requiere atención",
  item_inbox: "Item en bandeja", item_triaged: "Item clasificado", item_ready: "Item listo", item_in_progress: "Item en curso",
  item_validating: "Item en validación", item_blocked: "Item bloqueado", item_done: "Item hecho", item_cancelled: "Item cancelado",
};
// Only these events start enabled; the other item statuses are opt-in (same rule as emit_notification).
const onByDefault = new Set(["item_ready", "item_blocked", "thread_finished", "thread_attention"]);
function ago(iso: string) {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return "ahora";
  if (s < 3600) return `hace ${Math.floor(s / 60)} min`;
  if (s < 86400) return `hace ${Math.floor(s / 3600)} h`;
  return `hace ${Math.floor(s / 86400)} d`;
}
async function request<T>(path: string, method = "GET", body?: unknown): Promise<T> {
  const res = await fetch("/api/notifications" + path, { method, headers: { "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
  if (!res.ok) throw new Error("No se pudo guardar o recuperar las notificaciones");
  return res.status === 204 ? undefined as T : res.json();
}
export function Notifications() {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [subscribed, setSubscribed] = useState(false);
  const [toasts, setToasts] = useState<Notice[]>([]);
  const [expanded, setExpanded] = useState(false);
  const since = useRef<string | null>(null);
  const seen = useRef(new Set<number>());
  const feed = useQuery({ queryKey: ["notifications", "feed"], queryFn: () => request<{ items: Notice[]; unread: number; server_time: string }>(""), staleTime: 0, refetchInterval: 60_000, refetchOnWindowFocus: true });
  // The live stream refreshes the feed when items or T3 threads change (see api/live.ts); polling is a fallback.
  const history = useInfiniteQuery({
    queryKey: ["notifications", "history"], initialPageParam: 0,
    queryFn: ({ pageParam }) => request<{ items: Notice[] }>("?before=" + pageParam),
    getNextPageParam: page => page.items.length === 100 ? page.items[page.items.length - 1].id : undefined,
    enabled: open, staleTime: 0, refetchInterval: open ? 30_000 : false,
  });
  const notices = history.data?.pages.flatMap(p => p.items) ?? [];
  const prefs = useQuery({ queryKey: ["notification-preferences"], queryFn: () => request<{ settings: Settings; public_key: string }>("/preferences"), staleTime: 5 * 60_000 });
  const supported = "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
  useEffect(() => {
    if (supported) navigator.serviceWorker.ready.then(r => r.pushManager.getSubscription()).then(s => setSubscribed(!!s)).catch(() => {});
    const reset = () => { since.current = null; setToasts([]); };
    window.addEventListener("online", reset);
    document.addEventListener("visibilitychange", reset);
    return () => { window.removeEventListener("online", reset); document.removeEventListener("visibilitychange", reset); };
  }, [supported]);
  useEffect(() => {
    if (!feed.data) return;
    const data = feed.data;
    if (since.current === null) {
      since.current = data.server_time;
      data.items.forEach(n => seen.current.add(n.id));
      return;
    }
    const fresh = data.items.filter(n => !seen.current.has(n.id) && n.toast && !n.read && n.created_at > since.current!);
    data.items.forEach(n => seen.current.add(n.id));
    if (document.visibilityState === "visible" && fresh.length) setToasts(old => [...fresh, ...old].slice(0, 4));
  }, [feed.data]);
  // Auto-dismiss pauses while the stack is open; a stack of one cannot stay expanded.
  useEffect(() => { if (!toasts.length || expanded) return; const timer = window.setTimeout(() => setToasts([]), 8000); return () => clearTimeout(timer); }, [toasts, expanded]);
  useEffect(() => { if (toasts.length < 2) setExpanded(false); }, [toasts.length]);
  async function act(fn: () => Promise<unknown>) {
    setBusy(true); setError("");
    try { await fn(); await qc.invalidateQueries({ queryKey: ["notifications"] }); await qc.invalidateQueries({ queryKey: ["notification-preferences"] }); }
    catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  }
  useEffect(() => {
    if (!open) { setConfirmClear(false); return; }
    // Esc first dismisses a pending confirmation, then the panel.
    const esc = (e: KeyboardEvent) => { if (e.key !== "Escape") return; setConfirmClear((c) => { if (!c) setOpen(false); return false; }); };
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [open]);
  async function read(n: Notice) { await request("/" + n.id + "/read", "POST"); }
  async function push() {
    const reg = await navigator.serviceWorker.ready;
    if (subscribed) {
      const sub = await reg.pushManager.getSubscription();
      if (sub) { await request("/subscription", "DELETE", { endpoint: sub.endpoint }); await sub.unsubscribe(); }
      setSubscribed(false);
    } else {
      if (await Notification.requestPermission() !== "granted") throw new Error("El navegador no ha concedido permiso para push. Los avisos dentro de Dory siguen disponibles.");
      const key = prefs.data!.public_key;
      const bytes = Uint8Array.from(atob(key.replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
      const sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: bytes });
      await request("/subscription", "POST", sub.toJSON());
      setSubscribed(true);
    }
  }
  return <>
    <Button variant="bare" aria-label={`Notificaciones: ${feed.data?.unread ?? 0} sin leer`} onClick={() => setOpen(true)}>
      {/* The count is anchored to the bell itself, not to the button padding, so it sits the same on every screen. */}
      <span className="relative block">
        <Bell weight="duotone" className="size-5" aria-hidden="true" />
        {!!feed.data?.unread && <span className="absolute -top-1.5 left-3 grid min-w-4 place-items-center rounded-full bg-red-600 px-1 text-[10px] font-bold leading-4 text-white ring-2 ring-white dark:ring-surface">{feed.data.unread > 99 ? "99+" : feed.data.unread}</span>}
      </span>
    </Button>
    {open && createPortal(
      <div className="fixed inset-0 z-[100]" onMouseDown={(e) => e.target === e.currentTarget && setOpen(false)}>
        <div role="dialog" aria-modal="true" aria-label="Notificaciones"
          className="notif-in absolute inset-0 flex flex-col overflow-hidden bg-white shadow-2xl dark:bg-surface sm:inset-auto sm:bottom-3 sm:left-[14.5rem] sm:max-h-[min(36rem,calc(100dvh-1.5rem))] sm:w-[24rem] sm:rounded-lg sm:border sm:border-slate-200 sm:dark:border-border">
          <div className="flex shrink-0 items-center gap-1 border-b border-slate-200 px-3 py-2 dark:border-border">
            <h2 className="mr-auto flex items-center gap-2 text-sm font-semibold">
              {settingsOpen ? "Ajustes de avisos" : "Notificaciones"}
              {!settingsOpen && !!feed.data?.unread && <span className="rounded-md bg-red-600 px-1.5 text-[11px] font-bold leading-4 text-white">{feed.data.unread}</span>}
            </h2>
            <button type="button" title={settingsOpen ? "Volver a la lista" : "Ajustes"} aria-label={settingsOpen ? "Volver a la lista" : "Ajustes"} aria-pressed={settingsOpen} onClick={() => setSettingsOpen(!settingsOpen)} className={iconBtn}>{settingsOpen ? <ArrowLeft weight="regular" aria-hidden="true" className="size-4" /> : <GearSix aria-hidden="true" className="size-4" />}</button>
            {!settingsOpen && <button type="button" title="Borrar todo el historial" aria-label="Borrar todo el historial" disabled={busy || !notices.length} onClick={() => setConfirmClear(true)} aria-pressed={confirmClear} className={iconBtn}><Trash aria-hidden="true" className="size-4" /></button>}
            <button type="button" title="Cerrar" aria-label="Cerrar" autoFocus onClick={() => setOpen(false)} className={iconBtn}><X weight="regular" aria-hidden="true" className="size-4" /></button>
          </div>
          {confirmClear && !settingsOpen && (
            <div role="alertdialog" aria-label="Borrar historial" className="notif-in shrink-0 space-y-2 border-b border-red-200 bg-red-50 px-3 py-2.5 dark:border-red-900 dark:bg-red-950/40">
              <p className="text-sm"><span className="font-semibold text-red-800 dark:text-red-300">¿Borrar todo el historial?</span> <span className="text-red-700 dark:text-red-300/80">Se eliminarán todas las notificaciones. No se puede deshacer.</span></p>
              <div className="flex justify-end gap-2">
                <Button variant="ghost" autoFocus onClick={() => setConfirmClear(false)}>Cancelar</Button>
                <Button className="!bg-red-600 hover:!bg-red-700 dark:!bg-red-600 dark:hover:!bg-red-700" disabled={busy} onClick={() => { setConfirmClear(false); void act(() => request("", "DELETE")); }}>Borrar todo</Button>
              </div>
            </div>
          )}
          {(error || feed.error || prefs.error) && <p role="alert" className="shrink-0 border-b border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300">{error || "No se pueden cargar las notificaciones"}</p>}
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            {settingsOpen ? (
              <div className="space-y-4 p-3">
                <table className="w-full text-sm">
                  <thead className="text-xs text-slate-500 dark:text-muted"><tr><th className="pb-2 text-left font-medium">Evento</th>{(["enabled", "bell", "toast", "push"] as Channel[]).map(c => <th key={c} className="pb-2 text-center font-medium">{channelLabel[c]}</th>)}</tr></thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-border/60">
                    {Object.entries(events).map(([kind, label]) => <tr key={kind}>
                      <td className="py-2 pr-2">{label}</td>
                      {(["enabled", "bell", "toast", "push"] as Channel[]).map(c => <td key={c} className="py-2 text-center"><input type="checkbox" aria-label={`${label}: ${channelLabel[c]}`} className="size-4 accent-slate-900 dark:accent-accent" disabled={busy || !prefs.data} checked={prefs.data?.settings[kind]?.[c] ?? (c !== "enabled" || onByDefault.has(kind))} onChange={e => { const settings = prefs.data!.settings; void act(() => request("/preferences", "PUT", { ...settings, [kind]: { ...settings[kind], [c]: e.target.checked } })); }} /></td>)}
                    </tr>)}
                  </tbody>
                </table>
                <p className="text-xs text-slate-500 dark:text-muted">Los cambios afectan a avisos futuros. El push se activa en cada dispositivo.</p>
                {supported ? <Button variant="ghost" className="w-full" disabled={busy || !prefs.data} onClick={() => void act(push)}>{subscribed ? "Desactivar push en este dispositivo" : "Activar push en este dispositivo"}</Button> : <p className="text-xs text-slate-500 dark:text-muted">Push no disponible en este navegador.</p>}
              </div>
            ) : (
              <>
                <ul className="divide-y divide-slate-100 dark:divide-border/60">
                  {notices.filter(n => n.bell).map(n => <li key={n.id} className={`group relative flex gap-2.5 px-3 py-2.5 hover:bg-slate-50 dark:hover:bg-raised ${n.read ? "" : "bg-slate-50 dark:bg-raised/50"}`}>
                    <span aria-label={n.read ? undefined : "No leída"} className={`mt-1.5 size-2 shrink-0 rounded-full ${n.read ? "bg-transparent" : "bg-slate-400 dark:bg-white"}`} />
                    <a href={href(n.url)} {...external(href(n.url))} onClick={() => void act(() => read(n))} className="min-w-0 flex-1">
                      <div className={`text-sm ${n.read ? "text-slate-600 dark:text-text-secondary" : "font-semibold"}`}>{n.title}</div>
                      {n.body && <p className="line-clamp-2 break-words text-xs text-slate-500 dark:text-muted">{n.body}</p>}
                      <time className="mt-0.5 block text-[11px] text-slate-400 dark:text-muted" title={new Date(n.created_at).toLocaleString()}>{ago(n.created_at)}</time>
                    </a>
                    {!n.read && <button type="button" title="Marcar como leída" aria-label="Marcar como leída" disabled={busy} onClick={() => void act(() => read(n))} className={`${iconBtn} self-start opacity-100 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100`}><Check weight="regular" aria-hidden="true" className="size-4" /></button>}
                  </li>)}
                </ul>
                {history.hasNextPage && <div className="p-3"><Button variant="ghost" className="w-full" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>Cargar avisos anteriores</Button></div>}
                {history.data && !notices.some(n => n.bell) && <div className="flex flex-col items-center gap-2 px-3 py-12 text-sm text-slate-500 dark:text-muted"><Bell aria-hidden="true" className="size-6" />Sin avisos todavía.</div>}
                {history.isPending && <p className="px-3 py-6 text-center text-sm text-slate-500 dark:text-muted">Cargando…</p>}
              </>
            )}
          </div>
        </div>
      </div>,
      document.body,
    )}
    {createPortal(<ToastStack toasts={toasts} expanded={expanded} setExpanded={setExpanded} onOpen={(n) => void act(() => read(n))} onClose={(id) => setToasts(old => old.filter(x => x.id !== id))} />, document.body)}
  </>;
}

// Destinations resolve the item independently of the selected project.
export function NotificationItem() {
  const id = new URLSearchParams(window.location.search).get("notification-item");
  const item = useQuery({ queryKey: ["notification-item", id], queryFn: () => api.items.list({ id }), enabled: !!id });
  if (!id) return null;
  if (item.error) return <p role="alert">No se pudo abrir el item.</p>;
  if (!item.data) return null;
  if (!item.data[0]) return <p>El item ya no está disponible.</p>;
  return <ItemForm item={item.data[0]} onClose={() => { window.location.href = "/inbox"; }} />;
}

// ToastStack shows in-app notices top-right, stacking downwards. Collapsed, the newest sits on top and the
// older ones peek out above it; a click expands the stack upwards to show all of them.
function ToastStack({ toasts, expanded, setExpanded, onOpen, onClose }: { toasts: Notice[]; expanded: boolean; setExpanded: (v: boolean) => void; onOpen: (n: Notice) => void; onClose: (id: number) => void }) {
  const refs = useRef(new Map<number, HTMLDivElement>());
  const [heights, setHeights] = useState<Record<number, number>>({});
  useLayoutEffect(() => {
    const next: Record<number, number> = {};
    refs.current.forEach((el, id) => { next[id] = el.offsetHeight; });
    setHeights((old) => (JSON.stringify(old) === JSON.stringify(next) ? old : next));
  }, [toasts]);
  const stacked = toasts.length > 1 && !expanded;
  // Swipe to dismiss: sideways or upwards with finger or mouse. A drag never opens the notice.
  const [drag, setDrag] = useState<{ id: number; x: number; y: number; dx: number; dy: number } | null>(null);
  const dragged = useRef(false);
  const swipe = (n: Notice) => ({
    onPointerDown: (e: RPointerEvent) => {
      if (e.button !== 0 || (e.target as HTMLElement).closest("button")) return;
      dragged.current = false;
      setDrag({ id: n.id, x: e.clientX, y: e.clientY, dx: 0, dy: 0 });
    },
    onPointerMove: (e: RPointerEvent) => {
      if (drag?.id !== n.id) return;
      const dx = e.clientX - drag.x, dy = Math.min(0, e.clientY - drag.y);
      if (!dragged.current && Math.hypot(dx, dy) > 6) { dragged.current = true; (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); }
      if (dragged.current) setDrag({ ...drag, dx, dy });
    },
    onPointerUp: () => {
      if (drag?.id !== n.id) return;
      if (Math.abs(drag.dx) > 80 || drag.dy < -40) onClose(n.id);
      setDrag(null);
    },
    onPointerCancel: () => setDrag(null),
  });
  let offset = 0;
  return (
    <div aria-live="polite" onMouseLeave={() => setExpanded(false)}
      className="fixed right-4 top-4 z-[110] w-96 max-w-[calc(100vw-2rem)]"
      style={{ height: toasts.length ? (expanded ? toasts.reduce((h, n) => h + (heights[n.id] ?? 72) + 8, -8) : (heights[toasts[0]?.id] ?? 72) + Math.min(toasts.length - 1, 3) * 8) : 0 }}>
      {toasts.map((n, i) => {
        const y = expanded ? offset : i * 8;
        offset += (heights[n.id] ?? 72) + 8;
        return (
          <div key={n.id} ref={(el) => { if (el) refs.current.set(n.id, el); else refs.current.delete(n.id); }}
            {...swipe(n)}
            onClickCapture={(e) => { if (dragged.current) { e.preventDefault(); e.stopPropagation(); dragged.current = false; return; } if (stacked && !(e.target as HTMLElement).closest("button")) { e.preventDefault(); e.stopPropagation(); setExpanded(true); } }}
            style={(() => { const d = drag?.id === n.id ? drag : null; return { transform: `translate(${d?.dx ?? 0}px, ${y + (d?.dy ?? 0)}px) scale(${expanded ? 1 : 1 - i * 0.05})`, zIndex: 10 - i, opacity: !expanded && i > 2 ? 0 : d ? Math.max(0.2, 1 - Math.max(Math.abs(d.dx) / 200, -d.dy / 100)) : 1, transition: d ? "none" : undefined, touchAction: "none" }; })()}
            className="toast-in absolute inset-x-0 top-0 origin-top rounded-lg border border-slate-200 bg-white p-3 shadow-lg transition-all duration-300 ease-out motion-reduce:transition-none dark:border-border dark:bg-surface">
            <a href={href(n.url)} {...external(href(n.url))} onClick={() => onOpen(n)} className="block pr-6"><span className="block text-sm font-semibold">{n.title}</span>{n.body && <span className={`block break-words text-sm ${stacked && i > 0 ? "line-clamp-1" : ""}`}>{n.body}</span>}</a>
            <button className="absolute right-1 top-1 px-2" aria-label="Cerrar aviso" onClick={() => onClose(n.id)}><X weight="regular" aria-hidden="true" className="size-4" /></button>
            {stacked && i === 0 && <span className="mt-1 block text-xs text-slate-500 dark:text-muted">+{toasts.length - 1} más · clic para ver</span>}
          </div>
        );
      })}
    </div>
  );
}
