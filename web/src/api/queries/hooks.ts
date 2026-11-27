// Hooks of the shared resources: one place with each key and its fetcher.
// They take the options of useQuery (staleTime, refetchInterval, select...) so
// each screen keeps its own refresh rhythm.
import { useMutation,useQuery,useQueryClient,type UseMutationOptions,type UseQueryOptions } from "@tanstack/react-query";
import { api,type NodeStatus } from "../client";
import { qk } from "./keys";

export type Opts<D, S = D> = Omit<UseQueryOptions<D, Error, S>, "queryKey" | "queryFn">;
export type Of<F extends (...a: never[]) => Promise<unknown>> = Awaited<ReturnType<F>>;
// MOpts are the options of a mutation hook: the screen keeps its own effects
// (onSuccess, onSettled...), the hook owns the call.
export type MOpts<D, V> = Omit<UseMutationOptions<D, Error, V>, "mutationFn">;

// useCall is the base of the mutation hooks: a call to the API with the
// screen's options.
export const useCall = <D, V = void>(fn: (v: V) => Promise<D>, o?: MOpts<D, V>) => useMutation({ mutationFn: fn, ...o });

export const useNode = <S = Of<typeof api.node>>(o?: Opts<Of<typeof api.node>, S>) =>
  useQuery({ queryKey: qk.node(), queryFn: api.node, ...o });
export const useMesh = <S = Of<typeof api.mesh>>(o?: Opts<Of<typeof api.mesh>, S>) =>
  useQuery({ queryKey: qk.mesh(), queryFn: api.mesh, ...o });
export const useProjects = (o?: Opts<Of<typeof api.projects.list>>) =>
  useQuery({ queryKey: qk.projects(), queryFn: () => api.projects.list(), ...o });
export const useRepositories = (o?: Opts<Of<typeof api.repositories.list>>) =>
  useQuery({ queryKey: qk.repositories(), queryFn: () => api.repositories.list(), ...o });
export const useRepositoryLocations = (o?: Opts<Of<typeof api.repositoryLocations>>) =>
  useQuery({ queryKey: qk.repositoryLocations(), queryFn: api.repositoryLocations, ...o });
export const useMemories = (o?: Opts<Of<typeof api.memories.list>>) =>
  useQuery({ queryKey: qk.memories(), queryFn: () => api.memories.list(), ...o });
export const useNodeSettings = (node: string, o?: Opts<Of<typeof api.nodeSettings>>) =>
  useQuery({ queryKey: qk.nodeSettings(node), queryFn: () => api.nodeSettings(node), ...o });
// useNodeProviders lists a node's harnesses. This node's come with /node, so
// they share that request and its cached answer instead of asking twice.
export function useNodeProviders<S = Of<typeof api.nodeProviders>>(node: string, o?: Opts<Of<typeof api.nodeProviders>, S>) {
  const qc = useQueryClient();
  const status = () => qc.getQueryData<NodeStatus>(qk.node());
  const self = node === "local" || node === status()?.node.id;
  return useQuery({
    queryKey: qk.providers(node),
    queryFn: self ? () => qc.fetchQuery({ queryKey: qk.node(), queryFn: api.node, staleTime: 5_000 }).then((n) => n.providers) : () => api.nodeProviders(node),
    initialData: self ? () => status()?.providers : undefined,
    initialDataUpdatedAt: self ? () => qc.getQueryState(qk.node())?.dataUpdatedAt : undefined,
    ...o,
  });
}
export const usePairedDevices = (o?: Opts<Of<typeof api.devices>>) =>
  useQuery({ queryKey: qk.devices(), queryFn: api.devices, ...o });
export const useBoard = (project = "", o?: Opts<Of<typeof api.board>>) =>
  useQuery({ queryKey: qk.board(project), queryFn: () => api.board(project || undefined), ...o });
export const useCard = (id: string | null | undefined, o?: Opts<Of<typeof api.card>>) =>
  useQuery({ queryKey: qk.card(id), queryFn: () => api.card(id!), ...o });
export const usePlan = (taskId: string | null | undefined, o?: Opts<Of<typeof api.plan>>) =>
  useQuery({ queryKey: qk.planDoc(taskId), queryFn: () => api.plan(taskId!), ...o });
export const useThreadView = (id: string | undefined, o?: Opts<Of<typeof api.thread>>) =>
  useQuery({ queryKey: qk.thread(id), queryFn: () => api.thread(id!), enabled: !!id, ...o });
