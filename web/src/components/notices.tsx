import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Bell, BellRinging, Tray } from "@phosphor-icons/react";
import { api, req } from "../api/client";
import { Button, Modal } from "./ui";

const b64 = (s: string) => {
  const raw = atob((s + "=".repeat((4 - (s.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(raw, (c) => c.charCodeAt(0));
};

// subscribe asks for permission and registers this browser for push notices
// from this node (the phone app subscribes to the server node).
async function subscribe() {
  if (!("Notification" in window)) throw new Error("Este navegador no admite avisos");
  if ((await Notification.requestPermission()) !== "granted") throw new Error("Permiso denegado");
  const reg = await navigator.serviceWorker?.ready;
  if (reg?.pushManager) {
    const { key } = await req<{ key: string }>("GET", "/push/key");
    const sub = (await reg.pushManager.getSubscription()) ?? (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: b64(key) }));
    await req("POST", "/push/subscriptions", sub.toJSON());
  }
}

// NoticesToggle enables push and system notices on this device.
export function NoticesToggle() {
  const [perm, setPerm] = useState(() => ("Notification" in window ? Notification.permission : "denied"));
  const [error, setError] = useState("");
  const on = perm === "granted";
  return (
    <button type="button" title={error || (on ? "Avisos activados" : "Activar avisos")} aria-label={on ? "Avisos activados" : "Activar avisos"}
      onClick={() => { if (!on) subscribe().then(() => setPerm("granted"), (e: Error) => { setError(e.message); setPerm(Notification.permission); }); }}
      className="grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 dark:hover:bg-raised">
      {on ? <BellRinging className="size-5" /> : <Bell className="size-5" />}
    </button>
  );
}

type NoticeSetting = { kind: string; title: string; mode: "auto" | "push" | "app" | "off" };
type Notice = { id: string; kind: string; title: string; body: string; url: string; read_at: string | null; created_at: string };

// useNotices lists the inbox, newest first; it is shared by every device of the mesh.
function useNotices() {
  return useQuery({ queryKey: ["notices"], queryFn: () => req<Notice[]>("GET", "/notices"), refetchInterval: 15_000, refetchIntervalInBackground: true });
}

// NoticeInbox shows the inbox with its unread counter. Reading a notice marks
// it read on every device.
export function NoticeInbox() {
  const [open, setOpen] = useState(false);
  const notices = useNotices();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const read = useMutation({ mutationFn: (ids: string[]) => req("POST", "/notices/read", { ids }), onSettled: () => qc.invalidateQueries({ queryKey: ["notices"] }) });
  const list = notices.data ?? [];
  const unread = list.filter((n) => !n.read_at);
  const go = (n: Notice) => {
    if (!n.read_at) read.mutate([n.id]);
    setOpen(false);
    if (n.url) navigate({ to: n.url as never });
  };
  return (
    <>
      <button type="button" title="Avisos" aria-label={unread.length ? `Avisos, ${unread.length} sin leer` : "Avisos"} onClick={() => setOpen(true)}
        className="relative grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 dark:hover:bg-raised">
        <Tray className="size-5" />
        {unread.length > 0 && <span className="absolute right-0.5 top-0.5 grid min-w-4 place-items-center rounded-full bg-blue-600 px-1 text-[10px] font-bold leading-4 text-white">{unread.length > 99 ? "99+" : unread.length}</span>}
      </button>
      {open && (
        <Modal title="Avisos" onClose={() => setOpen(false)}>
          <div className="space-y-2">
            {unread.length > 0 && <Button variant="ghost" onClick={() => read.mutate([])}>Marcar todos como leídos</Button>}
            {list.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">No hay avisos.</p>}
            <ul className="space-y-1">
              {list.map((n) => (
                <li key={n.id}>
                  <button type="button" onClick={() => go(n)} className="flex w-full items-start gap-2 rounded-md px-2 py-2 text-left hover:bg-slate-100 dark:hover:bg-raised">
                    <span className={`mt-1.5 size-2 shrink-0 rounded-full ${n.read_at ? "bg-transparent" : "bg-blue-600"}`} aria-hidden="true" />
                    <span className="min-w-0 flex-1">
                      <span className={`block text-sm ${n.read_at ? "" : "font-semibold"}`}>{n.title}</span>
                      {n.body && <span className="block truncate text-sm text-slate-500 dark:text-muted">{n.body}</span>}
                      <span className="block text-xs text-slate-400 dark:text-muted">{new Date(n.created_at).toLocaleString()}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </Modal>
      )}
    </>
  );
}

// One id per loaded interface, so the mesh can tell open interfaces apart.
const interfaceId = crypto.randomUUID();

// usePresence tells the node whether this interface is open and has the
// focus. The node expires it if the heartbeat stops (a closed tab, a killed app).
function usePresence() {
  useEffect(() => {
    const send = (state: string) => fetch("/api/presence", { method: "POST", credentials: "include", keepalive: true, headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id: interfaceId, state }) }).catch(() => {});
    const state = () => (document.visibilityState === "visible" && document.hasFocus() ? "focus" : "open");
    const beat = () => send(state());
    beat();
    const id = setInterval(beat, 15_000);
    window.addEventListener("focus", beat);
    window.addEventListener("blur", beat);
    document.addEventListener("visibilitychange", beat);
    const leave = () => send("closed");
    window.addEventListener("pagehide", leave);
    return () => {
      clearInterval(id);
      window.removeEventListener("focus", beat);
      window.removeEventListener("blur", beat);
      document.removeEventListener("visibilitychange", beat);
      window.removeEventListener("pagehide", leave);
      leave();
    };
  }, []);
}

// NoticeAlerts shows each new notice where the user is: inside the app when
// this interface has the focus; as a system notification when none has the
// focus and there is no push to reach a phone. Otherwise push or the inbox
// take it, so nothing shows twice.
export function NoticeAlerts() {
  usePresence();
  const notices = useNotices();
  const navigate = useNavigate();
  const seen = useRef<Set<string> | undefined>(undefined);
  const [toasts, setToasts] = useState<Notice[]>([]);
  useEffect(() => {
    if (!notices.data) return;
    const known = seen.current;
    seen.current = new Set(notices.data.map((n) => n.id));
    if (!known) return;
    const fresh = notices.data.filter((n) => !n.read_at && !known.has(n.id) && Date.now() - Date.parse(n.created_at) < 120_000);
    if (fresh.length === 0) return;
    const focused = document.hasFocus();
    Promise.all([req<NoticeSetting[]>("GET", "/notice-settings"), focused ? Promise.resolve(undefined) : req<{ focus: boolean; push: boolean }>("GET", "/presence")]).then(([settings, d]) => {
      const mode = (n: Notice) => settings.find((s) => s.kind === n.kind)?.mode ?? "auto";
      if (focused) {
        const shown = fresh.filter((n) => mode(n) === "auto" || mode(n) === "app");
        if (shown.length === 0) return;
        setToasts((t) => [...shown, ...t].slice(0, 3));
        for (const n of shown) setTimeout(() => setToasts((t) => t.filter((x) => x.id !== n.id)), 8000);
        return;
      }
      if (!d || d.focus || d.push || !("Notification" in window) || Notification.permission !== "granted") return;
      for (const n of fresh.filter((n) => mode(n) === "auto")) new Notification(n.title, { body: n.body, tag: n.id, icon: "/icon-192.png" }).onclick = () => { window.focus(); if (n.url) location.assign(n.url); };
    }).catch(() => {});
  }, [notices.data]);
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-50 flex w-80 max-w-[calc(100%-2rem)] flex-col gap-2" role="status">
      {toasts.map((n) => (
        <button key={n.id} type="button" onClick={() => { setToasts((t) => t.filter((x) => x.id !== n.id)); if (n.url) navigate({ to: n.url as never }); }}
          className="rounded-lg border border-slate-200 bg-white p-3 text-left shadow-lg dark:border-border dark:bg-surface">
          <span className="block text-sm font-semibold">{n.title}</span>
          {n.body && <span className="block truncate text-sm text-slate-500 dark:text-muted">{n.body}</span>}
        </button>
      ))}
    </div>
  );
}
