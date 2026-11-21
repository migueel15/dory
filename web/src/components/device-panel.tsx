import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Circle, CircleNotch, Cpu, DeviceMobile, Plugs, Power, Square, SpeakerHigh, SpeakerLow, WifiHigh } from "@phosphor-icons/react";
import { devicesApi, type Device, type DeviceList } from "../api/client";
import { DeviceStream, Key, keyFor, webCodecsProblem, type StreamStatus } from "../lib/scrcpy";
import { askConfirm } from "./confirm";
import { Button, ErrorText } from "./ui";

// DevicePanel shows an Android device or emulator of the thread's node live.
// Only the visible tab streams: it connects on mount and closes on unmount.
// The chosen device is kept by the tab; if it drops, the tab waits for it.
export function DevicePanel({ node, serial, onSerial }: { node: string; serial: string | null; onSerial: (serial: string | null, name: string) => void }) {
  const list = useQuery({ queryKey: ["devices", node], queryFn: () => devicesApi.list(node), refetchInterval: 4000 });
  const problem = webCodecsProblem();
  if (problem) return <Notice title="Vídeo no disponible">{problem}</Notice>;
  if (list.error) return <div className="p-4"><ErrorText error={list.error} /></div>;
  if (!list.data) return <div className="p-4 text-sm text-slate-500 dark:text-muted">Buscando dispositivos…</div>;
  if (!list.data.tools.adb) return <Setup list={list.data} />;
  const dev = list.data.devices.find((d) => d.serial === serial);
  if (serial && dev?.state === "device") return <Live key={serial} node={node} device={dev} onBack={() => onSerial(null, "")} />;
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

function Live({ node, device, onBack }: { node: string; device: Device; onBack: () => void }) {
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
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs dark:border-border">
        <button type="button" onClick={onBack} className="text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">Dispositivos</button>
        <span className="min-w-0 flex-1 truncate font-medium">{device.avd || device.model || device.serial}</span>
        <span className={status === "live" ? "text-emerald-500" : "text-slate-500 dark:text-muted"}>{statusText[status]}</span>
      </div>
      {message && <div className="p-2"><ErrorText error={new Error(message)} /></div>}
      <div className="grid min-h-0 flex-1 place-items-center bg-black/5 p-2 dark:bg-black/30">
        <canvas ref={canvas} tabIndex={0} {...input} className="max-h-full max-w-full touch-none rounded-lg bg-black object-contain shadow outline-none focus:ring-2 focus:ring-blue-500/60" />
      </div>
      <div className="flex shrink-0 items-center justify-center gap-1 border-t border-slate-200 p-1 dark:border-border">
        {bar("Atrás", <ArrowLeft className="size-4" />, (s) => s.back())}
        {bar("Inicio", <Circle className="size-4" />, (s) => s.keycode(Key.home))}
        {bar("Recientes", <Square className="size-4" />, (s) => s.keycode(Key.appSwitch))}
        {bar("Bloquear o encender", <Power className="size-4" />, (s) => s.keycode(Key.power))}
        {bar("Bajar volumen", <SpeakerLow className="size-4" />, (s) => s.keycode(Key.volDown))}
        {bar("Subir volumen", <SpeakerHigh className="size-4" />, (s) => s.keycode(Key.volUp))}
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
