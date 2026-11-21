import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type WheelEvent } from "react";
import { ArrowClockwise, ArrowLeft, ArrowRight, Browser, HandPointing, Plus, Robot, X } from "@phosphor-icons/react";
import { req } from "../api/client";
import { showPanelTab } from "./side-panel";

// The browser of a thread lives on the node of the thread (internal/browser).
// In the desktop app the active tab is a native view the node drives through
// this panel; while the agent is in a turn, or without the desktop app, the
// panel shows the node's headless page as a screencast.
export type BrowserTab = { id: string; url: string; title: string; position: number; active: boolean; backend: "" | "node" | "desktop" };
type BrowserState = { tabs: BrowserTab[]; user_control: boolean };
type Frame = { tab: string; src: string; w: number; h: number };

const desktop = () => window.doryDesktop?.browser;
const wsURL = (path: string) => `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api${path}`;

// openInThreadBrowser opens url in a new tab of the thread's browser and
// brings the side panel to it.
export async function openInThreadBrowser(threadId: string, url: string) {
  await req("POST", `/threads/${threadId}/browser/tabs`, { url });
  showPanelTab(threadId, "browser");
}

// The desktop app reaches the loopback ports of a remote node through a
// tunnel: each local connection becomes a WebSocket to that node.
const tunnelThreads = new Map<number, string>();
let tunnelsReady = false;
function setupTunnels() {
  const b = desktop();
  if (!b || tunnelsReady) return;
  tunnelsReady = true;
  const conns = new Map<number, { ws: WebSocket; queue: Uint8Array[] }>();
  b.onTunnelOpen((id, port) => {
    const thread = tunnelThreads.get(port);
    if (!thread) { b.tunnelEnd(id); return; }
    const ws = new WebSocket(wsURL(`/threads/${thread}/browser/tunnel?port=${port}`));
    ws.binaryType = "arraybuffer";
    const c = { ws, queue: [] as Uint8Array[] };
    conns.set(id, c);
    ws.onopen = () => { for (const d of c.queue) ws.send(d); c.queue = []; };
    ws.onmessage = (e) => b.tunnelWrite(id, e.data as ArrayBuffer);
    ws.onclose = () => { conns.delete(id); b.tunnelEnd(id); };
  });
  b.onTunnelData((id, data) => {
    const c = conns.get(id);
    if (!c) return;
    if (c.ws.readyState === WebSocket.OPEN) c.ws.send(data); else c.queue.push(data);
  });
  b.onTunnelClose((id) => { conns.get(id)?.ws.close(); conns.delete(id); });
}

function loopbackPort(url: string): number | null {
  try {
    const u = new URL(url);
    if (!["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)) return null;
    return Number(u.port) || (u.protocol === "https:" ? 443 : 80);
  } catch { return null; }
}

// overlayOpen tells whether a menu or dialog of the interface is open; the
// native view hides then, since it would cover it.
const overlayOpen = () => !!document.querySelector('[role=dialog],[role=alertdialog],[role=menu],[role=listbox],[data-radix-popper-content-wrapper]');

export function BrowserPanel({ threadId, remote }: { threadId: string; remote: boolean }) {
  const [state, setState] = useState<BrowserState>({ tabs: [], user_control: false });
  const [frame, setFrame] = useState<Frame | null>(null);
  const [agentAt, setAgentAt] = useState<{ tab: string; at: number } | null>(null);
  const [error, setError] = useState("");
  const [attached, setAttached] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const ws = useRef<WebSocket | null>(null);
  const view = useRef<HTMLDivElement>(null);
  const active = state.tabs.find((t) => t.active) ?? null;
  const activeRef = useRef(active);
  activeRef.current = active;
  const send = (m: Record<string, unknown>) => { if (ws.current?.readyState === WebSocket.OPEN) ws.current.send(JSON.stringify(m)); };
  const size = () => { const r = view.current?.getBoundingClientRect(); return { width: Math.round(r?.width ?? 0), height: Math.round(r?.height ?? 0) }; };

  useEffect(() => {
    const b = desktop();
    if (b) setupTunnels();
    let closed = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    const offs: (() => void)[] = [];
    const connect = () => {
      const s = new WebSocket(wsURL(`/threads/${threadId}/browser/ws${b ? "?desktop=1" : ""}`));
      ws.current = s;
      s.onopen = () => setConnected(true);
      s.onclose = () => {
        setConnected(false);
        setAttached((t) => { if (t) b?.hide(t); return null; });
        if (!closed) retry = setTimeout(connect, 2000);
      };
      s.onmessage = async (e) => {
        const m = JSON.parse(e.data as string);
        switch (m.t) {
          case "state": setState(m.state); break;
          case "frame": setFrame({ tab: m.tab, src: `data:image/jpeg;base64,${m.data}`, w: m.w, h: m.h }); break;
          case "agent": setAgentAt({ tab: m.tab, at: Date.now() }); break;
          case "error": setError(m.message); setTimeout(() => setError(""), 5000); break;
          case "busy":
            // The agent is in a turn: watch the node's page and ask again soon.
            setTimeout(() => { if (activeRef.current?.id === m.tab) send({ t: "view", tab: m.tab, ...size() }); }, 3000);
            break;
          case "attach": {
            if (!b || !view.current) return;
            const port = loopbackPort(m.url);
            if (remote && port) {
              tunnelThreads.set(port, threadId);
              if (!(await b.tunnelListen(port))) setError(`El puerto ${port} ya está en uso en este equipo; se muestra lo que hay en él.`);
            }
            await b.show(m.tab, m.url, view.current.getBoundingClientRect());
            setAttached(m.tab);
            setFrame(null);
            send({ t: "ready", tab: m.tab });
            break;
          }
          case "detach": b?.hide(m.tab); setAttached((t) => (t === m.tab ? null : t)); break;
          case "cdp":
            b?.cdp(m.tab, m.method, m.params)
              .then((result) => send({ t: "cdp_result", tab: m.tab, id: m.id, result }))
              .catch((err: Error) => send({ t: "cdp_result", tab: m.tab, id: m.id, error: err?.message || String(err) }));
            break;
        }
      };
    };
    connect();
    if (b) {
      offs.push(b.onEvent((tab, method, params) => send({ t: "cdp_event", tab, method, params })));
      offs.push(b.onOpen((_tab, url) => send({ t: "open", url, new: true })));
      offs.push(b.onGone((tab) => setAttached((t) => (t === tab ? null : t))));
    }
    return () => {
      closed = true;
      clearTimeout(retry);
      offs.forEach((f) => f());
      ws.current?.close();
      setAttached((t) => { if (t) b?.hide(t); return null; });
    };
  }, [threadId]); // eslint-disable-line react-hooks/exhaustive-deps

  // The panel shows the active tab; leaving it gives the tab back to the node.
  const activeId = active?.id;
  useEffect(() => {
    if (!activeId || !connected) return;
    setFrame(null);
    send({ t: "view", tab: activeId, ...size() });
    return () => {
      send({ t: "hide", tab: activeId });
      const b = desktop();
      setAttached((t) => { if (t === activeId) b?.hide(t); return t === activeId ? null : t; });
    };
  }, [activeId, connected]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keep the native view on the panel area, clipped by the panel while it
  // slides, and out of the way of menus and dialogs.
  useEffect(() => {
    const b = desktop();
    if (!b || !attached) return;
    let last = "";
    let raf = 0;
    const tick = () => {
      const el = view.current;
      if (el) {
        const r = el.getBoundingClientRect();
        const clip = el.closest("aside")?.getBoundingClientRect() ?? r;
        const x = Math.max(r.left, clip.left), y = Math.max(r.top, clip.top);
        const box = { x, y, width: Math.max(0, Math.min(r.right, clip.right) - x), height: Math.max(0, Math.min(r.bottom, clip.bottom) - y) };
        const visible = !el.closest("[aria-hidden=true]") && box.width > 1 && !overlayOpen() && document.visibilityState === "visible";
        const key = `${box.x},${box.y},${box.width},${box.height},${visible}`;
        if (key !== last) { last = key; b.bounds(attached, box, visible); }
      }
      raf = requestAnimationFrame(tick);
    };
    tick();
    return () => cancelAnimationFrame(raf);
  }, [attached]);

  // A headless page follows the size of the panel.
  useEffect(() => {
    const el = view.current;
    if (!el || !activeId || attached) return;
    let t: ReturnType<typeof setTimeout> | undefined;
    const ro = new ResizeObserver(() => { clearTimeout(t); t = setTimeout(() => send({ t: "view", tab: activeId, ...size() }), 250); });
    ro.observe(el);
    return () => { ro.disconnect(); clearTimeout(t); };
  }, [activeId, attached]); // eslint-disable-line react-hooks/exhaustive-deps

  const [now, setNow] = useState(Date.now());
  useEffect(() => { if (!agentAt) return; const t = setTimeout(() => setNow(Date.now()), 4000); return () => clearTimeout(t); }, [agentAt]);
  const agentOn = (tab: string) => agentAt?.tab === tab && Math.max(now, Date.now()) - agentAt.at < 4000;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-slate-200 px-2 py-1 dark:border-border">
        {state.tabs.map((t) => (
          <div key={t.id} className={`group flex max-w-48 shrink-0 items-center rounded-md text-xs ${t.active ? "bg-slate-100 font-medium dark:bg-raised" : "text-slate-500 hover:bg-slate-50 dark:text-muted dark:hover:bg-raised/60"}`}>
            <button type="button" onClick={() => send({ t: "activate", tab: t.id })} title={t.url} className="flex min-w-0 items-center gap-1.5 py-1 pl-2 pr-1">
              {agentOn(t.id) ? <Robot weight="fill" className="size-3.5 shrink-0 animate-pulse text-violet-500" aria-label="El agente está usando esta pestaña" /> : <Browser className="size-3.5 shrink-0" />}
              <span className="truncate">{t.title || t.url}</span>
            </button>
            <button type="button" onClick={() => send({ t: "close", tab: t.id })} aria-label="Cerrar la pestaña" className="mr-1 grid size-4 place-items-center opacity-60 hover:opacity-100"><X className="size-3" /></button>
          </div>
        ))}
        <button type="button" onClick={() => send({ t: "open", url: "about:blank", new: true })} title="Nueva pestaña" aria-label="Nueva pestaña"
          className="grid size-6 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><Plus className="size-3.5" /></button>
      </div>
      {active && <AddressBar tab={active} userControl={state.user_control} send={send} />}
      {error && <p className="shrink-0 bg-amber-50 px-3 py-1 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">{error}</p>}
      <div ref={view} className="relative min-h-0 flex-1 bg-white dark:bg-[#1a1a1d]">
        {!active ? (
          <div className="grid h-full place-items-center p-6 text-center text-sm text-slate-500 dark:text-muted">
            <div className="space-y-2">
              <p>{connected ? "Este hilo no tiene pestañas abiertas." : "Conectando con el navegador del hilo…"}</p>
              {connected && <button type="button" onClick={() => send({ t: "open", url: "about:blank", new: true })} className="inline-flex items-center gap-1.5 rounded-md border border-slate-200 px-3 py-1.5 hover:bg-slate-50 dark:border-border dark:hover:bg-raised"><Plus className="size-3.5" />Nueva pestaña</button>}
            </div>
          </div>
        ) : attached === active.id ? null : (
          <Screencast tab={active.id} frame={frame?.tab === active.id ? frame : null} send={send} />
        )}
      </div>
    </div>
  );
}

function AddressBar({ tab, userControl, send }: { tab: BrowserTab; userControl: boolean; send: (m: Record<string, unknown>) => void }) {
  const [url, setUrl] = useState(tab.url);
  const [editing, setEditing] = useState(false);
  useEffect(() => { if (!editing) setUrl(tab.url === "about:blank" ? "" : tab.url); }, [tab.url, editing]);
  const btn = "grid size-7 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised";
  return (
    <div className="flex shrink-0 items-center gap-1 border-b border-slate-200 px-2 py-1 dark:border-border">
      <button type="button" className={btn} onClick={() => send({ t: "nav", tab: tab.id, to: "back" })} aria-label="Atrás"><ArrowLeft className="size-3.5" /></button>
      <button type="button" className={btn} onClick={() => send({ t: "nav", tab: tab.id, to: "forward" })} aria-label="Adelante"><ArrowRight className="size-3.5" /></button>
      <button type="button" className={btn} onClick={() => send({ t: "nav", tab: tab.id, to: "reload" })} aria-label="Recargar"><ArrowClockwise className="size-3.5" /></button>
      <form className="min-w-0 flex-1" onSubmit={(e) => { e.preventDefault(); setEditing(false); if (url.trim()) send({ t: "open", tab: tab.id, url: url.trim() }); }}>
        <input value={url} onChange={(e) => setUrl(e.target.value)} onFocus={(e) => { setEditing(true); e.target.select(); }} onBlur={() => setEditing(false)}
          placeholder="Escribe una dirección" spellCheck={false}
          className="h-7 w-full rounded-md border border-slate-200 bg-transparent px-2 text-xs outline-none focus:border-slate-400 dark:border-border" />
      </form>
      <button type="button" onClick={() => send({ t: "control", on: !userControl })}
        title={userControl ? "Devolver el control al agente" : "Tomar el control: el agente podrá leer y capturar, pero no actuar"}
        className={`flex h-7 shrink-0 items-center gap-1 rounded-md px-2 text-xs ${userControl ? "bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-200" : "text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised"}`}>
        {userControl ? <><Robot className="size-3.5" />Devolver</> : <><HandPointing className="size-3.5" />Tomar el control</>}
      </button>
    </div>
  );
}

const buttons = ["left", "middle", "right"] as const;
const mods = (e: { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }) => (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);

// Screencast draws the node's page and sends the user's input back.
function Screencast({ tab, frame, send }: { tab: string; frame: Frame | null; send: (m: Record<string, unknown>) => void }) {
  const img = useRef<HTMLImageElement>(null);
  const lastMove = useRef(0);
  const point = (e: { clientX: number; clientY: number }) => {
    const el = img.current;
    if (!el || !frame) return null;
    const r = el.getBoundingClientRect();
    return { x: ((e.clientX - r.left) / r.width) * frame.w, y: ((e.clientY - r.top) / r.height) * frame.h };
  };
  const input = (method: string, params: Record<string, unknown>) => send({ t: "input", tab, method, params });
  const onMouse = (type: string) => (e: MouseEvent) => {
    const p = point(e);
    if (!p) return;
    if (type === "mouseMoved") { const n = Date.now(); if (n - lastMove.current < 40) return; lastMove.current = n; }
    if (type === "mousePressed") (e.currentTarget as HTMLElement).focus();
    input("Input.dispatchMouseEvent", { type, ...p, button: type === "mouseMoved" ? "none" : buttons[e.button] ?? "left", clickCount: type === "mouseMoved" ? 0 : Math.max(1, e.detail), modifiers: mods(e) });
  };
  const onWheel = (e: WheelEvent) => { const p = point(e); if (p) input("Input.dispatchMouseEvent", { type: "mouseWheel", ...p, deltaX: e.deltaX, deltaY: e.deltaY, modifiers: mods(e) }); };
  const onKey = (type: "keyDown" | "keyUp") => (e: KeyboardEvent) => {
    e.preventDefault();
    const text = type === "keyDown" && e.key.length === 1 && !e.ctrlKey && !e.metaKey ? e.key : e.key === "Enter" && type === "keyDown" ? "\r" : undefined;
    input("Input.dispatchKeyEvent", { type: text ? "keyDown" : type === "keyDown" ? "rawKeyDown" : "keyUp", key: e.key, code: e.code, windowsVirtualKeyCode: e.keyCode, text, modifiers: mods(e) });
  };
  if (!frame) return <div className="grid h-full place-items-center text-sm text-slate-500 dark:text-muted">Cargando la página…</div>;
  return (
    <div tabIndex={0} className="grid h-full place-items-center outline-none" onKeyDown={onKey("keyDown")} onKeyUp={onKey("keyUp")} onContextMenu={(e) => e.preventDefault()}>
      <img ref={img} src={frame.src} alt="" draggable={false} className="max-h-full max-w-full select-none object-contain"
        onMouseDown={onMouse("mousePressed")} onMouseUp={onMouse("mouseReleased")} onMouseMove={onMouse("mouseMoved")} onWheel={onWheel} />
    </div>
  );
}
