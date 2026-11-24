// Hooks of the session: setup, login and the node version.
import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const useSetupNeeded = (o?: Opts<Of<typeof api.setupNeeded>>) =>
  useQuery({ queryKey: qk.setup(), queryFn: api.setupNeeded, ...o });
export const useVersion = (o?: Opts<Of<typeof api.version>>) =>
  useQuery({ queryKey: qk.version(), queryFn: api.version, ...o });
export const useMe = (o?: Opts<Of<typeof api.me>>) =>
  useQuery({ queryKey: qk.me(), queryFn: api.me, ...o });

// useLogin creates the first account of a new node, or signs in.
export const useLogin = (first: boolean, username: string, password: string, o?: MOpts<Of<typeof api.login>, void>) =>
  useCall(() => (first ? api.setup(username, password) : api.login(username, password)), o);
export const useLogout = (o?: MOpts<Of<typeof api.logout>, void>) => useCall(() => api.logout(), o);
