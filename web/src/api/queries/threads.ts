// Hooks of threads: lists, related threads, services, diffs and actions.
import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

type ThreadList = Of<typeof api.threads>;

export const useThreadsAttention = (o?: Opts<ThreadList>) =>
  useQuery({ queryKey: qk.threadsAttention(), queryFn: () => api.threads(), ...o });
export const useThreadList = (filter: string, o?: Opts<ThreadList>) =>
  useQuery({ queryKey: qk.threads(filter), queryFn: () => api.threads(undefined, filter === "archived"), ...o });
export const useThreadChildren = (id: string, o?: Opts<ThreadList>) =>
  useQuery({ queryKey: qk.threadChildren(id), queryFn: () => api.threads({ parent_id: id }, true), ...o });
export const useThreadRelated = (id: string, o?: Opts<ThreadList>) =>
  useQuery({ queryKey: qk.threadRelated(id), queryFn: () => api.threads({ origin_id: id }), ...o });
// useThreadSummary reads one thread, archived or not, or null when it is gone.
export const useThreadSummary = (id: string, o?: Opts<ThreadList[number] | null>) =>
  useQuery({ queryKey: qk.threadTitle(id), queryFn: () => api.threads({ id }, true).then((l) => l[0] ?? null), ...o });
export const useThreadServices = (id: string, o?: Opts<Of<typeof api.threadServices>>) =>
  useQuery({ queryKey: qk.threadServices(id), queryFn: () => api.threadServices(id), ...o });
export const useThreadDiff = (id: string, base: Parameters<typeof api.threadDiff>[1], o?: Opts<Of<typeof api.threadDiff>>) =>
  useQuery({ queryKey: qk.threadDiff(id, base), queryFn: () => api.threadDiff(id, base), ...o });
export const useExternalSession = (node: string, provider: string, id: string, o?: Opts<Of<typeof api.externalSession>>) =>
  useQuery({ queryKey: qk.external(node, provider, id), queryFn: () => api.externalSession(node, provider, id), ...o });
export const useExternalSessions = (node: string, o?: Opts<Of<typeof api.externalSessions>>) =>
  useQuery({ queryKey: qk.external(node), queryFn: () => api.externalSessions(node), ...o });
export const useUsageTurns = (node: string, thread: string, range: Parameters<typeof api.threadUsageTurns>[1], o?: Opts<Of<typeof api.threadUsageTurns>>) =>
  useQuery({ queryKey: qk.usageTurns(node, thread, range), queryFn: () => api.threadUsageTurns(thread, range, node), ...o });
export const useKeptWorktrees = (o?: Opts<Of<typeof api.keptWorktrees>>) =>
  useQuery({ queryKey: qk.keptWorktrees(), queryFn: () => api.keptWorktrees(), ...o });
export const useProjectWorktrees = (node: string, project: string | undefined, o?: Opts<Of<typeof api.projectWorktrees>>) =>
  useQuery({ queryKey: qk.projectWorktrees(node, project), queryFn: () => api.projectWorktrees(node, project!), ...o });
export const useProjectBranches = (node: string, project: string | undefined, o?: Opts<Of<typeof api.projectBranches>>) =>
  useQuery({ queryKey: qk.projectBranches(node, project), queryFn: () => api.projectBranches(node, project!), ...o });

export const useInterrupt = (o?: MOpts<Of<typeof api.interrupt>, string>) => useCall((id: string) => api.interrupt(id), o);
export const useStopThreadService = (thread: string, o?: MOpts<Of<typeof api.stopThreadService>, number>) =>
  useCall((pid: number) => api.stopThreadService(thread, pid), o);
export const useThreadAction = (thread: string, o?: MOpts<Of<typeof api.threadAction>, Parameters<typeof api.threadAction>[1]>) =>
  useCall((body: Parameters<typeof api.threadAction>[1]) => api.threadAction(thread, body), o);
export const useAdoptSession = (node: string, provider: string, id: string, o?: MOpts<Of<typeof api.adoptSession>, void>) =>
  useCall(() => api.adoptSession(node, provider, id), o);
export const useRemoveThreadWorktree = (o?: MOpts<Of<typeof api.removeThreadWorktree>, string>) =>
  useCall((id: string) => api.removeThreadWorktree(id), o);
export const useQueueAction = (thread: string, o?: MOpts<Of<typeof api.queue>, { run: string; action: "edit" | "cancel" | "steer"; text?: string }>) =>
  useCall(({ run, action, text }: { run: string; action: "edit" | "cancel" | "steer"; text?: string }) => api.queue(thread, run, action, text), o);
export const useSendMessage = <V extends { message: string; fileIds?: string[] }>(thread: string, o?: MOpts<void, V>) =>
  useCall((v: V) => api.send(thread, v.message, v.fileIds), o);
export const useSendText = (thread: string, text: string, o?: MOpts<void, void>) => useCall(() => api.send(thread, text), o);
// useAnswerQuestion answers a question of the harness, or, for Dory's own
// ask_user, sends the answers as a message.
export const useAnswerQuestion = (thread: string, request: string | undefined, asText: (answers: string[][]) => string, o?: MOpts<void, { answers: string[][]; files: string[][] }>) =>
  useCall(({ answers, files }: { answers: string[][]; files: string[][] }) => (request ? api.answer(thread, request, answers, files) : api.send(thread, asText(answers), files.flat())), o);
export const useAnswerSecret = (thread: string, request: string | undefined, o?: MOpts<Of<typeof api.answerSecret>, Parameters<typeof api.answerSecret>[2]>) =>
  useCall((body: Parameters<typeof api.answerSecret>[2]) => api.answerSecret(thread, request!, body), o);
export const useThreadSettings = (thread: string, o?: MOpts<Of<typeof api.threadSettings>, Parameters<typeof api.threadSettings>[1]>) =>
  useCall((body: Parameters<typeof api.threadSettings>[1]) => api.threadSettings(thread, body), o);
export const useCreateThread = <V = void>(body: (v: V) => Parameters<typeof api.createThread>[0], o?: MOpts<Of<typeof api.createThread>, V>) =>
  useCall((v: V) => api.createThread(body(v)), o);
export const useInterruptThread = (thread: string, o?: MOpts<Of<typeof api.interrupt>, void>) => useCall(() => api.interrupt(thread), o);
