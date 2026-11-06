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
export type T3ThreadContext = S["T3ThreadContext"];
export type GithubIssue = S["GithubIssue"];
export type GithubPull = S["GithubPull"];
export type GithubDetail = S["GithubDetail"];
export type GithubThread = S["GithubThread"];
export type ItemStatus = Item["status"];
export type ItemPlan = S["ItemPlan"];
export type PlanSections = S["PlanSections"];
export type ItemRefinement = S["ItemRefinement"];
export type RefineInput = { text?: string; restart?: boolean; instance_id?: string; project_id?: string; provider?: string; model?: string; effort_option?: string; effort?: string };

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
  providers: { instance_id: string; name: string; driver: string; models: T3CatalogModel[] }[];
  projects: { id: string; title: string; root: string }[];
};
export type T3CatalogModel = { slug: string; name: string; effort?: { id: string; values: string[]; default: string } };
export type IssueDefineInput = { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string };
export type IssueThread = S["IssueThread"];
export type T3LaunchInput = { instance_id: string; project_id: string; provider: string; model: string; role: "triage" | "implementation" | "replan"; feedback?: string; worktree: boolean; base_branch?: string; effort_option?: string; effort?: string };

export const api = {
  itemThreads: resource<S["ItemThread"], S["ItemThreadInput"]>("/item-threads"),
  github: (refresh = false) => req<S["GithubOverview"]>("GET", `/github/open${refresh ? "?refresh=true" : ""}`),
  githubClosed: (kind: "pulls" | "issues", project: string, cursor: string) =>
    req<S["GithubClosed"]>("GET", `/github/closed?${new URLSearchParams({ kind, ...(project && { project }), ...(cursor && { cursor }) })}`),
  githubDetail: (repo: string, kind: string, number: number) => req<GithubDetail>("GET", `/github/repos/${repo}/${kind}/${number}`),
  githubAct: (repo: string, kind: string, number: number, action: S["GithubAction"]) => req<void>("POST", `/github/repos/${repo}/${kind}/${number}/actions`, action),
  githubDiff: (repo: string, number: number) => req<{ diff: string }>("GET", `/github/repos/${repo}/pulls/${number}/diff`),
  t3Pair: (id: string) => req<{ url: string }>("POST", `/t3/instances/${id}/pair`),
  t3Context: (id: string) => req<T3ThreadContext>("GET", `/t3/threads/${id}/context`),
  t3Send: (id: string, text: string) => req<void>("POST", `/t3/threads/${id}/messages`, { text }),
  t3Respond: (id: string, requestId: string, body: { decision?: "accept" | "decline"; answers?: Record<string, unknown> }) =>
    req<void>("POST", `/t3/threads/${id}/requests/${encodeURIComponent(requestId)}`, body),
  t3Catalog: (id: string) => req<T3Catalog>("GET", `/t3/instances/${id}/catalog`),
  item: (id: string) => req<Item>("GET", `/items/${id}`),
  refinement: (id: string) => req<ItemRefinement>("GET", `/items/${id}/refinement`),
  refine: (id: string, body: RefineInput) => req<{ thread_id: string }>("POST", `/items/${id}/refine`, body),
  plans: (id: string) => req<ItemPlan[]>("GET", `/items/${id}/plans`),
  createPlan: (id: string, body: { sections: PlanSections; open_questions: string[] }) => req<ItemPlan>("POST", `/items/${id}/plans`, body),
  acceptPlan: (id: string) => req<{ item: Item; plan: ItemPlan }>("POST", `/items/${id}/plan/accept`),
  t3Launch: (itemId: string, body: T3LaunchInput) => req<{ thread_id: string }>("POST", `/items/${itemId}/launch`, body),
  resolveConflicts: (repo: string, number: number, body: IssueDefineInput) => req<{ thread_id: string }>("POST", `/github/repos/${repo}/pulls/${number}/resolve-conflicts`, body),
  issueDefine: (repo: string, number: number, body: IssueDefineInput) => req<{ thread_id: string }>("POST", `/github/repos/${repo}/issues/${number}/define`, body),
  branches: (repo: string) => req<string[]>("GET", `/github/repos/${repo}/branches`),
  issueThreads: () => req<IssueThread[]>("GET", "/issue-threads?limit=200"),
  t3Status: () => req<T3Status[]>("GET", "/t3/status"),
  t3Usage: () => req<T3InstanceUsage[]>("GET", "/t3/usage"),
  t3UsageRefresh: () => req<T3InstanceUsage[]>("POST", "/t3/usage/refresh"),
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
  ...patch,
});
