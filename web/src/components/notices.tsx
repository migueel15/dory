import { Bell,BellRinging,Tray,X } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect,useId,useRef,useState } from "react";
import { req } from "../api/client";
import { qk } from "../api/queries/keys";
import { useNotices as useNoticeInbox,useReadNotices } from "../api/queries/settings";
import type { Notice } from "../api/types.gen";
import { Button } from "./ui";

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

// useNotices lists the inbox, newest first; it is shared by every device of the mesh.
function useNotices() {
  return useNoticeInbox({ refetchInterval: 15_000, refetchIntervalInBackground: true });
}

// NoticeInbox shows the inbox with its unread counter. Reading a notice marks
// it read on every device.
export function NoticeInbox() {
  const popoverId = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const notices = useNotices();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const read = useReadNotices({
    onSettled: () => qc.invalidateQueries({ queryKey: qk.notices() })
  });
  const list = notices.data ?? [];
  const unread = list.filter((n) => !n.read_at);
  const go = (n: Notice) => {
    if (!n.read_at) read.mutate([n.id]);
    panel.current?.hidePopover();
    if (n.url) navigate({ to: n.url as never });
  };
  // place opens the panel to the right of the icon, clamped to the viewport.
  const place = () => {
    const t = trigger.current, p = panel.current;
    if (!t || !p) return;
    const r = t.getBoundingClientRect();
    const width = Math.min(360, window.innerWidth - 24);
    const left = r.right + 8 + width <= window.innerWidth - 12 ? r.right + 8 : Math.max(12, window.innerWidth - width - 12);
    // Beside the icon it grows upwards from the icon's bottom edge; below it, downwards.
    const beside = left === r.right + 8;
    p.style.width = `${width}px`;
    p.style.left = `${left}px`;
    p.style.top = beside ? "auto" : `${r.bottom + 8}px`;
    p.style.bottom = beside ? `${window.innerHeight - r.bottom}px` : "auto";
    // Under half the room it has, so it reads as a pop-up and not a full column.
    p.style.maxHeight = `${Math.round(0.45 * (beside ? r.bottom - 12 : window.innerHeight - r.bottom - 20))}px`;
  };
  useEffect(() => {
    const again = (e?: Event) => {
      // Scrolling the list itself must not move or resize the panel.
      if (e?.type === "scroll" && panel.current?.contains(e.target as Node)) return;
      if (panel.current?.matches(":popover-open")) place();
    };
    window.addEventListener("resize", again);
    return () => { window.removeEventListener("resize", again); };
  }, []);
  // One wrapper: the sidebar sizes its direct children, and the panel must not be one.
  return (
    <div>
      <button ref={trigger} type="button" popoverTarget={popoverId} title="Avisos" aria-label={unread.length ? `Avisos, ${unread.length} sin leer` : "Avisos"} onClick={() => requestAnimationFrame(place)}
        className="relative grid size-full min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 dark:hover:bg-raised">
        <Tray className="size-5" />
        {unread.length > 0 && <span className="absolute right-0.5 top-0.5 grid min-w-4 place-items-center rounded-full bg-blue-600 px-1 text-[10px] font-bold leading-4 text-white">{unread.length > 99 ? "99+" : unread.length}</span>}
      </button>
      <div ref={panel} id={popoverId} popover="auto" role="dialog" aria-label="Avisos" onToggle={(e) => { if (e.newState === "open") place(); }}
        className="fixed m-0 overflow-auto overscroll-contain [scrollbar-gutter:stable] rounded-xl border border-slate-200 bg-white p-3 text-slate-900 shadow-xl dark:border-border dark:bg-surface dark:text-white">
          <h2 className="sticky -top-3 z-10 -mx-3 -mt-3 mb-2 bg-white px-3 pb-2 pt-3 text-base font-semibold dark:bg-surface">Avisos</h2>
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
      </div>
    </div>
  );
}

// One id per loaded interface, so the mesh can tell open interfaces apart.
const interfaceId = crypto.randomUUID();

// idleAfter is how long without input before a focused interface counts as
// unattended.
const idleAfter = 2 * 60_000;

// usePresence tells the node whether this interface is open and has the
// focus. The node expires it if the heartbeat stops (a closed tab, a killed app).
function usePresence() {
  useEffect(() => {
    const send = (state: string) => fetch("/api/presence", { method: "POST", credentials: "include", keepalive: true, headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id: interfaceId, state }) }).catch(() => {});
    // Focus means the user is at this interface now: visible, focused and
    // touched recently. A window left focused (the user walked away, the
    // screen locked) stops counting, so the phone gets the notices again.
    let last = Date.now();
    let sent = "";
    const state = () => (document.visibilityState === "visible" && document.hasFocus() && Date.now() - last < idleAfter ? "focus" : "open");
    const beat = () => {
      sent = state();
      send(sent);
    };
    const active = () => {
      last = Date.now();
      if (sent !== state()) beat();
    };
    beat();
    const id = setInterval(beat, 15_000);
    const inputs = ["keydown", "pointerdown", "pointermove", "wheel", "touchstart"];
    inputs.forEach((e) => window.addEventListener(e, active, { passive: true }));
    window.addEventListener("focus", beat);
    window.addEventListener("blur", beat);
    document.addEventListener("visibilitychange", beat);
    const leave = () => send("closed");
    window.addEventListener("pagehide", leave);
    return () => {
      clearInterval(id);
      inputs.forEach((e) => window.removeEventListener(e, active));
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
// focus, even if push also reaches other devices.
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
    const focused = document.visibilityState === "visible" && document.hasFocus();
    req<NoticeSetting[]>("GET", "/notice-settings").then((settings) => {
      const mode = (n: Notice) => settings.find((s) => s.kind === n.kind)?.mode ?? "auto";
      if (focused) {
        const shown = fresh.filter((n) => mode(n) === "auto" || mode(n) === "app");
        setToasts((t) => [...shown, ...t].slice(0, 3));
        for (const n of shown) setTimeout(() => setToasts((t) => t.filter((x) => x.id !== n.id)), 8000);
      }
      // The Android app's native service notifies on its own.
      if ((window as unknown as { DoryAndroid?: unknown }).DoryAndroid) return;
      if (!("Notification" in window) || Notification.permission !== "granted") return;
      // Same tag as the push of this notice, so a browser with both shows it once.
      // "Always push" also notifies the focused interface.
      for (const n of fresh.filter((n) => mode(n) === "push" || (!focused && mode(n) === "auto"))) new Notification(n.title, { body: n.body, tag: n.id, icon: "/icon-192.png" }).onclick = () => { window.focus(); if (n.url) location.assign(n.url); };
    }).catch(() => {});
  }, [notices.data]);
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 max-w-[calc(100%-2rem)] flex-col gap-2" role="status">
      {toasts.map((n) => (
        <Toast key={n.id} notice={n} onDismiss={() => setToasts((t) => t.filter((x) => x.id !== n.id))}
          onOpen={() => { setToasts((t) => t.filter((x) => x.id !== n.id)); if (n.url) navigate({ to: n.url as never }); }} />
      ))}
    </div>
  );
}

// Toast is one in-app notice. A sideways swipe or the close button dismisses
// it, so it never blocks the composer underneath for long.
function Toast({ notice: n, onOpen, onDismiss }: { notice: Notice; onOpen: () => void; onDismiss: () => void }) {
  const [dx, setDx] = useState(0);
  const start = useRef<number | undefined>(undefined);
  const moved = useRef(false);
  return (
    <div style={{ transform: `translateX(${dx}px)`, opacity: 1 - Math.min(Math.abs(dx) / 160, 0.8), transition: start.current === undefined ? "transform 150ms, opacity 150ms" : "none" }}
      className="pointer-events-auto flex touch-pan-y items-start rounded-lg border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface"
      onPointerDown={(e) => { start.current = e.clientX; moved.current = false; }}
      onPointerMove={(e) => { if (start.current === undefined) return; const d = e.clientX - start.current; if (!moved.current && Math.abs(d) > 6) { moved.current = true; e.currentTarget.setPointerCapture(e.pointerId); } if (moved.current) setDx(d); }}
      onPointerUp={() => { start.current = undefined; if (Math.abs(dx) > 80) onDismiss(); else setDx(0); }}
      onPointerCancel={() => { start.current = undefined; setDx(0); }}>
      <button type="button" onClick={() => { if (!moved.current) onOpen(); }} className="min-w-0 flex-1 p-3 text-left">
        <span className="block text-sm font-semibold">{n.title}</span>
        {n.body && <span className="block truncate text-sm text-slate-500 dark:text-muted">{n.body}</span>}
      </button>
      <button type="button" aria-label="Cerrar aviso" onClick={onDismiss}
        className="grid min-h-11 min-w-11 shrink-0 place-items-center rounded-md text-slate-400 hover:text-slate-700 dark:hover:text-white">
        <X weight="regular" aria-hidden="true" className="size-4" />
      </button>
    </div>
  );
}
