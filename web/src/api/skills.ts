import { get, post, put, del } from "./http";
import type { Skill, SkillConflict, SkillState } from "./types.gen";

export const skillsApi = {
  skills: () => get<Skill[]>("/skills"),
  skillStates: () => get<SkillState[]>("/skills/states"),
  saveSkill: (s: Partial<Skill>) => (s.id ? put<Skill>(`/skills/${s.id}`, s) : post<Skill>("/skills", s)),
  archiveSkill: (id: string) => del(`/skills/${id}`),
  setSkillEnabled: (id: string, enabled: boolean) => put<Skill>(`/skills/${id}/enabled`, { enabled }),
  skillConflicts: (node: string, id: string) => get<SkillConflict[]>(`/nodes/${node}/skills/${id}/conflicts`),
  resolveSkill: (node: string, id: string, action: "restore" | "adopt", harness = "") => post<{ backup: string }>(`/nodes/${node}/skills/${id}/resolve`, { action, harness }),
  forkSkill: (id: string, name?: string) => post<Skill>(`/skills/${id}/fork`, { name: name ?? "" }),
};
