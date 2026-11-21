import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Circle, Cpu, DeviceMobile, Plugs, Power, Square, SpeakerHigh, SpeakerLow, WifiHigh } from "@phosphor-icons/react";
import { devicesApi, type Device, type DeviceList } from "../api/client";
import { DeviceStream, Key, keyFor, webCodecsProblem, type StreamStatus } from "../lib/scrcpy";
import { ErrorText } from "./ui";

// DevicePanel shows an Android device or emulator of the thread's node live.
// Only the visible tab streams: it connects on mount and closes on unmount.
export function DevicePanel({ node }: { node: string }) {
  const [serial, setSerial] = useState<string | null>(() => { try { return localStorage.getItem("dory.device-serial"); } catch { return null; } });
  const pick = (s: string | null) => { setSerial(s); try { if (s) localStorage.setItem("dory.device-serial", s); else localStorage.removeItem("dory.device-serial"); } catch { /* per-browser only */ } };
  const list = useQuery({ queryKey: ["devices", node], queryFn: () => devicesApi.list(node), refetchInterval: serial ? false : 4000 });
  const problem = webCodecsProblem();
  if (problem) return <Notice title="Vídeo no disponible">{problem}</Notice>;
  if (list.error) return <div className="p-4"><ErrorText error={list.error} /></div>;
  if (!list.data) return <div className="p-4 text-sm text-slate-500 dark:text-muted">Buscando dispositivos…</div>;
  if (!list.data.tools.adb) return <Setup list={list.data} />;
  const dev = list.data.devices.find((d) => d.serial === serial);
  if (serial && dev?.state === "device") return <Live key={serial} node={node} device={dev} onBack={() => pick(null)} />;
  return <Picker list={list.data} selected={serial} onPick={pick} />;
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

function Picker({ list, selected, onPick }: { list: DeviceList; selected: string | null; onPick: (s: string) => void }) {
  return (
    <div className="space-y-4 p-4 text-sm">
      {selected && <p className="text-slate-500 dark:text-muted">El dispositivo seleccionado no está disponible.</p>}
      <div className="space-y-1">
        {list.devices.length === 0 && <p className="text-slate-500 dark:text-muted">No hay dispositivos conectados. Activa la depuración USB o inalámbrica en el móvil, o arranca un emulador.</p>}
        {list.devices.map((d) => (
          <button key={d.serial} type="button" disabled={d.state !== "device"} onClick={() => onPick(d.serial)}
            className="flex w-full items-center gap-2 rounded-md border border-slate-200 px-3 py-2 text-left hover:bg-slate-50 disabled:opacity-50 dark:border-border dark:hover:bg-raised">
            {transportIcon(d)}
            <span className="min-w-0 flex-1"><span className="block truncate font-medium">{d.avd || d.model || d.serial}</span><span className="block truncate text-xs text-slate-500 dark:text-muted">{d.state === "device" ? d.serial : d.state === "unauthorized" ? "Acepta la depuración en el móvil" : d.state}</span></span>
          </button>
        ))}
      </div>
      {list.avds.some((a) => !list.devices.some((d) => d.avd === a)) && (
        <p className="text-xs text-slate-500 dark:text-muted">Emuladores sin arrancar: {list.avds.filter((a) => !list.devices.some((d) => d.avd === a)).join(", ")}.</p>
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
