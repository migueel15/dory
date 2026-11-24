import { get, post, put } from "./http";
import type { Plan, Task } from "./types.gen";

export const plansApi = {
  plans: (id: string) => get<Plan[]>(`/tasks/${id}/plans`),
  // The plan document of a task: one Markdown text with the revision it was read at.
  plan: (id: string) => get<Plan>(`/tasks/${id}/plan`),
  savePlan: (id: string, body: { body_md: string; revision: number }) => put<Plan>(`/tasks/${id}/plan`, body),
  acceptPlan: (id: string) => post<{ task: Task; plan: Plan }>(`/tasks/${id}/plan/accept`),
};
