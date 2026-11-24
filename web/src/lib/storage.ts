// The only place that touches localStorage. Preferences are per browser, so
// every access tolerates a missing or blocked storage and falls back to the
// default. The key names are the ones the app has always used, to keep the
// preferences already saved.

export const storageKeys = {
  uiScale: "ui-scale",
  theme: "theme",
  project: "project",
  sidebarFolded: "dory.sidebar-folded",
  threadsFolded: "dory.threads-folded",
  threadsWidth: "dory.threads-width",
  diffView: "dory.diff-view",
  githubTab: "github-tab",
  uiFont: "dory.ui-font",
  terminalFont: "dory.terminal-font",
  uiFontSize: "dory.ui-font-size",
  terminalFontSize: "dory.terminal-font-size",
  panelWidth: "dory.thread-panel-width",
  deviceQuality: "dory.device-quality",
  planFollow: "dory.plan-follow",
  lastModel: "dory.last-model",
  shortcuts: "dory.shortcuts",
  dictationModel: "dory.dictation-model",
  dictationShortcut: "dory.dictation-shortcut",
  usageProvider: "dory-usage-provider",
  launch: (section: string) => `dory.launch.${section}`,
  lastModelOf: (section: string) => `dory.last-model:${section}`,
  effort: (provider: string, model: string) => `effort:${provider}:${model}`,
  effortLegacy: (slug: string) => `effort:${slug}`,
  threadPanel: (threadId: string) => `dory-thread-panel:${threadId}`,
  threadInfo: (threadId: string) => `dory-thread-info:${threadId}`,
  threadTask: (threadId: string) => `dory-thread-task:${threadId}`,
  deviceFloat: (id: string) => `dory.device-float:${id}`,
} as const;

export const storage = {
  get(key: string): string | null {
    try { return localStorage.getItem(key); } catch { return null; }
  },
  // set reports whether the value was saved.
  set(key: string, value: string): boolean {
    try { localStorage.setItem(key, value); return true; } catch { return false; }
  },
  remove(key: string) {
    try { localStorage.removeItem(key); } catch { /* per-browser only */ }
  },
  getOr(key: string, fallback: string): string {
    return storage.get(key) ?? fallback;
  },
  getJSON<T>(key: string, fallback: T): T {
    try { const raw = storage.get(key); return raw === null ? fallback : (JSON.parse(raw) as T); } catch { return fallback; }
  },
  setJSON(key: string, value: unknown): boolean {
    return storage.set(key, JSON.stringify(value));
  },
};
