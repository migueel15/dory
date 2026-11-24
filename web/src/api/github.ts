import { get, post, put, del } from "./http";
import type { GithubAction as GithubActionFields, GithubClosed, GithubDetail, GithubDeviceCode, GithubDeviceState, GithubOverview, Task, Thread } from "./types.gen";

export type GithubAction = Pick<GithubActionFields, "type"> & Partial<GithubActionFields>;
export type GithubAuth = { source: "env" | "ui" | "gh" | "none"; login: string };
export type PullReview = { path: string; patch_hash: string; reviewed_at: string };
export type PullAgent = { available: boolean; thread_id?: string };
// LaunchChoice is where and how a new thread runs.
export type LaunchChoice = { node: string; provider: string; model: string; effort: string; runtime_mode: Thread["runtime_mode"] };

export const githubApi = {
  searchGithub: (text: string, signal?: AbortSignal) => get<{ items: { repo: string; number: number; title: string; kind: "pulls" | "issues"; state: string }[]; incomplete: boolean }>(`/github/search?q=${encodeURIComponent(text)}`, signal),
  issueTask: (repo: string, number: number, body: { title: string; description: string }) => post<Task>(`/github/repos/${repo}/issues/${number}/tasks`, body),
  githubPullRefs: (repo: string, q: string) => get<{ pulls: { number: number; title: string; state: "open" | "draft" | "merged" | "closed" }[] }>(`/github/repos/${repo}/pull-refs?q=${encodeURIComponent(q)}`),
  branches: (repo: string) => get<string[]>(`/github/repos/${repo}/branches`),
  githubAuth: () => get<GithubAuth>("/github/auth"),
  githubLogout: () => del("/github/auth"),
  githubDeviceStart: () => post<GithubDeviceCode>("/github/auth/device"),
  githubDeviceState: () => get<GithubDeviceState>("/github/auth/device"),
  github: (refresh = false) => get<GithubOverview>(`/github/open${refresh ? "?refresh=true" : ""}`),
  githubClosed: (kind: "pulls" | "issues", project: string, cursor: string) =>
    get<GithubClosed>(`/github/closed?${new URLSearchParams({ kind, ...(project && { project }), ...(cursor && { cursor }) })}`),
  githubDetail: (repo: string, kind: string, number: number) => get<GithubDetail>(`/github/repos/${repo}/${kind}/${number}`),
  githubAct: (repo: string, kind: string, number: number, action: GithubAction) => post<void>(`/github/repos/${repo}/${kind}/${number}/actions`, action),
  githubDiff: (repo: string, number: number) => get<{ diff: string }>(`/github/repos/${repo}/pulls/${number}/diff`),
  pullReviews: (repo: string, number: number) => get<PullReview[]>(`/github/repos/${repo}/pulls/${number}/reviews`),
  markPullReview: (repo: string, number: number, path: string, patch_hash: string) => put<void>(`/github/repos/${repo}/pulls/${number}/reviews`, { path, patch_hash }),
  unmarkPullReview: (repo: string, number: number, path: string) => del(`/github/repos/${repo}/pulls/${number}/reviews?path=${encodeURIComponent(path)}`),
  pullAgent: (repo: string, number: number) => get<PullAgent>(`/github/repos/${repo}/pulls/${number}/agent`),
  createPullReview: (repo: string, number: number, launch: LaunchChoice) => post<PullAgent>(`/github/repos/${repo}/pulls/${number}/agent`, { launch }),
  askPullAgent: (repo: string, number: number, body: { path: string; start: number; end: number; snippet: string; question: string; launch?: LaunchChoice }) =>
    post<PullAgent>(`/github/repos/${repo}/pulls/${number}/ask`, body),
};
