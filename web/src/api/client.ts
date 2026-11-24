// Client of the Dory v2 node API.
import type {
  Attachment, Card, Column, CreateThreadResponse, Dependency, Entry, External, ExternalView, GithubAction as GithubActionFields, GithubClosed,
  GithubDetail, GithubDeviceCode, GithubDeviceState, GithubOverview, Hit, Job, KeptWorktree,
  Memory, Message, NodeDefaults, NodeSettings, NodeUpdate, Plan, Project, Provider, Repository,
  RepositoryLocation, Skill, SkillConflict, SkillState, Task, Thread, ThreadService, ThreadView,
  TitleModel, UpdateStatus, UsageBreakdownRow, UsageReport, UsageTurn, UsageWindow, Version,
} from "./types.gen";
export type {
  Attachment, Card, Column, Dependency, Entry, External, ExternalView, FileEdit, GithubClosed,
  GithubDetail, GithubDeviceCode, GithubDeviceState, GithubEvent, GithubIssue, GithubLink,
  GithubMilestone, GithubOverview, GithubPull, GithubRef, GithubSubIssue, GithubThread,
  GithubThreadComment, GithubUser, Hit, InputChord, InputSettings, Job, KeptWorktree, Memory,
  Message, Model, ModelChoice, NodeDefaults, NodeSettings, NodeUpdate, PendingRequest, Plan,
  Project, Provider, ProviderVersion, Pull, Question, Queued, Repository, RepositoryLocation,
  Role, SecretField, SecretPrompt, Skill, SkillConflict, SkillState, Stage, Task, Thread,
  ThreadDefaults, ThreadService, ThreadStatus, ThreadView, TitleModel, Todo, ToolCall, ToolKind,
  TurnModel, UpdateStatus, UsageBreakdownRow, UsageCost, UsageReport, UsageRow, UsageTokens,
  UsageTurn, UsageTurnCall, UsageWindow, Version,
} from "./types.gen";
import { textFor, type ErrorParams } from "../lib/errors";

export type GithubAction = Pick<GithubActionFields, "type"> & Partial<GithubActionFields>;
export type TaskInput = Partial<Task> & { title?: string };
export type MemoryInput = Partial<Memory> & { content: string };
// usable says whether a provider can run threads: installed and not turned off.
export const usable = (p: Provider) => p.installed && p.enabled !== false;
export type NodeInfo = { id: string; name: string; os: string; version: string; data_dir: string; listen: string; platform: string; icon?: string };
export type Peer = { id: string; name: string; address: string; fingerprint: string; connected: boolean; last_seen: string; revoked: boolean; disabled?: boolean; protocol: number; incompatible: boolean; info: { node?: NodeInfo; providers?: Provider[] } };
export type Mesh = { node_id: string; fingerprint: string; port: number; peers: Peer[]; discovered: { id: string; name: string; address: string }[] };
export type UsageRange = "24h" | "7d" | "30d" | "90d";
export type NodeStatus = { node: NodeInfo; providers: Provider[]; usage: Record<string, UsageWindow[]>; peers?: Peer[] };
export type User = { id: string; username: string };

export type GithubAuth = { source: "env" | "ui" | "gh" | "none"; login: string };

// ApiError keeps what the backend sent: status, stable code, technical message
// (detail) and params. Its message is the Spanish text of the code, so what
// the components show as error.message is already the one the user reads.
export class ApiError extends Error {
  constructor(public status: number, public code: string, public detail: string, public params: ErrorParams = {}) {
    super(textFor(code, detail, params));
  }
}

export async function apiError(res: Response): Promise<ApiError> {
  const e = (await res.json().catch(() => null))?.error;
  return new ApiError(res.status, e?.code ?? "", e?.message ?? res.statusText, e?.params ?? {});
}

export async function req<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    signal,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    throw await apiError(res);
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
  node?: string; project_id?: string | null; task_id?: string | null; repository_id?: string; provider: string; model?: string; effort?: string;
  runtime_mode?: Thread["runtime_mode"]; title?: string; text?: string; worktree?: boolean; branch?: string; base_branch?: string; worktree_path?: string;
};
export type Launch = { provider: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"]; base_branch?: string; text?: string };
export type ThreadActionName = "rename" | "pin" | "unpin" | "read" | "unread" | "settle" | "unsettle" | "snooze" | "unsnooze" | "link" | "archive";

export const api = {
  version: () => req<Version>("GET", "/version"),
  mesh: () => req<Mesh>("GET", "/mesh"),
  meshUpdates: (fresh = false) => req<NodeUpdate[]>("GET", fresh ? "/mesh/update?fresh=1" : "/mesh/update"),
  updateMesh: (opts: { providers: boolean; force: boolean }) => req<NodeUpdate[]>("POST", "/mesh/update", opts),
  updateNode: (node: string, opts: { providers: boolean; force: boolean }) => req<UpdateStatus>("POST", `/nodes/${node}/update`, opts),
  pairCode: () => req<{ code: string; expires_at: string; fingerprint: string; port: number }>("POST", "/mesh/pair/code"),
  pair: (address: string, code: string) => req<Peer>("POST", "/mesh/pair", { address, code }),
  revokePeer: (id: string) => req<void>("DELETE", `/mesh/peers/${id}`),
  setPeerEnabled: (id: string, enabled: boolean) => req<void>("PUT", `/mesh/peers/${id}/enabled`, { enabled }),
  devices: () => req<{ id: string; name: string; created_at: string }[]>("GET", "/devices"),
  revokeDevice: (id: string) => req<void>("DELETE", `/devices/${id}`),
  folders: (node: string, path = "", hidden = false) => req<{ path: string; parent: string; home: string; folders: { name: string; path: string; git: boolean }[] }>("GET", `/nodes/${node}/folders?${new URLSearchParams({ path, ...(hidden && { hidden: "true" }) })}`),
  checkout: (node: string, path: string) => req<{ path: string; name: string; git: boolean; remote: string; default_branch: string }>("GET", `/nodes/${node}/checkout?path=${encodeURIComponent(path)}`),
  setNodeRepositoryPath: (node: string, id: string, path: string) => req<void>("PUT", `/nodes/${node}/repositories/${id}/path`, { path }),
  externalSessions: (node: string) => req<ExternalView[]>("GET", `/nodes/${node}/external`),
  externalSession: (node: string, provider: string, id: string) => req<{ session: External; messages: Message[] }>("GET", `/nodes/${node}/external/${provider}/${id}`),
  adoptSession: (node: string, provider: string, id: string) => req<{ thread_id: string }>("POST", `/nodes/${node}/external/${provider}/${id}/adopt`),
  nodeProviders: (node: string) => req<Provider[]>("GET", `/nodes/${node}/providers`),
  addAccount: (node: string, a: { driver: string; name: string; home: string }) => req<{ id: string }>("POST", `/nodes/${node}/accounts`, a),
  updateAccount: (node: string, id: string, a: { name: string; home: string }) => req<void>("PUT", `/nodes/${node}/accounts/${id}`, a),
  removeAccount: (node: string, id: string) => req<void>("DELETE", `/nodes/${node}/accounts/${id}`),
  setNodeIcon: (node: string, icon: string) => req<void>("PUT", `/nodes/${node}/icon`, { icon }),
  setProviderEnabled: (node: string, provider: string, enabled: boolean) => req<void>("PUT", `/nodes/${node}/providers/${provider}`, { enabled }),
  providerJob: (node: string, provider: string, kind: Job["kind"]) => req<Job>("POST", `/nodes/${node}/providers/${provider}/${kind}`),
  job: (node: string, id: string) => req<Job>("GET", `/nodes/${node}/jobs/${id}`),
  submitLoginCode: (node: string, id: string, code: string) => req<void>("POST", `/nodes/${node}/jobs/${id}/code`, { code }),
  skills: () => req<Skill[]>("GET", "/skills"),
  skillStates: () => req<SkillState[]>("GET", "/skills/states"),
  saveSkill: (s: Partial<Skill>) => (s.id ? req<Skill>("PUT", `/skills/${s.id}`, s) : req<Skill>("POST", "/skills", s)),
  archiveSkill: (id: string) => req<void>("DELETE", `/skills/${id}`),
  setSkillEnabled: (id: string, enabled: boolean) => req<Skill>("PUT", `/skills/${id}/enabled`, { enabled }),
  skillConflicts: (node: string, id: string) => req<SkillConflict[]>("GET", `/nodes/${node}/skills/${id}/conflicts`),
  resolveSkill: (node: string, id: string, action: "restore" | "adopt", harness = "") => req<{ backup: string }>("POST", `/nodes/${node}/skills/${id}/resolve`, { action, harness }),
  forkSkill: (id: string, name?: string) => req<Skill>("POST", `/skills/${id}/fork`, { name: name ?? "" }),
  node: () => req<NodeStatus>("GET", "/node"),
  nodeDefaults: (node: string) => req<NodeDefaults>("GET", `/nodes/${node}/defaults`),
  nodeSettings: (node = "local") => req<NodeSettings>("GET", `/nodes/${node}/settings`),
  setNodeSettings: (settings: NodeSettings, node = "local") => req<NodeSettings>("PUT", `/nodes/${node}/settings`, settings),
  keptWorktrees: (node = "local") => req<KeptWorktree[]>("GET", `/nodes/${node}/kept-worktrees`),
  removeThreadWorktree: (id: string) => req<void>("DELETE", `/threads/${id}/worktree`),
  usageBreakdown: (range: UsageRange, node = "local") => req<UsageBreakdownRow[]>("GET", `/nodes/${node}/usage/breakdown?${new URLSearchParams({ range, tz: Intl.DateTimeFormat().resolvedOptions().timeZone })}`),
  threadUsageTurns: (thread: string, range: UsageRange, node: string) => req<UsageTurn[]>("GET", `/nodes/${node}/usage/threads/${encodeURIComponent(thread)}/turns?${new URLSearchParams({ range, tz: Intl.DateTimeFormat().resolvedOptions().timeZone })}`),
  usageHistory: (range: UsageRange, node = "local") => req<UsageReport>("GET", `/nodes/${node}/usage/history?${new URLSearchParams({ range, tz: Intl.DateTimeFormat().resolvedOptions().timeZone })}`),
  searchTasks: (text: string, signal?: AbortSignal) => req<Hit[]>("GET", `/search?type=task&q=${encodeURIComponent(text)}`, undefined, signal),
  searchGithub: (text: string, signal?: AbortSignal) => req<{ items: { repo: string; number: number; title: string; kind: "pulls" | "issues"; state: string }[]; incomplete: boolean }>("GET", `/github/search?q=${encodeURIComponent(text)}`, undefined, signal),
  search: (text: string) => req<Hit[]>("GET", `/search?q=${encodeURIComponent(text)}`),
  login: (username: string, password: string) => req<User>("POST", "/auth/login", { username, password }),
  setupNeeded: () => req<{ needed: boolean }>("GET", "/auth/setup"),
  setup: (username: string, password: string) => req<User>("POST", "/auth/setup", { username, password }),
  logout: () => req<void>("POST", "/auth/logout"),
  me: () => req<User>("GET", "/auth/me"),
  projects: resource<Project, Partial<Project>>("/projects"),
  repositories: resource<Repository, Partial<Repository>>("/repositories"),
  repositoryLocations: () => req<RepositoryLocation[]>("GET", "/repository-locations"),
  projectFromRepository: (body: { name: string; slug: string; description: string; remote_url: string; default_branch: string }) => req<Repository>("POST", "/projects/from-repository", body),
  repositoryPaths: () => req<Record<string, string>>("GET", "/repository-paths"),
  setRepositoryPath: (id: string, path: string) => req<void>("PUT", `/repositories/${id}/path`, { path }),
  tasks: resource<Task, TaskInput>("/tasks"),
  // createIdea saves the text of «Nueva idea»; the server may retitle it in the background.
  createIdea: (body: { id: string; text: string; project_id: string | null }) => req<Task>("POST", "/ideas", body),
  titleModel: () => req<TitleModel>("GET", "/title-model"),
  setTitleModel: (body: TitleModel) => req<TitleModel>("PUT", "/title-model", body),
  board: (project?: string) => req<{ columns: Column[]; cards: Card[] }>("GET", `/board${project ? `?project=${project}` : ""}`),
  card: (id: string) => req<Card>("GET", `/tasks/${id}/card`),
  dependencies: (id: string) => req<Dependency[]>("GET", `/tasks/${id}/dependencies`),
  addDependency: (id: string, dep: string) => req<Dependency>("PUT", `/tasks/${id}/dependencies/${dep}`),
  removeDependency: (id: string, dep: string) => req<void>("DELETE", `/tasks/${id}/dependencies/${dep}`),
  plans: (id: string) => req<Plan[]>("GET", `/tasks/${id}/plans`),
  // The plan document of a task: one Markdown text with the revision it was read at.
  plan: (id: string) => req<Plan>("GET", `/tasks/${id}/plan`),
  savePlan: (id: string, body: { body_md: string; revision: number }) => req<Plan>("PUT", `/tasks/${id}/plan`, body),
  acceptPlan: (id: string) => req<{ task: Task; plan: Plan }>("POST", `/tasks/${id}/plan/accept`),
  launch: (id: string, body: Launch) => req<{ thread_id: string; branch: string }>("POST", `/tasks/${id}/launch`, body),
  memories: resource<Memory, MemoryInput>("/memories"),
  threads: (filter?: Record<string, unknown>, archived = false) => req<Thread[]>("GET", "/threads" + q(filter, archived)),
  thread: (id: string) => req<ThreadView>("GET", `/threads/${id}`),
  projectWorktrees: (node: string, project: string) => req<{ path: string; branch: string; threads: number; working: number }[]>("GET", `/nodes/${node || "local"}/projects/${project}/worktrees`),
  projectBranches: (node: string, project: string) => req<{ current: string; branches: string[] }>("GET", `/nodes/${node || "local"}/projects/${project}/branches`),
  createThread: ({ node, ...body }: NewThread) => req<CreateThreadResponse>("POST", `/nodes/${node || "local"}/threads`, body),
  send: (id: string, text: string, files: string[] = []) => req<void>("POST", `/threads/${id}/messages`, { text, files }),
  upload: async (id: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`/api/threads/${id}/attachments`, { method: "POST", credentials: "same-origin", body: form });
    if (!res.ok) throw await apiError(res);
    return (await res.json()) as Attachment;
  },
  attachmentUrl: (id: string, file: string) => `/api/threads/${id}/attachments/${file}`,
  queue: (id: string, run: string, action: "edit" | "cancel" | "steer", text?: string) => req<void>("POST", `/threads/${id}/queue/${run}`, { action, text }),
  threadSettings: (id: string, body: { provider?: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"] }) => req<Thread>("POST", `/threads/${id}/settings`, body),
  nodeCommands: (node: string, provider: string, project?: string) => req<{ commands: Entry[]; skills: Entry[] }>("GET", `/nodes/${node || "local"}/commands?provider=${encodeURIComponent(provider)}&project=${encodeURIComponent(project ?? "")}`),
  threadCommands: (id: string) => req<{ commands: Entry[]; skills: Entry[] }>("GET", `/threads/${id}/commands`),
  threadFiles: (id: string, q: string) => req<string[]>("GET", `/threads/${id}/files?q=${encodeURIComponent(q)}`),
  answerSecret: (id: string, request: string, body: { values: Record<string, string> } | { decline: true }) => req<void>("POST", `/threads/${id}/secrets/${encodeURIComponent(request)}`, body),
  answer: (id: string, request: string, answers: string[][], files: string[][] = []) => req<void>("POST", `/threads/${id}/requests/${encodeURIComponent(request)}`, { answers, files }),
  interrupt: (id: string) => req<void>("POST", `/threads/${id}/interrupt`),
  revert: (id: string, messageId: string) => req<{ text: string; files_restored: boolean }>("POST", `/threads/${id}/revert`, { message_id: messageId }),
  threadDiff: (id: string, base = "") => req<{ diff: string; branch: string; base?: string; branches?: string[] }>("GET", `/threads/${id}/diff${base ? `?base=${encodeURIComponent(base)}` : ""}`),
  threadServices: (id: string) => req<ThreadService[]>("GET", `/threads/${id}/services`),
  stopThreadService: (id: string, pid: number) => req<void>("POST", `/threads/${id}/processes/${pid}/stop`),
  threadAction: (id: string, body: { action: ThreadActionName; title?: string; until?: string; task_id?: string }) => req<Thread>("POST", `/threads/${id}/actions`, body),
  issueTask: (repo: string, number: number, body: { title: string; description: string }) => req<Task>("POST", `/github/repos/${repo}/issues/${number}/tasks`, body),
  githubPullRefs: (repo: string, q: string) => req<{ pulls: { number: number; title: string; state: "open" | "draft" | "merged" | "closed" }[] }>("GET", `/github/repos/${repo}/pull-refs?q=${encodeURIComponent(q)}`),
  branches: (repo: string) => req<string[]>("GET", `/github/repos/${repo}/branches`),
  githubAuth: () => req<GithubAuth>("GET", "/github/auth"),
  githubLogout: () => req<void>("DELETE", "/github/auth"),
  githubDeviceStart: () => req<GithubDeviceCode>("POST", "/github/auth/device"),
  githubDeviceState: () => req<GithubDeviceState>("GET", "/github/auth/device"),
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
export type LaunchChoice = { node: string; provider: string; model: string; effort: string; runtime_mode: Thread["runtime_mode"] };

export const COLUMN_LABELS: Record<Column, string> = {
  inbox: "Inbox", ready: "Ready", working: "Working", needs_you: "Needs you",
  in_review: "In review", ready_to_merge: "Ready to merge", done: "Done", cancelled: "Cancelled",
};

// repoName extracts owner/name from a GitHub remote.
export const repoName = (remote: string) => remote.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? remote;

export type DeviceTools = { adb: string; emulator: string; kvm: boolean; sdk: string };
export type Device = { serial: string; state: string; model: string; product: string; transport: "usb" | "wifi" | "emulator"; avd?: string; streaming: boolean };
export type AvdState = { status: "starting" | "booting" | "ready" | "failed"; error?: string };
export type DeviceList = { tools: DeviceTools; devices: Device[]; avds: string[]; avd_states: Record<string, AvdState>; error?: string };
export const devicesApi = {
  list: (node = "local") => req<DeviceList>("GET", `/nodes/${node}/devices`),
  startAvd: (name: string, node = "local") => req<{ ok: boolean }>("POST", `/nodes/${node}/devices/avd/${encodeURIComponent(name)}/start`),
  stop: (serial: string, confirm: boolean, node = "local") => req<{ ok: boolean }>("POST", `/nodes/${node}/devices/${encodeURIComponent(serial)}/stop`, { confirm }),
  pair: (address: string, code: string, node = "local") => req<{ message: string }>("POST", `/nodes/${node}/devices/pair`, { address, code }),
  connect: (address: string, node = "local") => req<{ message: string }>("POST", `/nodes/${node}/devices/connect`, { address }),
  action: (serial: string, body: Record<string, unknown>, node = "local") => req<Record<string, string>>("POST", `/nodes/${node}/devices/${encodeURIComponent(serial)}/action`, body),
  disconnect: (serial: string, node = "local") => req<{ ok: boolean }>("POST", `/nodes/${node}/devices/${encodeURIComponent(serial)}/disconnect`),
};
