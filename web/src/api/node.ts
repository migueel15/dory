import { get, post, put, del } from "./http";
import type { KeptWorktree, NodeDefaults, NodeSettings, TitleModel, UsageWindow, Version, Provider } from "./types.gen";
import type { NodeInfo, Peer } from "./mesh";

export type NodeStatus = { node: NodeInfo; providers: Provider[]; usage: Record<string, UsageWindow[]>; peers?: Peer[] };

export const nodeApi = {
  version: () => get<Version>("/version"),
  folders: (node: string, path = "", hidden = false) => get<{ path: string; parent: string; home: string; folders: { name: string; path: string; git: boolean }[] }>(`/nodes/${node}/folders?${new URLSearchParams({ path, ...(hidden && { hidden: "true" }) })}`),
  checkout: (node: string, path: string) => get<{ path: string; name: string; git: boolean; remote: string; default_branch: string }>(`/nodes/${node}/checkout?path=${encodeURIComponent(path)}`),
  setNodeRepositoryPath: (node: string, id: string, path: string) => put<void>(`/nodes/${node}/repositories/${id}/path`, { path }),
  setNodeIcon: (node: string, icon: string) => put<void>(`/nodes/${node}/icon`, { icon }),
  node: () => get<NodeStatus>("/node"),
  nodeDefaults: (node: string) => get<NodeDefaults>(`/nodes/${node}/defaults`),
  nodeSettings: (node = "local") => get<NodeSettings>(`/nodes/${node}/settings`),
  setNodeSettings: (settings: NodeSettings, node = "local") => put<NodeSettings>(`/nodes/${node}/settings`, settings),
  keptWorktrees: (node = "local") => get<KeptWorktree[]>(`/nodes/${node}/kept-worktrees`),
  titleModel: () => get<TitleModel>("/title-model"),
  setTitleModel: (body: TitleModel) => put<TitleModel>("/title-model", body),
  removeThreadWorktree: (id: string) => del(`/threads/${id}/worktree`),
};
