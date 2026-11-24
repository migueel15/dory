import { useMemo, type ReactNode } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { Warning } from "@phosphor-icons/react";
import { api, type NodeDefaults, type NodeSettings } from "../api/client";
import { useNodes } from "./launch-picker";

// Per-node settings helper.
//
// Every section of the settings modal receives `{ node: string }`: "all" means
// every node, anything else is a node id ("local" is this one). A section whose
// values live in /nodes/{id}/settings uses this file instead of calling the API:
//
//   export function MySettings({ node }: SectionProps) {
//     const settings = useNodeSettings(node);
//     return (
//       <section>
//         <SettingRow settings={settings} field="agents_use_devices" title="..." help="..." />
//         <ErrorText error={settings.error} />
//       </section>
//     );
//   }
//
// - useNodeSettings(node) loads the settings of every node. The value it shows
//   for a field is the one of the selected node; with "all" it is the first
//   node's, and settings.differs(field) tells whether the nodes disagree.
//   A field is a top-level key ("agents_use_devices") or a path into a nested
//   group (["threads", "default_model"]); get/values/differs compare paths by
//   deep equality, so a group is never compared or shown as a whole by accident.
// - Writing never copies one node's values to the others. Each node gets the
//   change applied on top of ITS OWN current settings:
//     settings.set({ agents_use_devices: true })              // plain patch
//     settings.set((cur) => ({ input: { ...cur.input, x } }))  // from each node's own value
//     settings.setPath(["worktrees", "location"], value)       // deep-merges one nested field
//     settings.setPath(["input", "shortcuts", cmd], chord)
//   Use setPath (or the function form) for any field inside a nested group: a
//   plain patch with a whole group built from get() would overwrite the other
//   sub-fields of that group on every node with the first node's values.
// - SettingRow (`field` is a key or a path) draws a checkbox row for a boolean field, or whatever `control`
//   returns for other kinds of field, plus the warning icon when nodes differ.
// - NodeDiff is the icon alone, for rows that SettingRow does not fit.
// A difference is a notice, never a conflict: nothing is blocked or merged.
//
// To add a field, add it to api.Settings in Go, regenerate the types and render a SettingRow.

export type SectionProps = { node: string };
export const ALL_NODES = "all";

// A field of NodeSettings: a top-level key or a path into nested groups.
export type SettingField = keyof NodeSettings | readonly string[];
type FieldValue<F> = F extends keyof NodeSettings ? NodeSettings[F] : any; // eslint-disable-line @typescript-eslint/no-explicit-any
type Patch = Partial<NodeSettings> | ((current: NodeSettings) => Partial<NodeSettings>);

const toPath = (field: SettingField): readonly string[] => (typeof field === "string" ? [field] : (field as readonly string[]));
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const getIn = (obj: unknown, path: readonly string[]): unknown => path.reduce<unknown>((o, k) => (isObj(o) ? o[k] : undefined), obj);
// setIn returns a copy of obj with value at path, keeping every sibling key.
const setIn = (obj: unknown, path: readonly string[], value: unknown): unknown => {
  if (path.length === 0) return value;
  const base = isObj(obj) ? obj : {};
  return { ...base, [path[0]]: setIn(base[path[0]], path.slice(1), value) };
};
// stable stringify so key order does not make equal objects look different.
const canon = (v: unknown): string => JSON.stringify(v ?? null, (_, x) => (isObj(x) ? Object.fromEntries(Object.entries(x).sort(([a], [b]) => a.localeCompare(b))) : x));

export type NodeSettingsView = ReturnType<typeof useNodeSettings>;

export function useNodeSettings(node: string) {
  const qc = useQueryClient();
  const nodes = useNodes();
  const targets = node === ALL_NODES ? nodes : nodes.filter((n) => n.value === node);
  const queries = useQueries({ queries: targets.map((n) => ({ queryKey: ["node-settings", n.value], queryFn: () => api.nodeSettings(n.value) })) });
  const save = useMutation({
    mutationFn: (patch: Patch) => Promise.all(targets.map((n, i) => {
      const current = queries[i].data;
      return current ? api.setNodeSettings({ ...current, ...(typeof patch === "function" ? patch(current) : patch) }, n.value).then((data) => qc.setQueryData(["node-settings", n.value], data)) : Promise.resolve();
    })),
    onSettled: () => qc.invalidateQueries({ queryKey: ["node-settings"] }),
  });
  const loaded = queries.length > 0 && queries.every((q) => q.data);
  // values lists what each target node has for a field (key or path).
  const values = <F extends SettingField>(field: F) => targets.flatMap((n, i) => queries[i].data ? [{ node: n.value, label: n.label, value: getIn(queries[i].data, toPath(field)) as FieldValue<F> }] : []);
  return {
    loaded,
    pending: save.isPending,
    error: queries.find((q) => q.error)?.error ?? save.error,
    values,
    get: <F extends SettingField>(field: F) => values(field)[0]?.value,
    differs: (field: SettingField) => new Set(values(field).map((v) => canon(v.value))).size > 1,
    set: (patch: Patch) => save.mutate(patch),
    setPath: (path: readonly string[], value: unknown) => save.mutate((current) => setIn(current, path, value) as NodeSettings),
  };
}

const show = (v: unknown) => (typeof v === "boolean" ? (v ? "on" : "off") : String(v ?? "—"));

// NodeDiff is the warning icon of a field whose value differs between nodes. Its
// tooltip lists node → value. It renders nothing when the nodes agree.
export function NodeDiff({ settings, field, format = show }: { settings: NodeSettingsView; field: SettingField; format?: (v: unknown) => string }) {
  const tip = useMemo(() => settings.values(field).map((v) => `${v.label}: ${format(v.value)}`).join(" · "), [settings, field, format]);
  if (!settings.differs(field)) return null;
  return <span title={tip} aria-label={`Difiere entre nodos. ${tip}`} className="shrink-0 text-amber-500"><Warning weight="fill" className="size-4" aria-hidden="true" /></span>;
}

export function SettingRow<K extends SettingField>({ settings, field, title, help, control }: {
  settings: NodeSettingsView; field: K; title: string; help?: ReactNode;
  control?: (value: FieldValue<K> | undefined, set: (v: FieldValue<K>) => void, disabled: boolean) => ReactNode;
}) {
  const value = settings.get(field);
  const disabled = !settings.loaded || settings.pending;
  const set = (v: FieldValue<K>) => settings.setPath(toPath(field), v);
  return (
    <div className="flex items-start gap-3 text-sm">
      {control ? control(value, set, disabled) : <input type="checkbox" className="mt-1" aria-label={title} checked={!!value} disabled={disabled} onChange={(e) => set(e.target.checked as FieldValue<K>)} />}
      <span className="min-w-0 flex-1"><strong className="block font-medium">{title}</strong>{help && <span className="text-slate-600 dark:text-muted">{help}</span>}</span>
      <NodeDiff settings={settings} field={field} />
    </div>
  );
}

// useNodeDefault is the placeholder of a text setting: the value the selected
// node uses when the field is empty. With "all" it is the shared value when
// every node agrees, and `fallback` (a machine-independent text) otherwise.
export function useNodeDefault(node: string, pick: (d: NodeDefaults) => string, fallback: string): string {
  const nodes = useNodes();
  const ids = node === ALL_NODES ? nodes.map((n) => n.value) : [node];
  const results = useQueries({ queries: ids.map((id) => ({ queryKey: ["node-defaults", id], queryFn: () => api.nodeDefaults(id), staleTime: 300_000 })) });
  const exact = results.map((r) => (r.data ? pick(r.data) : undefined)).filter((v): v is string => !!v);
  if (!exact.length) return fallback;
  if (exact.every((v) => v === exact[0])) return exact[0];
  // Nodes with different users or homes may still agree relative to their home.
  const tilde = results.map((r) => r.data && tildeOf(pick(r.data), r.data.home));
  return tilde.every((v) => v && v === tilde[0]) ? tilde[0]! : fallback;
}

function tildeOf(path: string, home: string): string {
  return home && (path === home || path.startsWith(home + "/")) ? "~" + path.slice(home.length) : path;
}
