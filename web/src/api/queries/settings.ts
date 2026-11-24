// Hooks of the node settings: harnesses and accounts, the mesh, notices,
// paired devices, dictation and the title model.
import { useQuery } from "@tanstack/react-query";
import { api,req } from "../client";
import type { Notice } from "../types.gen";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const useTitleModel = (o?: Opts<Of<typeof api.titleModel>>) =>
  useQuery({ queryKey: qk.titleModel(), queryFn: api.titleModel, ...o });
export const useJob = (node: string, job: string | undefined, o?: Opts<Of<typeof api.job>>) =>
  useQuery({ queryKey: qk.job(job), queryFn: () => api.job(node, job!), ...o });
export const useMeshUpdates = (o?: Opts<Of<typeof api.meshUpdates>>) =>
  useQuery({ queryKey: qk.meshUpdate(), queryFn: () => api.meshUpdates(), ...o });

export type NoticeSetting = { kind: string; title: string; mode: string };
export const useNoticeSettings = (o?: Opts<NoticeSetting[]>) =>
  useQuery({ queryKey: qk.noticeSettings(), queryFn: () => req<NoticeSetting[]>("GET", "/notice-settings"), ...o });
export const useNotices = (o?: Opts<Notice[]>) =>
  useQuery({ queryKey: qk.notices(), queryFn: () => req<Notice[]>("GET", "/notices"), ...o });

export const useSaveNoticeSetting = (o?: MOpts<unknown, { kind: string; mode: string }>) =>
  useCall((s: { kind: string; mode: string }) => req("PUT", "/notice-settings", s), o);
export const useReadNotices = (o?: MOpts<unknown, string[]>) => useCall((ids: string[]) => req("POST", "/notices/read", { ids }), o);
export const useSetTitleModel = (o?: MOpts<Of<typeof api.setTitleModel>, Parameters<typeof api.setTitleModel>[0]>) =>
  useCall((body: Parameters<typeof api.setTitleModel>[0]) => api.setTitleModel(body), o);
export const useSetProviderEnabled = (node: string, o?: MOpts<void, { id: string; enabled: boolean }>) =>
  useCall(({ id, enabled }: { id: string; enabled: boolean }) => api.setProviderEnabled(node, id, enabled), o);
export const useProviderJob = (node: string, provider: string, o?: MOpts<Of<typeof api.providerJob>, Parameters<typeof api.providerJob>[2]>) =>
  useCall((kind: Parameters<typeof api.providerJob>[2]) => api.providerJob(node, provider, kind), o);
export const useAddAccount = (node: string, account: Parameters<typeof api.addAccount>[1], o?: MOpts<Of<typeof api.addAccount>, void>) =>
  useCall(() => api.addAccount(node, account), o);
export const useUpdateAccount = (node: string, id: string, account: Parameters<typeof api.updateAccount>[2], o?: MOpts<void, void>) =>
  useCall(() => api.updateAccount(node, id, account), o);
export const useRemoveAccount = (node: string, id: string, o?: MOpts<unknown, void>) => useCall(() => api.removeAccount(node, id), o);
export const useCheckMeshUpdates = (o?: MOpts<Of<typeof api.meshUpdates>, void>) => useCall(() => api.meshUpdates(true), o);
export const useUpdateMesh = (opts: Parameters<typeof api.updateMesh>[0], o?: MOpts<Of<typeof api.updateMesh>, void>) =>
  useCall(() => api.updateMesh(opts), o);
export const useUpdateNode = (node: string, providers: boolean, o?: MOpts<Of<typeof api.updateNode>, boolean>) =>
  useCall((force: boolean) => api.updateNode(node, { providers, force }), o);
export const useSetPeerEnabled = (o?: MOpts<void, { id: string; enabled: boolean }>) =>
  useCall(({ id, enabled }: { id: string; enabled: boolean }) => api.setPeerEnabled(id, enabled), o);
export const useRevokePeer = (o?: MOpts<unknown, string>) => useCall((id: string) => api.revokePeer(id), o);
export const usePairCode = (o?: MOpts<Of<typeof api.pairCode>, void>) => useCall(() => api.pairCode(), o);
export const usePair = (address: string, code: string, o?: MOpts<Of<typeof api.pair>, void>) => useCall(() => api.pair(address, code), o);
export const useRevokeDevice = (o?: MOpts<unknown, string>) => useCall((id: string) => api.revokeDevice(id), o);
export const useSetNodeIcon = (node: string, o?: MOpts<void, string>) => useCall((id: string) => api.setNodeIcon(node, id), o);
