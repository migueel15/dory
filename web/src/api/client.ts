// Client of the Dory v2 node API. Types mirror internal/data and internal/api.
import type { GithubAction, GithubClosed, GithubDetail, GithubOverview } from "./github-types";
export type * from "./github-types";

export type UpdateStatus = { channel: string; current: string; latest?: string; available: boolean; desktop: boolean; active_turns: number; state: "idle" | "updating" | "failed"; error?: string; providers: ProviderVersion[] };
export type ProviderVersion = { id: string; name: string; current: string; latest?: string; available: boolean };
export type NodeUpdate = { node: string; name: string; status?: UpdateStatus; error?: string };
export type Project = { id: string; slug: string; name: string; description: string; created_at: string; updated_at: string; archived_at: string | null };
export type RepositoryLocation = { repository_id: string; node_id: string; path: string };
export type Repository = { id: string; project_id: string; remote_url: string; default_branch: string; created_at: string; updated_at: string; archived_at: string | null };
export type Stage = "inbox" | "ready" | "done" | "cancelled";
export type NodeSettings = { archive_threads_on_finish: boolean; remove_worktree_on_archive: boolean; stop_services_on_shutdown: boolean; agents_use_devices: boolean; browser_tunnel_disabled?: boolean; threads?: ThreadDefaults };
export type ModelChoice = { provider: string; model: string; effort: string };
export type ThreadDefaults = { default_model: ModelChoice; fallback_model: ModelChoice; default_workspace: "checkout" | "worktree" | "" };
export type ThreadService = { pid: number; address: string; port: number; command: string; kind: string };
export type KeptWorktree = { thread_id: string; path: string; dirty: boolean };
export type Task = {
  id: string; project_id: string | null; repository_id: string | null; title: string; description: string; stage: Stage;
  branch: string; issue_number: number | null; pr_number: number | null; position: string;
  created_at: string; updated_at: string; archived_at: string | null;
};
export type TaskInput = Partial<Task> & { title?: string };
export type Plan = { id: string; task_id: string; version: number; sections: Record<string, string>; open_questions: string[]; body_md: string; revision: number; author: "agent" | "user"; accepted_at: string | null; created_at: string; updated_at: string };
export type Dependency = { task_id: string; depends_on_id: string; created_at: string };
export type Memory = { id: string; scope: "global" | "project" | "node"; scope_id: string; kind: "decision" | "lesson" | "context"; title: string; content: string; labels: string[]; created_at: string; updated_at: string; archived_at: string | null };
export type MemoryInput = Partial<Memory> & { content: string };
export type ThreadStatus = "idle" | "working" | "waiting" | "error";
export type Thread = {
  id: string; node_id: string; project_id: string | null; task_id: string | null; parent_id: string | null; origin_id?: string | null;
  provider: string; model: string; effort: string; runtime_mode: "full-access" | "approval-required";
  title: string; branch: string; status: ThreadStatus;
  // task_linked_at is when the thread was linked to its task; older threads do not have it, so created_at stands in.
  task_linked_at?: string | null;
  pinned_at: string | null; read_at: string | null; last_activity_at: string | null; snoozed_until: string | null; settled_at: string | null;
  created_at: string; updated_at: string; archived_at: string | null;
};
export type Role = "user" | "assistant" | "reasoning" | "tool" | "plan" | "question" | "system" | "interrupt" | "clear" | "model-change";
// ToolCall is a tool call in Dory's own terms (runtime.ToolCall): every harness
// maps its tools onto these kinds, and the chat draws one card per kind.
export type ToolKind = "edit" | "command" | "todo" | "plan" | "read" | "search" | "web" | "agent" | "mcp" | "question" | "other";
export type FileEdit = { path: string; change?: "add" | "update" | "delete" | string; added: number; removed: number; diff?: string };
export type Todo = { text: string; status: "pending" | "in_progress" | "completed" | string; active?: string };
export type Question = { id?: string; question: string; header?: string; options?: { label: string; description?: string }[]; multi_select?: boolean; answer?: string[] };
// PendingRequest is something the agent waits for in the middle of its turn.
export type PendingRequest = { id: string; kind: "question" | "permission"; message_id?: string; questions: Question[]; created_at: string };
export type ToolCall = {
  kind: ToolKind; name: string; files?: FileEdit[]; command?: string; exit_code?: number; todos?: Todo[];
  path?: string; lines?: string; pattern?: string; url?: string; query?: string; agent?: string; summary?: string; steps?: number;
  server?: string; tool?: string; input?: string; questions?: Question[];
};
export type Message = { id: string; thread_id?: string; run_id?: string; role: Role; text: string; title?: string; detail?: string; status?: string; streaming: boolean; created_at: string; tool?: ToolCall; sender_thread_id?: string };
// Entry is a slash command or skill of a harness; insert is how it is invoked.
export type Entry = { name: string; description: string; insert: string };
export type Attachment = { id: string; name: string; mime: string; path: string };
export type Queued = { run_id: string; text: string; files?: Attachment[] };
export type TurnModel = { provider: string; model: string | null; reported_model?: string };
export type ThreadView = Thread & { last_turn_model?: TurnModel; reported_settings?: { model?: string; effort?: string }; messages: Message[]; queue: Queued[]; requests?: PendingRequest[]; running: boolean; worktree: string; local: boolean };
export type Column = "inbox" | "ready" | "working" | "needs_you" | "in_review" | "ready_to_merge" | "done" | "cancelled";
export type Pull = { repo: string; number: number; title: string; url: string; draft: boolean; ci: string; review_decision: string; review_requests: string[]; approved_by?: string[]; changes_by?: string[]; ci_passed?: number; ci_failed?: number; ci_pending?: number; mergeable: string; merge_state: string; head_ref: string };
export type Card = Task & { column: Column; repo?: string; threads: Thread[]; pull?: Pull; pr_state?: "open" | "draft" | "merged" | "closed"; blockers: string[]; has_plan: boolean };
export type Model = { slug: string; name: string; efforts?: string[]; resolved_model?: string };
export type TitleModel = { provider: string; model: string };
export type Provider = { id: string; name: string; installed: boolean; enabled?: boolean; version: string; models: Model[]; capabilities: string[]; logged_in?: boolean; account?: string };
// External is a conversation started in a harness, outside Dory.
// usable says whether a provider can run threads: installed and not turned off.
export const usable = (p: Provider) => p.installed && p.enabled !== false;
export type External = { provider: string; id: string; title: string; dir: string; branch?: string; updated_at: string; project_id: string | null; thread_id?: string };
export type Job = { id: string; provider: string; kind: "login" | "update" | "install"; output: string; done: boolean; accepts_code?: boolean; error?: string };
export type Skill = { id: string; name: string; description: string; body_md: string; files: Record<string, string>; version: number; targets: { nodes: string[] | null; harnesses: string[] | null }; source: "" | "system"; system_version: number; forked_from: string | null; disabled_at: string | null; created_at: string; updated_at: string; archived_at: string | null };
export type SkillConflict = { harness: string; path: string; kind: "link" | "foreign" | "edited"; skill_md: string; orphan?: boolean };
export type SkillState = { skill_id: string; node_id: string; status: "synced" | "pending" | "error" | "conflict" | "removed"; version: number; message: string; updated_at: string };
export type UsageWindow = { kind: string; utilization: number; resets_at: string; label?: string; window_minutes?: number };
export type NodeInfo = { id: string; name: string; os: string; version: string; data_dir: string; listen: string; platform: string; icon?: string };
export type Peer = { id: string; name: string; address: string; fingerprint: string; connected: boolean; last_seen: string; revoked: boolean; info: { node?: NodeInfo; providers?: Provider[] } };
export type Mesh = { node_id: string; fingerprint: string; port: number; peers: Peer[]; discovered: { id: string; name: string; address: string }[] };
export type UsageTokens = { input: number; output: number; cache_write: number; cache_read: number };
export type UsageCost = UsageTokens & { cache_saving: number };
export type UsageRow = { bucket: string; provider: string; model: string; sessions: number; tokens: UsageTokens; cost: UsageCost; priced: boolean };
export type UsageRange = "24h" | "7d" | "30d" | "90d";
export type UsageBreakdownRow = UsageTokens & { thread: string; title: string; provider: string; category: string; label: string; cost: number; priced_tokens: number };
export type UsageTurnCall = UsageTokens & { ordinal: number; model: string; reasoning: number; subagent: boolean };
export type UsageTurn = { id: string; provider: string; created_at: string; preview: string; calls: UsageTurnCall[] };
export type UsageReport = { from: string; bucket: "hour" | "day"; rows: UsageRow[]; sessions: Record<string, number> };
export type NodeStatus = { node: NodeInfo; providers: Provider[]; usage: Record<string, UsageWindow[]>; peers?: Peer[] };
export type Hit = { entity: "task" | "memory"; id: string; title: string; rank: number; record: Task | Memory };
export type User = { id: string; username: string };
export type Version = { revision: string; short: string; time: string; modified: boolean };

export type GithubAuth = { source: "env" | "ui" | "gh" | "none"; login: string };
export type GithubDeviceCode = { user_code: string; verification_uri: string; expires_in: number };
export type GithubDeviceState = { state: "pending" | "done" | "expired" | "denied" | "failed"; error?: string };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
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
  pairCode: () => req<{ code: string; expires_at: string; fingerprint: string; port: number }>("POST", "/mesh/pair/code"),
  pair: (address: string, code: string) => req<Peer>("POST", "/mesh/pair", { address, code }),
  revokePeer: (id: string) => req<void>("DELETE", `/mesh/peers/${id}`),
  devices: () => req<{ id: string; name: string; created_at: string }[]>("GET", "/devices"),
  revokeDevice: (id: string) => req<void>("DELETE", `/devices/${id}`),
  folders: (node: string, path = "", hidden = false) => req<{ path: string; parent: string; home: string; folders: { name: string; path: string; git: boolean }[] }>("GET", `/nodes/${node}/folders?${new URLSearchParams({ path, ...(hidden && { hidden: "true" }) })}`),
  checkout: (node: string, path: string) => req<{ path: string; name: string; git: boolean; remote: string; default_branch: string }>("GET", `/nodes/${node}/checkout?path=${encodeURIComponent(path)}`),
  setNodeRepositoryPath: (node: string, id: string, path: string) => req<void>("PUT", `/nodes/${node}/repositories/${id}/path`, { path }),
  externalSessions: (node: string) => req<External[]>("GET", `/nodes/${node}/external`),
  externalSession: (node: string, provider: string, id: string) => req<{ session: External; messages: Message[] }>("GET", `/nodes/${node}/external/${provider}/${id}`),
  adoptSession: (node: string, provider: string, id: string) => req<{ thread_id: string }>("POST", `/nodes/${node}/external/${provider}/${id}/adopt`),
  nodeProviders: (node: string) => req<Provider[]>("GET", `/nodes/${node}/providers`),
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
  createThread: ({ node, ...body }: NewThread) => req<{ thread_id: string }>("POST", `/nodes/${node || "local"}/threads`, body),
  send: (id: string, text: string, files: string[] = []) => req<void>("POST", `/threads/${id}/messages`, { text, files }),
  upload: async (id: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`/api/threads/${id}/attachments`, { method: "POST", credentials: "same-origin", body: form });
    if (!res.ok) { const e = await res.json().catch(() => null); throw new ApiError(res.status, e?.error?.message ?? res.statusText); }
    return (await res.json()) as Attachment;
  },
  attachmentUrl: (id: string, file: string) => `/api/threads/${id}/attachments/${file}`,
  queue: (id: string, run: string, action: "edit" | "cancel" | "steer", text?: string) => req<void>("POST", `/threads/${id}/queue/${run}`, { action, text }),
  threadSettings: (id: string, body: { provider?: string; model?: string; effort?: string; runtime_mode?: Thread["runtime_mode"] }) => req<Thread>("POST", `/threads/${id}/settings`, body),
  nodeCommands: (node: string, provider: string, project?: string) => req<{ commands: Entry[]; skills: Entry[] }>("GET", `/nodes/${node || "local"}/commands?provider=${encodeURIComponent(provider)}&project=${encodeURIComponent(project ?? "")}`),
  threadCommands: (id: string) => req<{ commands: Entry[]; skills: Entry[] }>("GET", `/threads/${id}/commands`),
  threadFiles: (id: string, q: string) => req<string[]>("GET", `/threads/${id}/files?q=${encodeURIComponent(q)}`),
  answer: (id: string, request: string, answers: string[][], files: string[][] = []) => req<void>("POST", `/threads/${id}/requests/${encodeURIComponent(request)}`, { answers, files }),
  interrupt: (id: string) => req<void>("POST", `/threads/${id}/interrupt`),
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
