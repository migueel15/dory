import { get, post, del } from "./http";

export type User = { id: string; username: string };

export const authApi = {
  devices: () => get<{ id: string; name: string; created_at: string }[]>("/devices"),
  revokeDevice: (id: string) => del(`/devices/${id}`),
  login: (username: string, password: string) => post<User>("/auth/login", { username, password }),
  setupNeeded: () => get<{ needed: boolean }>("/auth/setup"),
  setup: (username: string, password: string) => post<User>("/auth/setup", { username, password }),
  logout: () => post<void>("/auth/logout"),
  me: () => get<User>("/auth/me"),
};
