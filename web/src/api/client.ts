// Client of the Dory v2 node API, grouped by domain in the sibling files.
// Wire types come from types.gen.ts (generated from Go); only types of the web
// itself live in the domain files.
import type { Column, Provider } from "./types.gen";
import { authApi } from "./auth";
import { githubApi } from "./github";
import { memoriesApi } from "./memories";
import { meshApi } from "./mesh";
import { nodeApi } from "./node";
import { plansApi } from "./plans";
import { projectsApi } from "./projects";
import { providersApi } from "./providers";
import { skillsApi } from "./skills";
import { tasksApi } from "./tasks";
import { threadsApi } from "./threads";
import { usageApi } from "./usage";

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
export { ApiError, apiError, req } from "./http";
export type { Mesh, NodeInfo, Peer } from "./mesh";
export type { NodeStatus } from "./node";
export type { User } from "./auth";
export type { UsageRange } from "./usage";
export type { MemoryInput } from "./memories";
export type { TaskInput } from "./tasks";
export type { Launch, NewThread, ThreadActionName } from "./threads";
export type { GithubAction, GithubAuth, LaunchChoice, PullAgent, PullReview } from "./github";
export { devicesApi } from "./devices";
export type { AvdState, Device, DeviceList, DeviceTools } from "./devices";

// usable says whether a provider can run threads: installed and not turned off.
export const usable = (p: Provider) => p.installed && p.enabled !== false;

export const api = {
  ...meshApi, ...authApi, ...nodeApi, ...providersApi, ...skillsApi, ...usageApi, ...projectsApi,
  ...tasksApi, ...plansApi, ...memoriesApi, ...threadsApi, ...githubApi,
};

export const COLUMN_LABELS: Record<Column, string> = {
  inbox: "Inbox", ready: "Ready", working: "Working", needs_you: "Needs you",
  in_review: "In review", ready_to_merge: "Ready to merge", done: "Done", cancelled: "Cancelled",
};

// repoName extracts owner/name from a GitHub remote.
export const repoName = (remote: string) => remote.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? remote;

