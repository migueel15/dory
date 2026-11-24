import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, HandPointing, Circle, CircleNotch, Cpu, DeviceMobile, PictureInPicture, Plugs, Power, SidebarSimple, X, Sliders, Square, SpeakerHigh, SpeakerLow, WifiHigh } from "@phosphor-icons/react";
import { devicesApi, type Device, type DeviceList } from "../../api/client";
import { DeviceStream, Key, keyFor, webCodecsProblem, type StreamStatus } from "../../lib/scrcpy";
import { askConfirm } from "../../components/confirm";
import { Button, ErrorText, Input, Select, iconButton } from "../../components/ui";
import { qk } from "../../api/queries/keys";

// DevicePanel shows an Android device or emulator of the thread's node live.
// Only the visible tab streams: it connects on mount and closes on unmount.
// The chosen device is kept by the tab; if it drops, the tab waits for it.
export function DevicePanel({ node, serial, onSerial, floating = false, onFloat }: { node: string; serial: string | null; onSerial: (serial: string | null, name: string) => void; floating?: boolean; onFloat?: (v: boolean) => void }) {
  const list = useQuery({ queryKey: qk.devices(node), queryFn: () => devicesApi.list(node), refetchInterval: 4000 });
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
  const refresh = () => qc.invalidateQueries({ queryKey: qk.devices(node) });
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

// Stream quality: the picture is limited to maxSize pixels on its long side
// and to the bitrate; "Nativa" leaves the screen at its own resolution.
const QUALITY = {
  normal: { label: "Normal", maxSize: 1080, bitrate: 6_000_000 },
  high: { label: "Alta", maxSize: 1920, bitrate: 12_000_000 },
  native: { label: "Nativa", maxSize: 4096, bitrate: 20_000_000 },
} as const;
type Quality = keyof typeof QUALITY;
const readQuality = (): Quality => { try { const q = localStorage.getItem("dory.device-quality"); return q && q in QUALITY ? (q as Quality) : "high"; } catch { return "high"; } };

const statusText: Record<StreamStatus, string> = { connecting: "Conectando…", live: "En directo", closed: "Desconectado, reintentando…", error: "Error" };

function Live({ node, device, onBack, floating, onFloat }: { node: string; device: Device; onBack: () => void; floating: boolean; onFloat?: (v: boolean) => void }) {
  const [tools, setTools] = useState(false);
  const [quality, setQuality] = useState<Quality>(readQuality);
  const pickQuality = (q: Quality) => { setQuality(q); try { localStorage.setItem("dory.device-quality", q); } catch { /* per-browser only */ } };
  const canvas = useRef<HTMLCanvasElement>(null);
  const stream = useRef<DeviceStream | null>(null);
  const [status, setStatus] = useState<StreamStatus>("connecting");
  const [message, setMessage] = useState("");
  useEffect(() => {
    const s = new DeviceStream(canvas.current!, { node, serial: device.serial, maxSize: QUALITY[quality].maxSize, bitrate: QUALITY[quality].bitrate }, { status: (st, m) => { setStatus(st); setMessage(m ?? ""); } });
    stream.current = s;
    s.start();
    return () => { s.stop(); stream.current = null; };
  }, [node, device.serial, quality]);
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
        <Select className="w-24" value={quality} onChange={(v) => pickQuality(v as Quality)} options={(Object.keys(QUALITY) as Quality[]).map((q) => ({ value: q, label: QUALITY[q].label }))} />
        {onFloat && <button type="button" onClick={() => onFloat(!floating)} title={floating ? "Abrir en el panel lateral" : "Flotar sobre el chat"} aria-label={floating ? "Abrir en el panel lateral" : "Flotar sobre el chat"} className="text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">{floating ? <SidebarSimple className="size-4 -scale-x-100" /> : <PictureInPicture className="size-4" />}</button>}
      </div>
      {message && <div className="p-2"><ErrorText error={new Error(message)} /></div>}
      {/* The canvas fills the free space and object-contain fits the picture inside it, so the
          phone keeps its aspect ratio and never grows past the panel, whatever its height. */}
      <div className="relative min-h-0 flex-1 overflow-hidden bg-black/5 dark:bg-black/30">
        <canvas ref={canvas} tabIndex={0} {...input} className="absolute inset-2 size-[calc(100%-1rem)] touch-none bg-transparent object-contain outline-none" />
      </div>
      {tools && <Tools node={node} device={device} onClose={() => setTools(false)} />}
      <div className="flex shrink-0 items-center justify-center gap-1 border-t border-slate-200 p-1 dark:border-border">
        {bar("Atrás", <ArrowLeft className="size-4" />, (s) => s.back())}
        {bar("Inicio", <Circle className="size-4" />, (s) => s.keycode(Key.home))}
        {bar("Recientes", <Square className="size-4" />, (s) => s.keycode(Key.appSwitch))}
        {bar("Bloquear o encender", <Power className="size-4" />, (s) => s.keycode(Key.power))}
        {bar("Bajar volumen", <SpeakerLow className="size-4" />, (s) => s.keycode(Key.volDown))}
        {bar("Subir volumen", <SpeakerHigh className="size-4" />, (s) => s.keycode(Key.volUp))}
        <TouchStatus node={node} device={device} />
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

// Tools changes device settings through typed adb actions and shows the value
// the device reports back after each change; only what the device supports shows.
function Tools({ node, device, onClose }: { node: string; device: Device; onClose: () => void }) {
  const qc = useQueryClient();
  const key = qk.deviceState(node, device.serial);
  const state = useQuery({ queryKey: key, queryFn: () => devicesApi.action(device.serial, { action: "state" }, node), refetchInterval: 8000 });
  const apply = useMutation({
    mutationFn: (body: Record<string, unknown>) => devicesApi.action(device.serial, body, node),
    onSuccess: (out) => qc.setQueryData(key, (old: Record<string, string> | undefined) => ({ ...old, ...out })),
  });
  const st = state.data ?? {};
  const [pkg, setPkg] = useState("");
  const [perm, setPerm] = useState("android.permission.CAMERA");
  const [coords, setCoords] = useState("40.4168,-3.7038");
  const scale = st.font_scale ? Number(st.font_scale).toFixed(2) : "";
  const scales = ["0.85", "1.00", "1.15", "1.30", "1.50", "2.00"];
  const row = (name: string, control: React.ReactNode) => <div className="flex min-h-8 items-center justify-between gap-3"><span className="text-slate-600 dark:text-text-secondary">{name}</span>{control}</div>;
  const check = (action: string, value?: string) => (
    <input type="checkbox" aria-label={action} checked={value === "on"} disabled={!value || apply.isPending} onChange={(e) => apply.mutate({ action, value: e.target.checked ? "on" : "off" })} />
  );
  const heading = (text: string) => <h3 className="pt-1 text-xs font-medium text-slate-500 dark:text-muted">{text}</h3>;
  return (
    <section aria-label="Herramientas del dispositivo" className="max-h-72 shrink-0 space-y-2 overflow-auto border-t border-slate-200 bg-white p-3 text-sm dark:border-border dark:bg-surface">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">Herramientas</h2>
        <button type="button" onClick={onClose} aria-label="Cerrar herramientas" className={iconButton}><X weight="regular" className="size-4" /></button>
      </div>
      {st.foreground && <p className="truncate text-xs text-slate-500 dark:text-muted" title={st.foreground}>En primer plano: {st.foreground}</p>}
      {heading("Pantalla")}
      {row("Modo oscuro", <Select className="w-40" value={st.dark_mode ?? ""} onChange={(v) => apply.mutate({ action: "dark_mode", value: v })} options={[{ value: "yes", label: "Sí" }, { value: "no", label: "No" }, { value: "auto", label: "Automático" }]} />)}
      {row("Tamaño de texto", <Select className="w-40" value={scale} onChange={(v) => apply.mutate({ action: "font_scale", value: v })} options={[...(scale && !scales.includes(scale) ? [scale] : []), ...scales].map((v) => ({ value: v, label: `${v}×` }))} />)}
      {row("Orientación", <Select className="w-40" value={st.orientation ?? ""} onChange={(v) => apply.mutate({ action: "orientation", value: v })} options={[{ value: "auto", label: "Automática" }, { value: "portrait", label: "Vertical" }, { value: "landscape", label: "Horizontal" }, { value: "reverse_portrait", label: "Vertical invertida" }, { value: "reverse_landscape", label: "Horizontal invertida" }]} />)}
      {row("Encendida al cargar", check("stay_awake", st.stay_awake))}
      {heading("Conexión")}
      {row("Wi-Fi", check("wifi", st.wifi))}
      {row("Datos móviles", check("data", st.data))}
      {heading("Permisos de una app")}
      <div className="grid grid-cols-2 gap-2"><Input placeholder="com.paquete" value={pkg} onChange={(e) => setPkg(e.target.value)} /><Input value={perm} onChange={(e) => setPerm(e.target.value)} /></div>
      <div className="flex items-center gap-2">
        <Button variant="ghost" disabled={!pkg || apply.isPending} onClick={() => apply.mutate({ action: "permission", package: pkg.trim(), permission: perm.trim(), value: "grant" })}>Conceder</Button>
        <Button variant="ghost" disabled={!pkg || apply.isPending} onClick={() => apply.mutate({ action: "permission", package: pkg.trim(), permission: perm.trim(), value: "revoke" })}>Revocar</Button>
        {apply.data?.permission && <span className="text-xs text-slate-500 dark:text-muted">{apply.data.state === "granted" ? "Concedido" : "Revocado"}</span>}
      </div>
      {device.transport === "emulator" && (<>
        {heading("Ubicación simulada")}
        <div className="flex items-center gap-2"><Input placeholder="lat,lon" value={coords} onChange={(e) => setCoords(e.target.value)} />
          <Button variant="ghost" disabled={apply.isPending} onClick={() => { const [lat, lon] = coords.split(",").map(Number); apply.mutate({ action: "location", lat, lon }); }}>Fijar</Button></div>
        {apply.data?.location && <p className="text-xs text-slate-500 dark:text-muted">Ubicación: {apply.data.location}</p>}
      </>)}
      {apply.error && <ErrorText error={apply.error} />}
    </section>
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

const touchHelp = `Para controlar el móvil con el ratón y el teclado desde Dory, activa esta opción en el móvil:
1. Ajustes → Acerca del teléfono → toca 7 veces la versión del sistema para activar las Opciones de desarrollador.
2. Ajustes → Ajustes adicionales → Opciones de desarrollador.
3. Con el cable USB conectado, activa «Depuración USB».
4. Activa «Depuración USB (ajustes de seguridad)» y acepta el aviso (puede pedir cuenta Xiaomi).
5. Desconecta y vuelve a conectar la depuración, y recarga esta pestaña.`;

// TouchStatus shows whether the device lets Dory inject touches and keys: lit
// when it does, dimmed with the steps to enable it when it refuses.
function TouchStatus({ node, device }: { node: string; device: Device }) {
  const check = useQuery({ queryKey: qk.deviceInput(node, device.serial), queryFn: () => devicesApi.action(device.serial, { action: "input_check" }, node), refetchInterval: 6000 });
  const on = check.data?.input === "ok";
  const blocked = check.data?.input === "blocked";
  const title = on ? "Control táctil activado" : blocked ? touchHelp : check.isLoading ? "Comprobando el control táctil…" : "No se pudo comprobar el control táctil";
  return (
    <span title={title} aria-label={on ? "Control táctil activado" : "Control táctil desactivado"} role="img"
      className={`grid size-8 place-items-center rounded-md ${on ? "text-emerald-500" : "cursor-help text-slate-400 opacity-50 dark:text-muted"}`}>
      <HandPointing className="size-4" weight={on ? "fill" : "regular"} />
    </span>
  );
}
