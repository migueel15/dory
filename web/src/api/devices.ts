// Android devices and emulators of a node.
import { get, post } from "./http";

export type DeviceTools = { adb: string; emulator: string; kvm: boolean; sdk: string };
export type Device = { serial: string; state: string; model: string; product: string; transport: "usb" | "wifi" | "emulator"; avd?: string; streaming: boolean };
export type AvdState = { status: "starting" | "booting" | "ready" | "failed"; code?: string; error?: string };
export type DeviceList = { tools: DeviceTools; devices: Device[]; avds: string[]; avd_states: Record<string, AvdState>; error?: string };
export const devicesApi = {
  list: (node = "local") => get<DeviceList>(`/nodes/${node}/devices`),
  startAvd: (name: string, node = "local") => post<{ ok: boolean }>(`/nodes/${node}/devices/avd/${encodeURIComponent(name)}/start`),
  stop: (serial: string, confirm: boolean, node = "local") => post<{ ok: boolean }>(`/nodes/${node}/devices/${encodeURIComponent(serial)}/stop`, { confirm }),
  pair: (address: string, code: string, node = "local") => post<{ message: string }>(`/nodes/${node}/devices/pair`, { address, code }),
  connect: (address: string, node = "local") => post<{ message: string }>(`/nodes/${node}/devices/connect`, { address }),
  action: (serial: string, body: Record<string, unknown>, node = "local") => post<Record<string, string>>(`/nodes/${node}/devices/${encodeURIComponent(serial)}/action`, body),
  disconnect: (serial: string, node = "local") => post<{ ok: boolean }>(`/nodes/${node}/devices/${encodeURIComponent(serial)}/disconnect`),
};
