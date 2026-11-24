import { get, post, put, del } from "./http";
import type { NodeUpdate, Provider, UpdateStatus } from "./types.gen";

export type NodeInfo = { id: string; name: string; os: string; version: string; data_dir: string; listen: string; platform: string; icon?: string };
export type Peer = { id: string; name: string; address: string; fingerprint: string; connected: boolean; last_seen: string; revoked: boolean; disabled?: boolean; protocol: number; incompatible: boolean; info: { node?: NodeInfo; providers?: Provider[] } };
export type Mesh = { node_id: string; fingerprint: string; port: number; peers: Peer[]; discovered: { id: string; name: string; address: string }[] };

export const meshApi = {
  mesh: () => get<Mesh>("/mesh"),
  meshUpdates: (fresh = false) => get<NodeUpdate[]>(fresh ? "/mesh/update?fresh=1" : "/mesh/update"),
  updateMesh: (opts: { providers: boolean; force: boolean }) => post<NodeUpdate[]>("/mesh/update", opts),
  updateNode: (node: string, opts: { providers: boolean; force: boolean }) => post<UpdateStatus>(`/nodes/${node}/update`, opts),
  pairCode: () => post<{ code: string; expires_at: string; fingerprint: string; port: number }>("/mesh/pair/code"),
  pair: (address: string, code: string) => post<Peer>("/mesh/pair", { address, code }),
  revokePeer: (id: string) => del(`/mesh/peers/${id}`),
  setPeerEnabled: (id: string, enabled: boolean) => put<void>(`/mesh/peers/${id}/enabled`, { enabled }),
};
