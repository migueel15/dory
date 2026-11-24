// Query keys of the web, in one place. Every key starts with the name of its
// resource, so the call without arguments is the prefix that refreshes the whole
// family (qk.board() refreshes every board, qk.board("p1") only that one).

// key builds the function of a family: the arguments are optional so the same
// function also gives the prefix.
const key = <A extends unknown[]>(name: string) => (...args: Partial<A>) => [name, ...args] as unknown[];

export const qk = {
  setup: key<[]>("setup"),
  version: key<[]>("version"),
  me: key<[]>("me"),
  node: key<[]>("node"),
  mesh: key<[]>("mesh"),
  meshUpdate: key<[]>("mesh-update"),
  devices: key<[node: string]>("devices"),
  deviceState: key<[node: string, serial: string]>("device-state"),
  deviceInput: key<[node: string, serial: string]>("device-input"),
  job: key<[id: string | null]>("job"),
  titleModel: key<[]>("title-model"),
  keptWorktrees: key<[]>("kept-worktrees"),
  notices: key<[]>("notices"),
  noticeSettings: key<[]>("notice-settings"),
  speechModels: key<[]>("speech-models"),

  nodeSettings: key<[node: string]>("node-settings"),
  nodeDefaults: key<[node: string]>("node-defaults"),
  providers: key<[node: string]>("node-providers"),

  projects: key<[]>("projects"),
  project: (id: string) => ["projects", id],
  repositories: key<[]>("repositories"),
  repositoriesOf: (project: string) => ["repositories", project],
  repositoryPaths: () => ["repositories", "paths"],
  repositoryLocations: key<[]>("repository-locations"),
  projectWorktrees: key<[node: string, project: string]>("project-worktrees"),
  projectBranches: key<[node: string, project: string]>("project-branches"),
  folders: key<[node: string | undefined, dir: string, hidden: boolean]>("folders"),
  checkout: key<[node: string | undefined, path: string | null]>("checkout"),

  board: key<[project: string]>("board"),
  card: key<[id: string | null | undefined]>("card"),
  dependencies: key<[id: string]>("dependencies"),
  plans: key<[taskId: string]>("plans"),
  planDoc: (taskId: string | null | undefined) => ["plans", taskId, "doc"],
  memories: key<[]>("memories"),

  threads: key<[filter: unknown]>("threads"),
  threadsAttention: () => ["threads", "attention"],
  threadChildren: (id: string) => ["threads", "children", id],
  threadRelated: (id: string) => ["threads", "related", id],
  thread: key<[id: string]>("thread"),
  threadTitle: key<[id: string]>("thread-title"),
  threadTerminals: key<[id: string]>("thread-terminals"),
  threadServices: key<[id: string]>("thread-services"),
  threadDiff: key<[id: string, base: string]>("thread-diff"),
  external: key<[node: string, provider?: string, id?: string]>("external"),
  commands: (...source: unknown[]) => ["commands", ...source],
  files: (...source: unknown[]) => ["files", ...source],

  skills: key<[]>("skills"),
  skillStates: () => ["skills", "states"],
  skillConflicts: (node: string, id: string) => ["skills", "conflicts", node, id],

  usageHistory: key<[range: string, node: string]>("usage-history"),
  usageBreakdown: key<[range: string, node: string]>("usage-breakdown"),
  usageTurns: (node: string, thread: string, range: string) => ["usage-breakdown", "turns", node, thread, range],

  github: key<[]>("github"),
  githubAuth: key<[]>("github-auth"),
  githubDevice: key<[]>("github-device"),
  githubDetail: key<[repo: string, kind: string, number: number]>("github-detail"),
  githubClosed: key<[tab: string, project: string]>("github-closed"),
  githubDiff: key<[repo: string, number: number]>("github-diff"),
  pullReviews: key<[repo: string, number: number]>("pull-reviews"),
  pullAgent: key<[repo: string, number: number]>("pull-agent"),
  paletteTasks: key<[text: string]>("palette-tasks"),
  paletteGithub: key<[text: string]>("palette-github"),
};
