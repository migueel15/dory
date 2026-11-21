import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Circle, CircleNotch, Cpu, DeviceMobile, PictureInPicture, Plugs, Power, SidebarSimple, Sliders, Square, SpeakerHigh, SpeakerLow, WifiHigh } from "@phosphor-icons/react";
import { devicesApi, type Device, type DeviceList } from "../api/client";
import { DeviceStream, Key, keyFor, webCodecsProblem, type StreamStatus } from "../lib/scrcpy";
import { askConfirm } from "./confirm";
import { Button, ErrorText } from "./ui";

// DevicePanel shows an Android device or emulator of the thread's node live.
// Only the visible tab streams: it connects on mount and closes on unmount.
// The chosen device is kept by the tab; if it drops, the tab waits for it.
export function DevicePanel({ node, serial, onSerial, floating = false, onFloat }: { node: string; serial: string | null; onSerial: (serial: string | null, name: string) => void; floating?: boolean; onFloat?: (v: boolean) => void }) {
  const list = useQuery({ queryKey: ["devices", node], queryFn: () => devicesApi.list(node), refetchInterval: 4000 });
  const problem = webCodecsProblem();
  if (problem) return <Notice title="Vídeo no disponible">{problem}</Notice>;
  if (list.error) return <div className="p-4"><ErrorText error={list.error} /></div>;
  if (!list.data) return <div className="p-4 text-sm text-slate-500 dark:text-muted">Buscando dispositivos…</div>;
  if (!list.data.tools.adb) return <Setup list={list.data} />;
  const dev = list.data.devices.find((d) => d.serial === serial);
  if (serial && dev?.state === "device") return <Live key={serial} node={node} device={dev} onBack={() => onSerial(null, "")} floating={floating} onFloat={onFloat} />;
  return <Picker node={node} list={list.data} lost={serial} onPick={(d) => onSerial(d.serial, d.avd || d.model || d.serial)} />;
}

function Notice({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="grid h-full place-items-center p-6"><div className="max-w-sm space-y-2 text-center text-sm">
      <DeviceMobile className="mx-auto size-8 text-slate-400" weight="regular" />
      <p className="font-medium">{title}</p><div className="text-slate-500 dark:text-muted">{children}</div>
    </div></div>
  );
}

function Setup({ list }: { list: DeviceList }) {
  return (
    <Notice title="Falta Android platform-tools">
      <p>Este nodo no tiene <code>adb</code>. Instala las platform-tools del SDK de Android y define <code>ANDROID_HOME</code> (se busca también en <code>~/Android/Sdk</code>).</p>
      <p className="mt-2">Para emuladores hacen falta además el paquete <code>emulator</code> y KVM{list.tools.kvm ? " (KVM disponible)" : " (KVM no disponible)"}.</p>
    </Notice>
  );
}

const transportIcon = (d: Device) => d.transport === "wifi" ? <WifiHigh className="size-4" /> : d.transport === "emulator" ? <Cpu className="size-4" /> : <Plugs className="size-4" />;

const label = (d: Device) => d.avd || d.model || d.serial;

function Picker({ node, list, lost, onPick }: { node: string; list: DeviceList; lost: string | null; onPick: (d: Device) => void }) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["devices", node] });
  const run = useMutation({ mutationFn: (f: () => Promise<unknown>) => f(), onSettled: refresh });
  const stopped = list.avds.filter((a) => !list.devices.some((d) => d.avd === a));
  const stop = async (d: Device) => {
    const emu = d.transport === "emulator";
    if (emu || await askConfirm(`¿Apagar ${label(d)}? El móvil se apagará y habrá que encenderlo a mano.`, "Apagar")) run.mutate(() => devicesApi.stop(d.serial, true, node));
  };
  return (
    <div className="space-y-4 p-4 text-sm">
      {lost && <p className="text-amber-600 dark:text-amber-400">El dispositivo de esta pestaña no está disponible; se recuperará solo cuando vuelva.</p>}
      <div className="space-y-1">
        {list.devices.length === 0 && <p className="text-slate-500 dark:text-muted">No hay dispositivos conectados. Activa la depuración USB o inalámbrica en el móvil, o arranca un emulador.</p>}
        {list.devices.map((d) => (
          <div key={d.serial} className="flex items-center gap-1 rounded-md border border-slate-200 pr-1 dark:border-border">
            <button type="button" disabled={d.state !== "device"} onClick={() => onPick(d)} className="flex min-w-0 flex-1 items-center gap-2 rounded-md px-3 py-2 text-left hover:bg-slate-50 disabled:opacity-50 dark:hover:bg-raised">
              {transportIcon(d)}
              <span className="min-w-0 flex-1"><span className="block truncate font-medium">{label(d)}</span><span className="block truncate text-xs text-slate-500 dark:text-muted">{d.state === "device" ? d.serial : d.state === "unauthorized" ? "Acepta la depuración en el móvil" : d.state}</span></span>
            </button>
            {d.transport === "wifi" && <button type="button" onClick={() => run.mutate(() => devicesApi.disconnect(d.serial, node))} className="rounded px-2 py-1 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">Desconectar</button>}
            {d.state === "device" && <button type="button" onClick={() => stop(d)} className="rounded px-2 py-1 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">Apagar</button>}
          </div>
        ))}
      </div>
      {stopped.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs font-medium text-slate-500 dark:text-muted">Emuladores</p>
          {stopped.map((a) => {
            const st = list.avd_states[a];
            const busy = st?.status === "starting" || st?.status === "booting";
            return (
              <div key={a} className="flex items-center gap-2 rounded-md border border-slate-200 px-3 py-2 dark:border-border">
                <Cpu className="size-4" /><span className="min-w-0 flex-1 truncate font-medium">{a}</span>
                {st?.status === "failed" && <span className="max-w-40 truncate text-xs text-red-500" title={st.error}>{st.error}</span>}
                {busy ? <span className="flex items-center gap-1 text-xs text-slate-500 dark:text-muted"><CircleNotch className="size-3.5 animate-spin" />{st.status === "starting" ? "Arrancando…" : "Iniciando Android…"}</span>
                  : <Button variant="ghost" onClick={() => run.mutate(() => devicesApi.startAvd(a, node))}>Iniciar</Button>}
              </div>
            );
          })}
        </div>
      )}
      {list.tools.emulator && !list.tools.kvm && <p className="text-xs text-amber-600 dark:text-amber-400">KVM no está disponible en este nodo: los emuladores no arrancarán.</p>}
      {run.error && <ErrorText error={run.error} />}
      <WifiForm node={node} onDone={refresh} />
    </div>
  );
}

// WifiForm pairs with and connects to a phone over wireless debugging.
function WifiForm({ node, onDone }: { node: string; onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [pairAddr, setPairAddr] = useState("");
  const [code, setCode] = useState("");
  const [addr, setAddr] = useState("");
  const [note, setNote] = useState("");
  const act = useMutation({ mutationFn: (f: () => Promise<{ message: string }>) => f(), onSuccess: (r) => { setNote(String(r.message ?? "")); onDone(); } });
  const field = "w-full rounded-md border border-slate-300 bg-transparent px-2 py-1 dark:border-border-strong";
  return (
    <div className="border-t border-slate-200 pt-3 dark:border-border">
      <button type="button" onClick={() => setOpen(!open)} className="font-medium">{open ? "▾" : "▸"} Conectar por Wi-Fi</button>
      {open && (
        <div className="mt-2 space-y-3">
          <ol className="list-decimal space-y-0.5 pl-4 text-xs text-slate-500 dark:text-muted">
            <li>En el móvil: Ajustes → Opciones de desarrollador → Depuración inalámbrica.</li>
            <li>Si es la primera vez, «Vincular dispositivo con código» y escribe aquí la dirección y el código.</li>
            <li>Después conecta con la dirección que aparece en la pantalla de Depuración inalámbrica.</li>
          </ol>
          <div className="space-y-1"><div className="flex gap-1"><input className={field} placeholder="IP:puerto de vinculación" value={pairAddr} onChange={(e) => setPairAddr(e.target.value)} /><input className={`${field} w-28`} placeholder="Código" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)} /></div>
            <Button variant="ghost" disabled={act.isPending || !pairAddr || !code} onClick={() => act.mutate(() => devicesApi.pair(pairAddr.trim(), code.trim(), node))}>Vincular</Button></div>
          <div className="space-y-1"><input className={field} placeholder="IP:puerto de conexión" value={addr} onChange={(e) => setAddr(e.target.value)} />
            <Button variant="ghost" disabled={act.isPending || !addr} onClick={() => act.mutate(() => devicesApi.connect(addr.trim(), node))}>Conectar</Button></div>
          {note && <p className="text-xs text-emerald-600 dark:text-emerald-400">{note}</p>}
          {act.error && <ErrorText error={act.error} />}
        </div>
      )}
    </div>
  );
}

const statusText: Record<StreamStatus, string> = { connecting: "Conectando…", live: "En directo", closed: "Desconectado, reintentando…", error: "Error" };

function Live({ node, device, onBack, floating, onFloat }: { node: string; device: Device; onBack: () => void; floating: boolean; onFloat?: (v: boolean) => void }) {
  const [tools, setTools] = useState(false);
  const canvas = useRef<HTMLCanvasElement>(null);
  const stream = useRef<DeviceStream | null>(null);
  const [status, setStatus] = useState<StreamStatus>("connecting");
  const [message, setMessage] = useState("");
  useEffect(() => {
    const s = new DeviceStream(canvas.current!, { node, serial: device.serial }, { status: (st, m) => { setStatus(st); setMessage(m ?? ""); } });
    stream.current = s;
    s.start();
    return () => { s.stop(); stream.current = null; };
  }, [node, device.serial]);
  const input = useDeviceInput(canvas, stream);
  const bar = (title: string, icon: React.ReactNode, run: (s: DeviceStream) => void) => (
    <button type="button" title={title} aria-label={title} onClick={() => stream.current && run(stream.current)}
      className="grid size-8 place-items-center rounded-md text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised">{icon}</button>
  );
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs dark:border-border">
        <button type="button" onClick={onBack} className="text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">Dispositivos</button>
        <span className="min-w-0 flex-1 truncate font-medium">{device.avd || device.model || device.serial}</span>
        <span className={status === "live" ? "text-emerald-500" : "text-slate-500 dark:text-muted"}>{statusText[status]}</span>
        {onFloat && <button type="button" onClick={() => onFloat(!floating)} title={floating ? "Abrir en el panel lateral" : "Flotar sobre el chat"} aria-label={floating ? "Abrir en el panel lateral" : "Flotar sobre el chat"} className="text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">{floating ? <SidebarSimple className="size-4 -scale-x-100" /> : <PictureInPicture className="size-4" />}</button>}
      </div>
      {message && <div className="p-2"><ErrorText error={new Error(message)} /></div>}
      {/* The canvas fills the free space and object-contain fits the picture inside it, so the
          phone keeps its aspect ratio and never grows past the panel, whatever its height. */}
      <div className="relative min-h-0 flex-1 overflow-hidden bg-black/5 dark:bg-black/30">
        <canvas ref={canvas} tabIndex={0} {...input} className="absolute inset-2 size-[calc(100%-1rem)] touch-none bg-transparent object-contain outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60" />
      </div>
      {device.transport !== "emulator" && <p className="shrink-0 px-3 pb-1 text-center text-[11px] text-slate-500 dark:text-muted">¿Se ve pero no responde al tocar? En Xiaomi/HyperOS, Opciones de desarrollador → activa «Depuración USB (ajustes de seguridad)».</p>}
      {tools && <Tools node={node} device={device} />}
      <div className="flex shrink-0 items-center justify-center gap-1 border-t border-slate-200 p-1 dark:border-border">
        {bar("Atrás", <ArrowLeft className="size-4" />, (s) => s.back())}
        {bar("Inicio", <Circle className="size-4" />, (s) => s.keycode(Key.home))}
        {bar("Recientes", <Square className="size-4" />, (s) => s.keycode(Key.appSwitch))}
        {bar("Bloquear o encender", <Power className="size-4" />, (s) => s.keycode(Key.power))}
        {bar("Bajar volumen", <SpeakerLow className="size-4" />, (s) => s.keycode(Key.volDown))}
        {bar("Subir volumen", <SpeakerHigh className="size-4" />, (s) => s.keycode(Key.volUp))}
        <button type="button" title="Herramientas" aria-label="Herramientas" aria-pressed={tools} onClick={() => setTools(!tools)}
          className={`grid size-8 place-items-center rounded-md text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised ${tools ? "bg-slate-100 dark:bg-raised" : ""}`}><Sliders className="size-4" /></button>
      </div>
    </div>
  );
}

// useDeviceInput turns pointer, wheel and keyboard events on the canvas into device input.
function useDeviceInput(canvas: React.RefObject<HTMLCanvasElement | null>, stream: React.RefObject<DeviceStream | null>) {
  const pointers = useRef(new Map<number, number>());
  // to device pixels, accounting for the letterbox of object-contain
  const at = (e: { clientX: number; clientY: number }) => {
    const c = canvas.current!, r = c.getBoundingClientRect();
    const k = Math.min(r.width / c.width, r.height / c.height);
    const ox = (r.width - c.width * k) / 2, oy = (r.height - c.height * k) / 2;
    return { x: Math.max(0, Math.min(c.width - 1, (e.clientX - r.left - ox) / k)), y: Math.max(0, Math.min(c.height - 1, (e.clientY - r.top - oy) / k)) };
  };
  const idOf = (e: React.PointerEvent) => pointers.current.get(e.pointerId);
  return {
    onPointerDown: (e: React.PointerEvent) => {
      e.currentTarget.setPointerCapture(e.pointerId);
      (e.currentTarget as HTMLElement).focus();
      let n = 0; while ([...pointers.current.values()].includes(n)) n++;
      pointers.current.set(e.pointerId, n);
      const p = at(e); stream.current?.touch(0, p.x, p.y, n);
    },
    onPointerMove: (e: React.PointerEvent) => {
      const id = idOf(e); if (id === undefined) return;
      const p = at(e); stream.current?.touch(2, p.x, p.y, id);
    },
    onPointerUp: (e: React.PointerEvent) => {
      const id = idOf(e); if (id === undefined) return;
      pointers.current.delete(e.pointerId);
      const p = at(e); stream.current?.touch(1, p.x, p.y, id);
    },
    onPointerCancel: (e: React.PointerEvent) => {
      const id = idOf(e); if (id === undefined) return;
      pointers.current.delete(e.pointerId);
      const p = at(e); stream.current?.touch(1, p.x, p.y, id);
    },
    onWheel: (e: React.WheelEvent) => {
      const p = at(e); stream.current?.scroll(p.x, p.y, -e.deltaX / 40, -e.deltaY / 40);
    },
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const code = keyFor(e.key);
      if (code) stream.current?.keycode(code);
      else if (e.key.length === 1) stream.current?.text(e.key);
      else return;
      e.preventDefault();
    },
  };
}

const select = "rounded-md border border-slate-300 bg-transparent px-1.5 py-0.5 text-xs dark:border-border-strong dark:bg-surface";

// Tools changes device settings through typed adb actions and shows the value
// the device reports back after each change; only what the device supports shows.
function Tools({ node, device }: { node: string; device: Device }) {
  const qc = useQueryClient();
  const key = ["device-state", node, device.serial];
  const state = useQuery({ queryKey: key, queryFn: () => devicesApi.action(device.serial, { action: "state" }, node), refetchInterval: 8000 });
  const apply = useMutation({
    mutationFn: (body: Record<string, unknown>) => devicesApi.action(device.serial, body, node),
    onSuccess: (out) => qc.setQueryData(key, (old: Record<string, string> | undefined) => ({ ...old, ...out })),
  });
  const st = state.data ?? {};
  const [pkg, setPkg] = useState("");
  const [perm, setPerm] = useState("android.permission.CAMERA");
  const [coords, setCoords] = useState("40.4168,-3.7038");
  const row = (name: string, control: React.ReactNode) => <div className="flex items-center justify-between gap-2"><span>{name}</span>{control}</div>;
  const toggle = (action: string, value?: string) => (
    <select className={select} value={value ?? ""} onChange={(e) => apply.mutate({ action, value: e.target.value })}><option value="" disabled>—</option><option value="on">Activado</option><option value="off">Desactivado</option></select>
  );
  return (
    <div className="max-h-64 shrink-0 space-y-1.5 overflow-auto border-t border-slate-200 p-3 text-xs dark:border-border">
      {st.foreground && row("App en primer plano", <span className="max-w-48 truncate text-slate-500 dark:text-muted" title={st.foreground}>{st.foreground}</span>)}
      {row("Modo oscuro", <select className={select} value={st.dark_mode ?? ""} onChange={(e) => apply.mutate({ action: "dark_mode", value: e.target.value })}><option value="" disabled>—</option><option value="yes">Sí</option><option value="no">No</option><option value="auto">Automático</option></select>)}
      {row("Tamaño de texto", <select className={select} value={st.font_scale ? Number(st.font_scale).toFixed(2) : ""} onChange={(e) => apply.mutate({ action: "font_scale", value: e.target.value })}>{["0.85", "1.00", "1.15", "1.30", "1.50", "2.00"].map((v) => <option key={v} value={v}>{v}×</option>)}{st.font_scale && !["0.85", "1.00", "1.15", "1.30", "1.50", "2.00"].includes(Number(st.font_scale).toFixed(2)) && <option value={Number(st.font_scale).toFixed(2)}>{Number(st.font_scale).toFixed(2)}×</option>}</select>)}
      {row("Orientación", <select className={select} value={st.orientation ?? ""} onChange={(e) => apply.mutate({ action: "orientation", value: e.target.value })}><option value="" disabled>—</option><option value="auto">Automática</option><option value="portrait">Vertical</option><option value="landscape">Horizontal</option><option value="reverse_portrait">Vertical invertida</option><option value="reverse_landscape">Horizontal invertida</option></select>)}
      {row("Wi-Fi", toggle("wifi", st.wifi))}
      {row("Datos móviles", toggle("data", st.data))}
      <div className="space-y-1 border-t border-slate-200 pt-1.5 dark:border-border">
        <div className="font-medium">Permisos de una app</div>
        <div className="flex gap-1"><input className={`${select} min-w-0 flex-1`} placeholder="com.paquete" value={pkg} onChange={(e) => setPkg(e.target.value)} /><input className={`${select} min-w-0 flex-1`} value={perm} onChange={(e) => setPerm(e.target.value)} /></div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" disabled={!pkg || apply.isPending} onClick={() => apply.mutate({ action: "permission", package: pkg.trim(), permission: perm.trim(), value: "grant" })}>Conceder</Button>
          <Button variant="ghost" disabled={!pkg || apply.isPending} onClick={() => apply.mutate({ action: "permission", package: pkg.trim(), permission: perm.trim(), value: "revoke" })}>Revocar</Button>
          {apply.data?.permission && <span className="text-slate-500 dark:text-muted">{apply.data.state === "granted" ? "concedido" : "revocado"}</span>}
        </div>
      </div>
      {device.transport === "emulator" && (
        <div className="space-y-1 border-t border-slate-200 pt-1.5 dark:border-border">
          <div className="font-medium">Ubicación simulada</div>
          <div className="flex items-center gap-1"><input className={`${select} min-w-0 flex-1`} value={coords} onChange={(e) => setCoords(e.target.value)} placeholder="lat,lon" />
            <Button variant="ghost" disabled={apply.isPending} onClick={() => { const [lat, lon] = coords.split(",").map(Number); apply.mutate({ action: "location", lat, lon }); }}>Fijar</Button></div>
          {apply.data?.location && <span className="text-slate-500 dark:text-muted">Ubicación: {apply.data.location}</span>}
        </div>
      )}
      {apply.error && <ErrorText error={apply.error} />}
    </div>
  );
}

// FloatingWindow holds a device above the chat: drag it by its bar and
// resize it by the corner.
export function FloatingWindow({ id, children }: { id: string; children: React.ReactNode }) {
  const storeKey = `dory.device-float:${id}`;
  const [pos, setPos] = useState<{ x: number; y: number }>(() => { try { return JSON.parse(localStorage.getItem(storeKey) ?? "") ; } catch { return { x: Math.max(16, window.innerWidth - 400), y: 80 }; } });
  const drag = useRef<{ dx: number; dy: number } | null>(null);
  const clamp = (x: number, y: number) => ({ x: Math.max(0, Math.min(window.innerWidth - 80, x)), y: Math.max(0, Math.min(window.innerHeight - 40, y)) });
  return (
    <div className="fixed z-40 flex min-h-[320px] min-w-[240px] resize flex-col overflow-hidden rounded-xl border border-slate-300 bg-white shadow-2xl dark:border-border-strong dark:bg-surface"
      style={{ left: pos.x, top: pos.y, width: 340, height: 640, maxWidth: "95vw", maxHeight: "92vh" }}>
      <div className="h-3 shrink-0 cursor-move touch-none bg-slate-100 dark:bg-raised" title="Mover"
        onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); drag.current = { dx: e.clientX - pos.x, dy: e.clientY - pos.y }; }}
        onPointerMove={(e) => { if (drag.current) setPos(clamp(e.clientX - drag.current.dx, e.clientY - drag.current.dy)); }}
        onPointerUp={() => { drag.current = null; try { localStorage.setItem(storeKey, JSON.stringify(pos)); } catch { /* per-browser only */ } }} />
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}
