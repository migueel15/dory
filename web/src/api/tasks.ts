import { get, post, put, del, resource } from "./http";
import type { Card, Column, Dependency, Hit, Task } from "./types.gen";
import type { Launch } from "./threads";

export type TaskInput = Partial<Task> & { title?: string };

export const tasksApi = {
  tasks: resource<Task, TaskInput>("/tasks"),
  // createIdea saves the text of «Nueva idea»; the server may retitle it in the background.
  createIdea: (body: { id: string; text: string; project_id: string | null }) => post<Task>("/ideas", body),
  board: (project?: string) => get<{ columns: Column[]; cards: Card[] }>(`/board${project ? `?project=${project}` : ""}`),
  card: (id: string) => get<Card>(`/tasks/${id}/card`),
  dependencies: (id: string) => get<Dependency[]>(`/tasks/${id}/dependencies`),
  addDependency: (id: string, dep: string) => put<Dependency>(`/tasks/${id}/dependencies/${dep}`),
  removeDependency: (id: string, dep: string) => del(`/tasks/${id}/dependencies/${dep}`),
  launch: (id: string, body: Launch) => post<{ thread_id: string; branch: string }>(`/tasks/${id}/launch`, body),
  searchTasks: (text: string, signal?: AbortSignal) => get<Hit[]>(`/search?type=task&q=${encodeURIComponent(text)}`, signal),
  search: (text: string) => get<Hit[]>(`/search?q=${encodeURIComponent(text)}`),
};
