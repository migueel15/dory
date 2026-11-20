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

// NoticesToggle enables notices; with them on, it also shows a local notice
// when a thread of any node finishes, fails or needs you while the window is
// not in focus (the desktop app has no push service).
export function NoticesToggle() {
  const [perm, setPerm] = useState(() => ("Notification" in window ? Notification.permission : "denied"));
  const [error, setError] = useState("");
  const threads = useQuery({ queryKey: ["threads", "attention"], queryFn: () => api.threads(), refetchInterval: 15_000, enabled: perm === "granted" });
  const last = useRef<Record<string, string> | undefined>(undefined);
  useEffect(() => {
    if (!threads.data) return;
    const prev = last.current;
    last.current = Object.fromEntries(threads.data.map((t) => [t.id, t.status]));
    if (!prev || document.hasFocus() || perm !== "granted") return;
    for (const t of threads.data) {
      const before = prev[t.id];
      if (!before || before === t.status || (before !== "working" && t.status !== "waiting")) continue;
      const title = { idle: "El agente terminó", waiting: "Un agente te necesita", error: "El agente falló" }[t.status as string];
      if (title) new Notification(title, { body: t.title, tag: t.id, icon: "/icon-192.png" }).onclick = () => { window.focus(); location.assign(`/threads/${t.id}`); };
    }
  }, [threads.data, perm]);
  const on = perm === "granted";
  return (
    <button type="button" title={error || (on ? "Avisos activados" : "Activar avisos")} aria-label={on ? "Avisos activados" : "Activar avisos"}
      onClick={() => { if (!on) subscribe().then(() => setPerm("granted"), (e: Error) => { setError(e.message); setPerm(Notification.permission); }); }}
      className="grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 dark:hover:bg-raised">
      {on ? <BellRinging className="size-5" /> : <Bell className="size-5" />}
    </button>
  );
}

type Notice = { id: string; kind: string; title: string; body: string; url: string; read_at: string | null; created_at: string };

// useNotices lists the inbox, newest first; it is shared by every device of the mesh.
function useNotices() {
  return useQuery({ queryKey: ["notices"], queryFn: () => req<Notice[]>("GET", "/notices"), refetchInterval: 60_000 });
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
