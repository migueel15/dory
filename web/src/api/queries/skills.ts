// Hooks of skills and their state on each node.
import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const useSkills = (o?: Opts<Of<typeof api.skills>>) => useQuery({ queryKey: qk.skills(), queryFn: api.skills, ...o });
export const useSkillStates = (o?: Opts<Of<typeof api.skillStates>>) => useQuery({ queryKey: qk.skillStates(), queryFn: api.skillStates, ...o });
export const useSkillConflicts = (node: string, id: string, o?: Opts<Of<typeof api.skillConflicts>>) =>
  useQuery({ queryKey: qk.skillConflicts(node, id), queryFn: () => api.skillConflicts(node, id), ...o });

export const useSetSkillEnabled = (o?: MOpts<Of<typeof api.setSkillEnabled>, { id: string; on: boolean }>) =>
  useCall(({ id, on }: { id: string; on: boolean }) => api.setSkillEnabled(id, on), o);
export const useSaveSkill = (skill: Parameters<typeof api.saveSkill>[0], o?: MOpts<Of<typeof api.saveSkill>, void>) => useCall(() => api.saveSkill(skill), o);
export const useArchiveSkill = (id: string | undefined, o?: MOpts<unknown, void>) => useCall(() => api.archiveSkill(id!), o);
export const useForkSkill = (id: string, name: string, o?: MOpts<Of<typeof api.forkSkill>, void>) => useCall(() => api.forkSkill(id, name), o);
export const useResolveSkill = (node: string, id: string, o?: MOpts<Of<typeof api.resolveSkill>, { action: "restore" | "adopt"; harness?: string }>) =>
  useCall(({ action, harness }: { action: "restore" | "adopt"; harness?: string }) => api.resolveSkill(node, id, action, harness), o);
