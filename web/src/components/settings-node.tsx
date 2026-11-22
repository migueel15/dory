import { useMemo, type ReactNode } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { Warning } from "@phosphor-icons/react";
import { api, type NodeSettings } from "../api/client";
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
//   settings.set(patch) writes the patch to every node ("all") or to one.
// - SettingRow draws a checkbox row for a boolean field, or whatever `control`
//   returns for other kinds of field, plus the warning icon when nodes differ.
// - NodeDiff is the icon alone, for rows that SettingRow does not fit.
// A difference is a notice, never a conflict: nothing is blocked or merged.
//
// To add a field, add it to NodeSettings (api/client.ts) and render a SettingRow.

export type SectionProps = { node: string };
export const ALL_NODES = "all";

export type NodeSettingsView = ReturnType<typeof useNodeSettings>;

export function useNodeSettings(node: string) {
  const qc = useQueryClient();
  const nodes = useNodes();
  const targets = node === ALL_NODES ? nodes : nodes.filter((n) => n.value === node);
  const queries = useQueries({ queries: targets.map((n) => ({ queryKey: ["node-settings", n.value], queryFn: () => api.nodeSettings(n.value) })) });
  const save = useMutation({
    mutationFn: (patch: Partial<NodeSettings>) => Promise.all(targets.map((n, i) => {
      const current = queries[i].data;
      return current ? api.setNodeSettings({ ...current, ...patch }, n.value).then((data) => qc.setQueryData(["node-settings", n.value], data)) : Promise.resolve();
    })),
    onSettled: () => qc.invalidateQueries({ queryKey: ["node-settings"] }),
  });
  const loaded = queries.length > 0 && queries.every((q) => q.data);
  // values lists what each target node has for a field.
  const values = <K extends keyof NodeSettings>(field: K) => targets.flatMap((n, i) => queries[i].data ? [{ node: n.value, label: n.label, value: queries[i].data![field] }] : []);
  return {
    loaded,
    pending: save.isPending,
    error: queries.find((q) => q.error)?.error ?? save.error,
    values,
    get: <K extends keyof NodeSettings>(field: K) => values(field)[0]?.value,
    differs: (field: keyof NodeSettings) => new Set(values(field).map((v) => JSON.stringify(v.value ?? null))).size > 1,
    set: (patch: Partial<NodeSettings>) => save.mutate(patch),
  };
}

const show = (v: unknown) => (typeof v === "boolean" ? (v ? "on" : "off") : String(v ?? "—"));

// NodeDiff is the warning icon of a field whose value differs between nodes. Its
// tooltip lists node → value. It renders nothing when the nodes agree.
export function NodeDiff({ settings, field, format = show }: { settings: NodeSettingsView; field: keyof NodeSettings; format?: (v: unknown) => string }) {
  const tip = useMemo(() => settings.values(field).map((v) => `${v.label}: ${format(v.value)}`).join(" · "), [settings, field, format]);
  if (!settings.differs(field)) return null;
  return <span title={tip} aria-label={`Difiere entre nodos. ${tip}`} className="shrink-0 text-amber-500"><Warning weight="fill" className="size-4" aria-hidden="true" /></span>;
}

export function SettingRow<K extends keyof NodeSettings>({ settings, field, title, help, control }: {
  settings: NodeSettingsView; field: K; title: string; help?: ReactNode;
  control?: (value: NodeSettings[K] | undefined, set: (v: NodeSettings[K]) => void, disabled: boolean) => ReactNode;
}) {
  const value = settings.get(field);
  const disabled = !settings.loaded || settings.pending;
  const set = (v: NodeSettings[K]) => settings.set({ [field]: v } as Partial<NodeSettings>);
  return (
    <div className="flex items-start gap-3 text-sm">
      {control ? control(value, set, disabled) : <input type="checkbox" className="mt-1" aria-label={title} checked={!!value} disabled={disabled} onChange={(e) => set(e.target.checked as NodeSettings[K])} />}
      <span className="min-w-0 flex-1"><strong className="block font-medium">{title}</strong>{help && <span className="text-slate-600 dark:text-muted">{help}</span>}</span>
      <NodeDiff settings={settings} field={field} />
    </div>
  );
}
