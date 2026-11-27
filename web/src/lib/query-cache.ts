// The query cache survives a restart of the app: on open it paints what it
// showed last time at once and refreshes it in the background, instead of
// waiting for the network. Only small, near-static families are kept, in
// IndexedDB, and logging out deletes them.
import type { Query, QueryClient } from "@tanstack/react-query";
import type { PersistedClient, Persister, PersistQueryClientOptions } from "@tanstack/react-query-persist-client";

// kept lists the families saved across restarts: what the shell, the sidebar
// and the model picker need to paint.
const kept = new Set([
  "me", "node", "mesh", "node-providers", "node-settings", "node-defaults", "title-model",
  "projects", "repositories", "repository-locations", "notices", "board",
]);
const keep = (q: Query) =>
  q.state.status === "success" && (kept.has(String(q.queryKey[0])) || (q.queryKey[0] === "threads" && q.queryKey[1] === "attention"));

// maxAge is how old a saved cache can be and still paint the first screen.
const maxAge = 7 * 24 * 60 * 60_000;

// stable families change only by this user's actions or come with their own
// event: they count as fresh for longer.
const stable = ["node-settings", "node-defaults", "node-providers", "title-model", "projects", "repositories", "repository-locations"];

// setCacheDefaults lets the kept families live as long as the saved cache
// (a query collected from memory would also leave the saved copy) and gives
// the stable ones a longer freshness.
export function setCacheDefaults(qc: QueryClient) {
  for (const k of kept) qc.setQueryDefaults([k], { gcTime: maxAge, ...(stable.includes(k) && { staleTime: 5 * 60_000 }) });
  qc.setQueryDefaults(["threads", "attention"], { gcTime: maxAge });
}

const db = "dory-cache", store = "queries", slot = "client";
let opening: Promise<IDBDatabase> | undefined;
const open = () => (opening ??= new Promise((resolve, reject) => {
  const r = indexedDB.open(db, 1);
  r.onupgradeneeded = () => r.result.createObjectStore(store);
  r.onsuccess = () => resolve(r.result);
  r.onerror = () => reject(r.error);
}));
function run<T>(mode: IDBTransactionMode, op: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return open().then((d) => new Promise<T>((resolve, reject) => {
    const tx = d.transaction(store, mode);
    const r = op(tx.objectStore(store));
    tx.oncomplete = () => resolve(r.result);
    tx.onerror = () => reject(tx.error);
  }));
}

// Writes come in bursts (every query change): only the last one of each
// second is stored.
let pending: PersistedClient | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;
const write = () => {
  timer = undefined;
  const c = pending;
  pending = undefined;
  if (c) run("readwrite", (s) => s.put(JSON.stringify(c), slot)).catch(() => {});
};

export const persister: Persister = {
  persistClient: (c) => { pending = c; timer ??= setTimeout(write, 1_000); },
  restoreClient: () => run<string | undefined>("readonly", (s) => s.get(slot))
    .then((raw) => (raw ? (JSON.parse(raw) as PersistedClient) : undefined))
    .catch(() => undefined),
  removeClient: () => {
    clearTimeout(timer);
    timer = undefined;
    pending = undefined;
    return run("readwrite", (s) => s.delete(slot)).then(() => {}, () => {});
  },
};

// The cache of another build may not match its types: each build keeps its own.
const buster = new URL(import.meta.url).pathname;

export const persistOptions: Omit<PersistQueryClientOptions, "queryClient"> = {
  persister, maxAge, buster, dehydrateOptions: { shouldDehydrateQuery: keep },
};

// forget empties the cache in memory and on disk, after logging out or when
// the session is no longer valid.
export function forget(qc: QueryClient) {
  qc.clear();
  void persister.removeClient();
}
