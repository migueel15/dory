// Client of the Dory v2 node API. Types mirror internal/data and internal/api.
import type { GithubAction, GithubClosed, GithubDetail, GithubOverview } from "./github-types";
export type * from "./github-types";

export type Project = { id: string; slug: string; name: string; description: string; created_at: string; updated_at: string; archived_at: string | null };
export type Repository = { id: string; project_id: string; remote_url: string; default_branch: string; created_at: string; updated_at: string; archived_at: string | null };
export type Stage = "inbox" | "ready" | "cancelled";
export type Task = {
  id: string; project_id: string | null; repository_id: string | null; title: string; description: string; stage: Stage;
  branch: string; issue_number: number | null; pr_number: number | null; position: string;
  created_at: string; updated_at: string; archived_at: string | null;
};
export type TaskInput = Partial<Task> & { title?: string };
export type Plan = { id: string; task_id: string; version: number; sections: Record<string, string>; open_questions: string[]; author: "agent" | "user"; accepted_at: string | null; created_at: string };
export type Dependency = { task_id: string; depends_on_id: string; created_at: string };
export type Memory = { id: string; scope: "global" | "project" | "node"; scope_id: string; kind: "decision" | "lesson" | "context"; title: string; content: string; labels: string[]; created_at: string; updated_at: string; archived_at: string | null };
export type MemoryInput = Partial<Memory> & { content: string };
export type ThreadStatus = "idle" | "working" | "waiting" | "error";
export type Thread = {
  id: string; node_id: string; project_id: string | null; task_id: string | null; parent_id: string | null;
  provider: string; model: string; effort: string; runtime_mode: "full-access" | "approval-required";
  title: string; branch: string; status: ThreadStatus;
  pinned_at: string | null; read_at: string | null; last_activity_at: string | null; snoozed_until: string | null; settled_at: string | null;
  created_at: string; updated_at: string; archived_at: string | null;
};
export type Role = "user" | "assistant" | "reasoning" | "tool" | "plan" | "system" | "interrupt";
export type Message = { id: string; thread_id?: string; run_id?: string; role: Role; text: string; title?: string; detail?: string; status?: string; streaming: boolean; created_at: string };
export type Queued = { run_id: string; text: string };
export type ThreadView = Thread & { messages: Message[]; queue: Queued[]; running: boolean; worktree: string; local: boolean };
export type Column = "inbox" | "ready" | "working" | "needs_you" | "in_review" | "ready_to_merge" | "done" | "cancelled";
export type Pull = { repo: string; number: number; title: string; url: string; draft: boolean; ci: string; review_decision: string; review_requests: string[]; mergeable: string; merge_state: string; head_ref: string };
export type Card = Task & { column: Column; repo?: string; threads: Thread[]; pull?: Pull; pr_state?: "open" | "draft" | "merged" | "closed"; blockers: string[]; has_plan: boolean };
export type Model = { slug: string; name: string; efforts?: string[] };
export type Provider = { id: string; name: string; installed: boolean; version: string; models: Model[]; capabilities: string[]; logged_in?: boolean; account?: string };
export type Job = { id: string; provider: string; kind: "login" | "update"; output: string; done: boolean; error?: string };
export type Skill = { id: string; name: string; description: string; body_md: string; files: Record<string, string>; version: number; targets: { nodes: string[] | null; harnesses: string[] | null }; created_at: string; updated_at: string; archived_at: string | null };
export type SkillState = { skill_id: string; node_id: string; status: "synced" | "pending" | "error" | "conflict" | "removed"; version: number; message: string; updated_at: string };
export type UsageWindow = { kind: string; utilization: number; resets_at: string };
export type NodeInfo = { id: string; name: string; os: string; version: string; data_dir: string; listen: string; platform: string };
export type Peer = { id: string; name: string; address: string; fingerprint: string; connected: boolean; last_seen: string; revoked: boolean; info: { node?: NodeInfo; providers?: Provider[] } };
export type Mesh = { node_id: string; fingerprint: string; port: number; peers: Peer[]; discovered: { id: string; name: string; address: string }[] };
export type NodeStatus = { node: NodeInfo; providers: Provider[]; usage: Record<string, UsageWindow[]>; peers?: Peer[] };
export type Hit = { entity: "task" | "memory"; id: string; title: string; rank: number; record: Task | Memory };
export type User = { id: string; username: string };
export type Version = { revision: string; short: string; time: string; modified: boolean };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const e = await res.json().catch(() => null);
    throw new ApiError(res.status, e?.error?.message ?? res.statusText);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

const q = (filter?: Record<string, unknown>, archived = false) =>
  `?limit=1000${filter ? `&filter=${encodeURIComponent(JSON.stringify(filter))}` : ""}${archived ? "&include_archived=true" : ""}`;

// Generic resource: list, get, create, update (fields left out keep their value) and archive.
function resource<T extends { id: string }, I>(path: string) {
  return {
    list: (filter?: Record<string, unknown>, archived = false) => req<T[]>("GET", path + q(filter, archived)),
    get: (id: string) => req<T>("GET", `${path}/${id}`),
    create: (b: I) => req<T>("POST", path, b),
    update: (id: string, b: Partial<I>) => req<T>("PUT", `${path}/${id}`, b),
    archive: (id: string) => req<void>("DELETE", `${path}/${id}`),
  };
}

export type NewThread = {
  project_id?: string | null; task_id?: string | null; repository_id?: string; provider: string; model?: string; effort?: string;
  runtime_mode?: Thread["runtime_mode"]; title?: string; text?: string; worktree?: boolean; branch?: string; base_branch?: string;
};
export type Launch = { provider: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"]; base_branch?: string; text?: string };
export type ThreadActionName = "rename" | "pin" | "unpin" | "read" | "unread" | "settle" | "unsettle" | "snooze" | "unsnooze" | "link" | "archive";

export const api = {
  version: () => req<Version>("GET", "/version"),
  mesh: () => req<Mesh>("GET", "/mesh"),
  pairCode: () => req<{ code: string; expires_at: string; fingerprint: string; port: number }>("POST", "/mesh/pair/code"),
  pair: (address: string, code: string) => req<Peer>("POST", "/mesh/pair", { address, code }),
  revokePeer: (id: string) => req<void>("DELETE", `/mesh/peers/${id}`),
  nodeProviders: (node: string) => req<Provider[]>("GET", `/nodes/${node}/providers`),
  providerJob: (node: string, provider: string, kind: Job["kind"]) => req<Job>("POST", `/nodes/${node}/providers/${provider}/${kind}`),
  job: (node: string, id: string) => req<Job>("GET", `/nodes/${node}/jobs/${id}`),
  skills: () => req<Skill[]>("GET", "/skills"),
  skillStates: () => req<SkillState[]>("GET", "/skills/states"),
  saveSkill: (s: Partial<Skill>) => (s.id ? req<Skill>("PUT", `/skills/${s.id}`, s) : req<Skill>("POST", "/skills", s)),
  archiveSkill: (id: string) => req<void>("DELETE", `/skills/${id}`),
  node: () => req<NodeStatus>("GET", "/node"),
  search: (text: string) => req<Hit[]>("GET", `/search?q=${encodeURIComponent(text)}`),
  login: (username: string, password: string) => req<User>("POST", "/auth/login", { username, password }),
  setupNeeded: () => req<{ needed: boolean }>("GET", "/auth/setup"),
  setup: (username: string, password: string) => req<User>("POST", "/auth/setup", { username, password }),
  logout: () => req<void>("POST", "/auth/logout"),
  me: () => req<User>("GET", "/auth/me"),
  projects: resource<Project, Partial<Project>>("/projects"),
  repositories: resource<Repository, Partial<Repository>>("/repositories"),
  repositoryPaths: () => req<Record<string, string>>("GET", "/repository-paths"),
  setRepositoryPath: (id: string, path: string) => req<void>("PUT", `/repositories/${id}/path`, { path }),
  tasks: resource<Task, TaskInput>("/tasks"),
  board: (project?: string) => req<{ columns: Column[]; cards: Card[] }>("GET", `/board${project ? `?project=${project}` : ""}`),
  card: (id: string) => req<Card>("GET", `/tasks/${id}/card`),
  dependencies: (id: string) => req<Dependency[]>("GET", `/tasks/${id}/dependencies`),
  addDependency: (id: string, dep: string) => req<Dependency>("PUT", `/tasks/${id}/dependencies/${dep}`),
  removeDependency: (id: string, dep: string) => req<void>("DELETE", `/tasks/${id}/dependencies/${dep}`),
  plans: (id: string) => req<Plan[]>("GET", `/tasks/${id}/plans`),
  createPlan: (id: string, body: { sections: Record<string, string>; open_questions: string[] }) => req<Plan>("POST", `/tasks/${id}/plans`, body),
  acceptPlan: (id: string) => req<{ task: Task; plan: Plan }>("POST", `/tasks/${id}/plan/accept`),
  launch: (id: string, body: Launch) => req<{ thread_id: string; branch: string }>("POST", `/tasks/${id}/launch`, body),
  memories: resource<Memory, MemoryInput>("/memories"),
  threads: (filter?: Record<string, unknown>, archived = false) => req<Thread[]>("GET", "/threads" + q(filter, archived)),
  thread: (id: string) => req<ThreadView>("GET", `/threads/${id}`),
  createThread: (body: NewThread) => req<{ thread_id: string }>("POST", "/threads", body),
  send: (id: string, text: string) => req<void>("POST", `/threads/${id}/messages`, { text }),
  interrupt: (id: string) => req<void>("POST", `/threads/${id}/interrupt`),
  threadDiff: (id: string) => req<{ diff: string; branch: string }>("GET", `/threads/${id}/diff`),
  threadAction: (id: string, body: { action: ThreadActionName; title?: string; until?: string; task_id?: string }) => req<Thread>("POST", `/threads/${id}/actions`, body),
  issueTask: (repo: string, number: number, body: { title: string; description: string }) => req<Task>("POST", `/github/repos/${repo}/issues/${number}/tasks`, body),
  githubPullRefs: (repo: string, q: string) => req<{ pulls: { number: number; title: string; state: "open" | "draft" | "merged" | "closed" }[] }>("GET", `/github/repos/${repo}/pull-refs?q=${encodeURIComponent(q)}`),
  branches: (repo: string) => req<string[]>("GET", `/github/repos/${repo}/branches`),
  github: (refresh = false) => req<GithubOverview>("GET", `/github/open${refresh ? "?refresh=true" : ""}`),
  githubClosed: (kind: "pulls" | "issues", project: string, cursor: string) =>
    req<GithubClosed>("GET", `/github/closed?${new URLSearchParams({ kind, ...(project && { project }), ...(cursor && { cursor }) })}`),
  githubDetail: (repo: string, kind: string, number: number) => req<GithubDetail>("GET", `/github/repos/${repo}/${kind}/${number}`),
  githubAct: (repo: string, kind: string, number: number, action: GithubAction) => req<void>("POST", `/github/repos/${repo}/${kind}/${number}/actions`, action),
  githubDiff: (repo: string, number: number) => req<{ diff: string }>("GET", `/github/repos/${repo}/pulls/${number}/diff`),
  pullReviews: (repo: string, number: number) => req<PullReview[]>("GET", `/github/repos/${repo}/pulls/${number}/reviews`),
  markPullReview: (repo: string, number: number, path: string, patch_hash: string) => req<void>("PUT", `/github/repos/${repo}/pulls/${number}/reviews`, { path, patch_hash }),
  unmarkPullReview: (repo: string, number: number, path: string) => req<void>("DELETE", `/github/repos/${repo}/pulls/${number}/reviews?path=${encodeURIComponent(path)}`),
  pullAgent: (repo: string, number: number) => req<PullAgent>("GET", `/github/repos/${repo}/pulls/${number}/agent`),
  createPullReview: (repo: string, number: number, launch: LaunchChoice) => req<PullAgent>("POST", `/github/repos/${repo}/pulls/${number}/agent`, { launch }),
  askPullAgent: (repo: string, number: number, body: { path: string; start: number; end: number; snippet: string; question: string; launch?: LaunchChoice }) =>
    req<PullAgent>("POST", `/github/repos/${repo}/pulls/${number}/ask`, body),
};

export type PullReview = { path: string; patch_hash: string; reviewed_at: string };
export type PullAgent = { available: boolean; thread_id?: string };
// LaunchChoice is where and how a new thread runs.
export type LaunchChoice = { provider: string; model: string; effort: string; runtime_mode: Thread["runtime_mode"] };

export const COLUMN_LABELS: Record<Column, string> = {
  inbox: "Inbox", ready: "Ready", working: "Working", needs_you: "Needs you",
  in_review: "In review", ready_to_merge: "Ready to merge", done: "Done", cancelled: "Cancelled",
};

// repoName extracts owner/name from a GitHub remote.
export const repoName = (remote: string) => remote.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? remote;
