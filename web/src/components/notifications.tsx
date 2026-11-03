import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Modal } from "./ui";
import { ItemForm } from "../pages/items";
import { api } from "../api/client";

type Notice = { id: number; title: string; body: string; url: string; bell: boolean; toast: boolean; read: boolean; created_at: string };
type Channel = "enabled" | "bell" | "toast" | "push";
type Settings = Record<string, Partial<Record<Channel, boolean>>>;
const events = { item_ready: "Item listo", item_blocked: "Item bloqueado", thread_finished: "Hilo finalizado", thread_attention: "Hilo requiere atención" };
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
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [subscribed, setSubscribed] = useState(false);
  const [toasts, setToasts] = useState<Notice[]>([]);
  const since = useRef<string | null>(null);
  const seen = useRef(new Set<number>());
  const feed = useQuery({ queryKey: ["notifications", "feed"], queryFn: () => request<{ items: Notice[]; unread: number; server_time: string }>(""), refetchInterval: 3000, refetchOnWindowFocus: true });
  const history = useInfiniteQuery({
    queryKey: ["notifications", "history"], initialPageParam: 0,
    queryFn: ({ pageParam }) => request<{ items: Notice[] }>("?before=" + pageParam),
    getNextPageParam: page => page.items.length === 100 ? page.items[page.items.length - 1].id : undefined,
    enabled: open, refetchInterval: open ? 5000 : false,
  });
  const notices = history.data?.pages.flatMap(p => p.items) ?? [];
  const prefs = useQuery({ queryKey: ["notification-preferences"], queryFn: () => request<{ settings: Settings; public_key: string }>("/preferences"), refetchInterval: 5000 });
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
  useEffect(() => { if (!toasts.length) return; const timer = window.setTimeout(() => setToasts([]), 8000); return () => clearTimeout(timer); }, [toasts]);
  async function act(fn: () => Promise<unknown>) {
    setBusy(true); setError("");
    try { await fn(); await qc.invalidateQueries({ queryKey: ["notifications"] }); await qc.invalidateQueries({ queryKey: ["notification-preferences"] }); }
    catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  }
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
    <Button variant="ghost" className="relative" aria-label={`Notificaciones: ${feed.data?.unread ?? 0} sin leer`} onClick={() => setOpen(true)}>
      <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" /></svg>
      {!!feed.data?.unread && <span className="absolute -right-0.5 -top-0.5 grid min-w-4 place-items-center rounded-full bg-red-600 px-1 text-[10px] font-bold leading-4 text-white">{feed.data.unread > 99 ? "99+" : feed.data.unread}</span>}
    </Button>
    {open && <Modal title="Notificaciones · todos los proyectos" onClose={() => setOpen(false)}>
      <div className="flex gap-2">
        <Button onClick={() => setSettingsOpen(!settingsOpen)}>Ajustes</Button>
        <Button disabled={busy || !notices.length} onClick={() => { if (window.confirm("¿Borrar todo el historial de notificaciones? No se puede deshacer.")) void act(() => request("", "DELETE")); }}>Limpiar todo</Button>
      </div>
      {(error || feed.error || prefs.error) && <p role="alert" className="text-red-600 dark:text-red-400">{error || "No se pueden cargar las notificaciones"}</p>}
      {settingsOpen && <div className="space-y-4 py-4">
        {Object.entries(events).map(([kind, label]) => <fieldset key={kind} className="rounded border border-slate-200 dark:border-border p-2"><legend>{label}</legend><div className="flex flex-wrap gap-3">
          {(["enabled", "bell", "toast", "push"] as Channel[]).map(c => <label key={c} className="flex items-center gap-1 text-sm"><input type="checkbox" disabled={busy || !prefs.data} checked={prefs.data?.settings[kind]?.[c] ?? true} onChange={e => { const settings = prefs.data!.settings; void act(() => request("/preferences", "PUT", { ...settings, [kind]: { ...settings[kind], [c]: e.target.checked } })); }} />{{ enabled: "Activo", bell: "Campana/lista", toast: "Toast", push: "Push" }[c]}</label>)}
        </div></fieldset>)}
        <p className="text-sm">Los cambios afectan a futuros avisos. El permiso y la activación push corresponden a este dispositivo.</p>
        {supported ? <Button disabled={busy || !prefs.data} onClick={() => void act(push)}>{subscribed ? "Desactivar push en este dispositivo" : "Activar push en este dispositivo"}</Button> : <p>Push no disponible en este navegador. Campana y toast siguen disponibles.</p>}
      </div>}
      <ul className="space-y-2 py-3">
        {notices.filter(n => n.bell).map(n => <li key={n.id} className={`rounded border p-3 ${n.read ? "border-slate-200 dark:border-border opacity-70" : "border-sky-300 bg-sky-50 dark:border-sky-700 dark:bg-sky-950/40"}`}>
          <a href={n.url} onClick={() => void act(() => read(n))} className="block hover:underline">
            <div className="flex items-center gap-2 text-sm font-semibold">{!n.read && <span aria-label="No leída" className="size-2 shrink-0 rounded-full bg-sky-500" />}{n.title}</div>
            {n.body && <p className="line-clamp-2 break-words text-sm text-slate-600 dark:text-muted">{n.body}</p>}
          </a>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-muted"><time title={new Date(n.created_at).toLocaleString()}>{ago(n.created_at)}</time>{!n.read && <Button variant="ghost" disabled={busy} onClick={() => void act(() => read(n))}>Marcar leído</Button>}</div>
        </li>)}
      </ul>
      {history.hasNextPage && <Button disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>Cargar avisos anteriores</Button>}
      {history.data && !notices.some(n => n.bell) && <p className="py-4">Sin avisos todavía.</p>}
    </Modal>}
    <div aria-live="polite" className="fixed bottom-4 right-4 z-50 w-80 max-w-[calc(100vw-2rem)] space-y-2">
      {toasts.map(n => <div key={n.id} className="relative rounded border border-slate-200 dark:border-border bg-white dark:bg-surface p-3 shadow-lg"><a href={n.url} onClick={() => void act(() => read(n))} className="block pr-6"><span className="block text-sm font-semibold">{n.title}</span>{n.body && <span className="block break-words text-sm">{n.body}</span>}</a><button className="absolute right-1 top-1 px-2" aria-label="Cerrar aviso" onClick={() => setToasts(old => old.filter(x => x.id !== n.id))}>×</button></div>)}
    </div>
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
