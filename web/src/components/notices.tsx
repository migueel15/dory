import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bell, BellRinging } from "@phosphor-icons/react";
import { api, req } from "../api/client";

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
