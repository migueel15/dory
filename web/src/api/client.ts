import type { components } from "./schema";

type S = components["schemas"];
export type Project = S["Project"];
export type ProjectInput = S["ProjectInput"];
export type Environment = S["Environment"];
export type EnvironmentInput = S["EnvironmentInput"];
export type Repository = S["Repository"];
export type Feature = S["Feature"];
export type FeatureInput = S["FeatureInput"];
export type Item = S["Item"];
export type ItemInput = Partial<S["ItemInput"]> & { title: string };
export type Memory = S["Memory"];
export type MemoryInput = S["MemoryInput"];
export type User = S["User"];
export type T3Status = S["T3Status"];
export type T3InstanceUsage = S["T3InstanceUsage"];
export type T3InstanceHistory = S["T3InstanceHistory"];
export type T3DayUsage = S["T3DayUsage"];
export type T3ThreadContext = S["T3ThreadContext"];
// Reasoning and tool entries are steps of the model between messages.
export type T3ViewMessage = { id: string; role: "user" | "assistant" | "system" | "reasoning" | "tool" | "interrupt" | "plan" | "handoff" | "html" | "secret"; text: string; streaming: boolean; created_at: string; title?: string; detail?: string; status?: string; attachments?: T3Attachment[]; mentions?: string[]; skills?: string[]; from?: string; to?: string; from_provider?: string; to_provider?: string; html?: { attachment_id: string; title: string; height: number } };
export type T3QueuedRun = { run_id: string; text: string; position: number; held: boolean };
export type T3ThreadView = {
  id: string; title: string; provider_id: string; model: string; effort: string; effort_option: string;
  interaction_mode: string; runtime_mode: string; branch: string; worktree_path: string; project_id: string;
  active_run_id: string; active_status: string; instance_id: string; interrupted: boolean;
  pull_requests: { repository: string; number: number; url: string }[];
  messages: T3ViewMessage[]; queue: T3QueuedRun[]; pending: T3ThreadContext["pending"];
  items: { item_id: string; role: string }[];
  history_cursor: string;
};
export type T3Attachment = { type: "image" | "file"; id: string; name: string; mime_type: string; size_bytes: number };
export type T3SendInput = { text: string; mode: "auto" | "steer"; provider_id?: string; model?: string; effort_option?: string; effort?: string; attachments?: T3Attachment[]; skills?: string[]; mentions?: string[] };
export type GithubIssue = S["GithubIssue"];
export type GithubPull = S["GithubPull"];
export type GithubDetail = S["GithubDetail"];
export type GithubThread = S["GithubThread"];
export type ItemStatus = Item["status"];
export type ItemPlan = S["ItemPlan"];
export type PlanSections = S["PlanSections"];
export type ItemRefinement = S["ItemRefinement"];
export type TitleSettings = S["TitleSettings"];
export type RefineInput = { text?: string; restart?: boolean; instance_id?: string; project_id?: string; provider?: string; model?: string; effort_option?: string; effort?: string };

export type PullReview = { path: string; patch_hash: string; reviewed_at: string };
export type PullAgent = { available: boolean; thread_id?: string; url?: string; t3_environment_id?: string };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
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

const q = (filter?: Record<string, unknown>) =>
  filter ? `?filter=${encodeURIComponent(JSON.stringify(filter))}&limit=1000` : "?limit=1000";

// Recurso CRUD genérico: list/create/update(PUT)/archive.
function resource<T extends { id: string }, I>(path: string) {
  return {
    list: (filter?: Record<string, unknown>) => req<T[]>("GET", path + q(filter)),
    create: (b: I) => req<T>("POST", path, b),
    update: (id: string, b: I) => req<T>("PUT", `${path}/${id}`, b),
    archive: (id: string) => req<void>("DELETE", `${path}/${id}`),
  };
}

export type T3Catalog = {
  providers: { instance_id: string; name: string; driver: string; models: T3CatalogModel[]; commands?: T3ComposerEntry[]; skills?: T3ComposerEntry[] }[];
  projects: { id: string; title: string; root: string; repo: string }[];
};
export type T3ComposerEntry = { name: string; description?: string };
export type T3CatalogModel = { slug: string; name: string; effort?: { id: string; values: string[]; default: string } };
export type IssueDefineInput = { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string };
export type IssueThread = S["IssueThread"];
export type T3LaunchInput = { instance_id: string; project_id: string; provider: string; model: string; role: "implementation"; worktree: boolean; base_branch?: string; effort_option?: string; effort?: string };

export const api = {
  itemThreads: resource<S["ItemThread"], S["ItemThreadInput"]>("/item-threads"),
  github: (refresh = false) => req<S["GithubOverview"]>("GET", `/github/open${refresh ? "?refresh=true" : ""}`),
  githubClosed: (kind: "pulls" | "issues", project: string, cursor: string) =>
    req<S["GithubClosed"]>("GET", `/github/closed?${new URLSearchParams({ kind, ...(project && { project }), ...(cursor && { cursor }) })}`),
  githubDetail: (repo: string, kind: string, number: number) => req<GithubDetail>("GET", `/github/repos/${repo}/${kind}/${number}`),
  githubAct: (repo: string, kind: string, number: number, action: S["GithubAction"]) => req<void>("POST", `/github/repos/${repo}/${kind}/${number}/actions`, action),
  githubDiff: (repo: string, number: number) => req<{ diff: string }>("GET", `/github/repos/${repo}/pulls/${number}/diff`),
  pullReviews: (repo: string, number: number) => req<PullReview[]>("GET", `/github/repos/${repo}/pulls/${number}/reviews`),
  markPullReview: (repo: string, number: number, path: string, patch_hash: string) => req<void>("PUT", `/github/repos/${repo}/pulls/${number}/reviews`, { path, patch_hash }),
  unmarkPullReview: (repo: string, number: number, path: string) => req<void>("DELETE", `/github/repos/${repo}/pulls/${number}/reviews?path=${encodeURIComponent(path)}`),
  pullAgent: (repo: string, number: number) => req<PullAgent>("GET", `/github/repos/${repo}/pulls/${number}/agent`),
  askPullAgent: (repo: string, number: number, body: { path: string; start: number; end: number; snippet: string; question: string; launch?: { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string } }) =>
    req<PullAgent>("POST", `/github/repos/${repo}/pulls/${number}/ask`, body),
  t3Pair: (id: string) => req<{ url: string }>("POST", `/t3/instances/${id}/pair`),
  t3Context: (id: string) => req<T3ThreadContext>("GET", `/t3/threads/${id}/context`),
  t3Send: (id: string, text: string) => req<void>("POST", `/t3/threads/${id}/messages`, { text }),
  t3View: (id: string) => req<T3ThreadView>("GET", `/t3/threads/${id}/view`),
  t3History: (id: string, cursor: string) => req<{ messages: T3ViewMessage[]; next_cursor: string; has_more: boolean }>("GET", `/t3/threads/${id}/history?cursor=${encodeURIComponent(cursor)}`),
  t3Diff: (id: string) => req<{ diff: string; source: "t3" | "git"; branch?: string; worktree?: string; base?: string }>("GET", `/t3/threads/${id}/diff`),
  githubPullRefs: (repo: string, q: string) => req<{ pulls: { number: number; title: string; state: "open" | "draft" | "merged" | "closed" }[] }>("GET", `/github/repos/${repo}/pull-refs?q=${encodeURIComponent(q)}`),
  t3Composer: (id: string, provider: string) => req<{ commands: T3ComposerEntry[]; skills: T3ComposerEntry[] }>("GET", `/t3/threads/${id}/composer?provider=${encodeURIComponent(provider)}`),
  t3Entries: (id: string, q: string) => req<{ entries: { path: string; kind: string }[] }>("GET", `/t3/threads/${id}/entries?q=${encodeURIComponent(q)}`),
  t3Dispatch: (id: string, body: T3SendInput) => req<void>("POST", `/t3/threads/${id}/send`, body),
  t3QueueEdit: (id: string, run: string, text: string) => req<void>("PATCH", `/t3/threads/${id}/queue/${run}`, { text }),
  t3QueueReorder: (id: string, run: string, before: string) => req<void>("POST", `/t3/threads/${id}/queue/${run}/reorder`, { before_run_id: before }),
  t3QueueSteer: (id: string, run: string) => req<void>("POST", `/t3/threads/${id}/queue/${run}/steer`),
  // t3Upload sends one file as the raw body; the result goes in t3Dispatch.
  t3Upload: async (id: string, file: File): Promise<T3Attachment> => {
    const res = await fetch(`/api/t3/threads/${id}/attachments?name=${encodeURIComponent(file.name || "adjunto")}`, {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": file.type || "application/octet-stream" }, body: file,
    });
    if (!res.ok) {
      const e = await res.json().catch(() => null);
      throw new ApiError(res.status, e?.error?.message ?? res.statusText);
    }
    return res.json();
  },
  t3Interrupt: (id: string) => req<void>("POST", `/t3/threads/${id}/interrupt`),
  t3QueueResume: (id: string) => req<void>("POST", `/t3/threads/${id}/queue/resume`),
  t3QueueCancel: (id: string, run: string) => req<void>("DELETE", `/t3/threads/${id}/queue/${run}`),
  t3Settings: (id: string, body: { interaction_mode?: string; runtime_mode?: string }) => req<void>("PATCH", `/t3/threads/${id}/settings`, body),
  t3Respond: (id: string, requestId: string, body: { decision?: "accept" | "decline"; answers?: Record<string, unknown> }) =>
    req<void>("POST", `/t3/threads/${id}/requests/${encodeURIComponent(requestId)}`, body),
  titleSettings: () => req<TitleSettings>("GET", "/settings/titles"),
  saveTitleSettings: (body: TitleSettings) => req<TitleSettings>("PUT", "/settings/titles", body),
  t3Catalog: (id: string) => req<T3Catalog>("GET", `/t3/instances/${id}/catalog`),
  item: (id: string) => req<Item>("GET", `/items/${id}`),
  regenerateTitle: (id: string) => req<Item>("POST", `/items/${id}/title/regenerate`),
  refinement: (id: string) => req<ItemRefinement>("GET", `/items/${id}/refinement`),
  refine: (id: string, body: RefineInput) => req<{ thread_id: string }>("POST", `/items/${id}/refine`, body),
  plans: (id: string) => req<ItemPlan[]>("GET", `/items/${id}/plans`),
  createPlan: (id: string, body: { sections: PlanSections; open_questions: string[] }) => req<ItemPlan>("POST", `/items/${id}/plans`, body),
  acceptPlan: (id: string) => req<{ item: Item; plan: ItemPlan }>("POST", `/items/${id}/plan/accept`),
  t3AnswerSecret: (threadId: string, itemId: string, body: { secret: string } | { decline: true }) => req<void>("POST", `/t3/threads/${threadId}/secrets/${encodeURIComponent(itemId)}`, body),
  t3CreateThread: (body: { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string; text?: string }) => req<{ thread_id: string }>("POST", "/t3/threads", body),
  t3Launch: (itemId: string, body: T3LaunchInput) => req<{ thread_id: string }>("POST", `/items/${itemId}/launch`, body),
  resolveConflicts: (repo: string, number: number, body: IssueDefineInput) => req<{ thread_id: string }>("POST", `/github/repos/${repo}/pulls/${number}/resolve-conflicts`, body),
  issueDefine: (repo: string, number: number, body: IssueDefineInput) => req<{ thread_id: string }>("POST", `/github/repos/${repo}/issues/${number}/define`, body),
  branches: (repo: string) => req<string[]>("GET", `/github/repos/${repo}/branches`),
  issueThreads: () => req<IssueThread[]>("GET", "/issue-threads?limit=200"),
  version: () => req<S["Version"]>("GET", "/version"),
  t3Status: () => req<T3Status[]>("GET", "/t3/status"),
  t3Usage: () => req<T3InstanceUsage[]>("GET", "/t3/usage"),
  t3UsageRefresh: () => req<T3InstanceUsage[]>("POST", "/t3/usage/refresh"),
  t3UsageHistory: (days: number) => req<T3InstanceHistory[]>("GET", `/t3/usage/history?days=${days}`),
  login: (username: string, password: string) => req<User>("POST", "/auth/login", { username, password }),
  logout: () => req<void>("POST", "/auth/logout"),
  me: () => req<User>("GET", "/auth/me"),
  projects: resource<Project, ProjectInput>("/projects"),
  environments: resource<Environment, EnvironmentInput>("/environments"),
  repositories: resource<Repository, S["RepositoryInput"]>("/repositories"),
  features: resource<Feature, FeatureInput>("/features"),
  items: resource<Item, ItemInput>("/items"),
  memories: resource<Memory, MemoryInput>("/memories"),
};

// Convierte un Item en su Input para PUT (que reemplaza el recurso).
export const itemInput = (i: Item, patch: Partial<ItemInput> = {}): ItemInput => ({
  id: i.id,
  project_id: i.project_id,
  feature_id: i.feature_id,
  raw_text: i.raw_text,
  title: i.title,
  body_md: i.body_md,
  progress: i.progress,
  refs: i.refs,
  tags: i.tags,
  status: i.status,
  blocked_reason: i.blocked_reason,
  priority: i.priority,
  source: i.source,
  title_source: i.title_source,
  ...patch,
});
