import { get, post, put, del } from "./http";
import type { Job, Provider } from "./types.gen";

export const providersApi = {
  nodeProviders: (node: string) => get<Provider[]>(`/nodes/${node}/providers`),
  addAccount: (node: string, a: { driver: string; name: string; home: string }) => post<{ id: string }>(`/nodes/${node}/accounts`, a),
  updateAccount: (node: string, id: string, a: { name: string; home: string }) => put<void>(`/nodes/${node}/accounts/${id}`, a),
  removeAccount: (node: string, id: string) => del(`/nodes/${node}/accounts/${id}`),
  setProviderEnabled: (node: string, provider: string, enabled: boolean) => put<void>(`/nodes/${node}/providers/${provider}`, { enabled }),
  providerJob: (node: string, provider: string, kind: Job["kind"]) => post<Job>(`/nodes/${node}/providers/${provider}/${kind}`),
  job: (node: string, id: string) => get<Job>(`/nodes/${node}/jobs/${id}`),
  submitLoginCode: (node: string, id: string, code: string) => post<void>(`/nodes/${node}/jobs/${id}/code`, { code }),
};
