// Hooks of tasks: plans, dependencies and the actions on a task.
import { useQuery } from "@tanstack/react-query";
import { api,type Stage } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const usePlans = (task: string, o?: Opts<Of<typeof api.plans>>) =>
  useQuery({ queryKey: qk.plans(task), queryFn: () => api.plans(task), ...o });
export const useDependencies = (task: string, o?: Opts<Of<typeof api.dependencies>>) =>
  useQuery({ queryKey: qk.dependencies(task), queryFn: () => api.dependencies(task), ...o });

type Task = Of<typeof api.tasks.update>;
export const useUpdateTask = <V = void>(id: string, patch: (v: V) => Parameters<typeof api.tasks.update>[1], o?: MOpts<Task, V>) =>
  useCall((v: V) => api.tasks.update(id, patch(v)), o);
export const useTaskStage = (id: string, o?: MOpts<Task, Stage>) => useUpdateTask(id, (stage: Stage) => ({ stage }), o);
export const useMoveTask = (o?: MOpts<Task, { id: string; stage: Stage }>) =>
  useCall(({ id, stage }: { id: string; stage: Stage }) => api.tasks.update(id, { stage }), o);
export const useArchiveTask = (id: string, o?: MOpts<Of<typeof api.tasks.archive>, void>) => useCall(() => api.tasks.archive(id), o);
export const useTaskDependency = (id: string, o?: MOpts<void, { other: string; add: boolean }>) =>
  useCall(({ other, add }: { other: string; add: boolean }) => (add ? api.addDependency(id, other) : api.removeDependency(id, other)).then(() => undefined), o);
export const useLaunchTask = <V = void>(id: string, body: (v: V) => Parameters<typeof api.launch>[1], o?: MOpts<Of<typeof api.launch>, V>) =>
  useCall((v: V) => api.launch(id, body(v)), o);
export const useAcceptPlan = (task: string, o?: MOpts<Of<typeof api.acceptPlan>, void>) => useCall(() => api.acceptPlan(task), o);
export const useIssueTask = (repo: string, number: number, body: Parameters<typeof api.issueTask>[2], o?: MOpts<Of<typeof api.issueTask>, void>) =>
  useCall(() => api.issueTask(repo, number, body), o);
export const useCreateIdea = <V extends { id: string; raw: string; files?: File[] }>(project: string, o?: MOpts<Of<typeof api.createIdea>, V>) =>
  useCall(async ({ id, raw, files }: V) => {
    const attachments = files?.length ? await Promise.all(files.map((file) => api.uploadTaskAttachment(id, file))) : undefined;
    return api.createIdea({ id, text: raw, project_id: project || null, attachments });
  }, o);
