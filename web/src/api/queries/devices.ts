// Hooks of the Android devices of a node.
import { useQuery } from "@tanstack/react-query";
import { devicesApi } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

type Action = Of<typeof devicesApi.action>;

export const useDeviceList = (node: string, o?: Opts<Of<typeof devicesApi.list>>) =>
  useQuery({ queryKey: qk.devices(node), queryFn: () => devicesApi.list(node), ...o });
// useDeviceAction reads the outcome of an action that only queries the
// device ("state", "input_check"), cached under key.
export const useDeviceQuery = (key: readonly unknown[], node: string, serial: string, action: string, o?: Opts<Action>) =>
  useQuery({ queryKey: key, queryFn: () => devicesApi.action(serial, { action }, node), ...o });
export const useDeviceAction = (node: string, serial: string, o?: MOpts<Action, Record<string, unknown>>) =>
  useCall((body: Record<string, unknown>) => devicesApi.action(serial, body, node), o);
