import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DeviceMobile, Plugs, WifiHigh, Cpu } from "@phosphor-icons/react";
import { devicesApi, type Device, type DeviceList } from "../api/client";
import { DeviceStream, webCodecsProblem, type StreamStatus } from "../lib/scrcpy";
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
  const [status, setStatus] = useState<StreamStatus>("connecting");
  const [message, setMessage] = useState("");
  useEffect(() => {
    const stream = new DeviceStream(canvas.current!, { node, serial: device.serial }, { status: (s, m) => { setStatus(s); setMessage(m ?? ""); } });
    stream.start();
    return () => stream.stop();
  }, [node, device.serial]);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs dark:border-border">
        <button type="button" onClick={onBack} className="text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">Dispositivos</button>
        <span className="min-w-0 flex-1 truncate font-medium">{device.avd || device.model || device.serial}</span>
        <span className={status === "live" ? "text-emerald-500" : "text-slate-500 dark:text-muted"}>{statusText[status]}</span>
      </div>
      {message && <div className="p-2"><ErrorText error={new Error(message)} /></div>}
      <div className="grid min-h-0 flex-1 place-items-center bg-black/5 p-2 dark:bg-black/30">
        <canvas ref={canvas} className="max-h-full max-w-full rounded-lg bg-black object-contain shadow" />
      </div>
    </div>
  );
}

