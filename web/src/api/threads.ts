import { get, post, del, listQuery, upload } from "./http";
import type { Attachment, CreateThreadResponse, Entry, External, ExternalView, Message, Thread, ThreadService, ThreadView } from "./types.gen";

export type NewThread = {
  node?: string; project_id?: string | null; task_id?: string | null; repository_id?: string; provider: string; model?: string; effort?: string;
  runtime_mode?: Thread["runtime_mode"]; title?: string; text?: string; worktree?: boolean; branch?: string; base_branch?: string; worktree_path?: string;
};
export type Launch = { provider: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"]; base_branch?: string; text?: string };
export type ThreadActionName = "rename" | "pin" | "unpin" | "read" | "unread" | "settle" | "unsettle" | "snooze" | "unsnooze" | "link" | "archive";

export const threadsApi = {
  externalSessions: (node: string) => get<ExternalView[]>(`/nodes/${node}/external`),
  externalSession: (node: string, provider: string, id: string) => get<{ session: External; messages: Message[] }>(`/nodes/${node}/external/${provider}/${id}`),
  adoptSession: (node: string, provider: string, id: string) => post<{ thread_id: string }>(`/nodes/${node}/external/${provider}/${id}/adopt`),
  threads: (filter?: Record<string, unknown>, archived = false) => get<Thread[]>("/threads" + listQuery(filter, archived)),
  thread: (id: string) => get<ThreadView>(`/threads/${id}`),
  projectWorktrees: (node: string, project: string) => get<{ path: string; branch: string; threads: number; working: number }[]>(`/nodes/${node || "local"}/projects/${project}/worktrees`),
  projectBranches: (node: string, project: string) => get<{ current: string; branches: string[] }>(`/nodes/${node || "local"}/projects/${project}/branches`),
  createThread: ({ node, ...body }: NewThread) => post<CreateThreadResponse>(`/nodes/${node || "local"}/threads`, body),
  send: (id: string, text: string, files: string[] = [], steer = false) => post<void>(`/threads/${id}/messages`, { text, files, steer }),
  upload: (id: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    return upload<Attachment>(`/threads/${id}/attachments`, form);
  },
  attachmentUrl: (id: string, file: string) => `/api/threads/${id}/attachments/${file}`,
  queue: (id: string, run: string, action: "edit" | "cancel" | "steer", text?: string) => post<void>(`/threads/${id}/queue/${run}`, { action, text }),
  threadSettings: (id: string, body: { provider?: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"] }) => post<Thread>(`/threads/${id}/settings`, body),
  nodeCommands: (node: string, provider: string, project?: string) => get<{ commands: Entry[]; skills: Entry[] }>(`/nodes/${node || "local"}/commands?provider=${encodeURIComponent(provider)}&project=${encodeURIComponent(project ?? "")}`),
  threadCommands: (id: string) => get<{ commands: Entry[]; skills: Entry[] }>(`/threads/${id}/commands`),
  threadFiles: (id: string, q: string) => get<string[]>(`/threads/${id}/files?q=${encodeURIComponent(q)}`),
  answerSecret: (id: string, request: string, body: { values: Record<string, string> } | { decline: true }) => post<void>(`/threads/${id}/secrets/${encodeURIComponent(request)}`, body),
  answer: (id: string, request: string, answers: string[][], files: string[][] = []) => post<void>(`/threads/${id}/requests/${encodeURIComponent(request)}`, { answers, files }),
  interrupt: (id: string) => post<void>(`/threads/${id}/interrupt`),
  revert: (id: string, messageId: string) => post<{ text: string; files_restored: boolean }>(`/threads/${id}/revert`, { message_id: messageId }),
  threadDiff: (id: string, base = "") => get<{ diff: string; branch: string; base?: string; branches?: string[] }>(`/threads/${id}/diff${base ? `?base=${encodeURIComponent(base)}` : ""}`),
  threadServices: (id: string) => get<ThreadService[]>(`/threads/${id}/services`),
  stopThreadService: (id: string, pid: number) => post<void>(`/threads/${id}/processes/${pid}/stop`),
  threadAction: (id: string, body: { action: ThreadActionName; title?: string; until?: string; task_id?: string }) => post<Thread>(`/threads/${id}/actions`, body),
};
