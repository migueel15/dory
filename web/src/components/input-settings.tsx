import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Warning } from "@phosphor-icons/react";
import { api, type InputSettings } from "../api/client";
import { dictationShortcutEvent, dictationShortcutKey } from "./dictation-shortcut";

// Shortcuts and the dictation model live in the settings of each node
// (NodeSettings.input). The running app is a client of the node that serves it
// ("local"), so it follows that node's values. The browser keeps a copy of them
// in localStorage so that key handlers can read them synchronously; the copy is
// refreshed every time the settings of the local node load or change.
export const shortcutsKey = "dory.shortcuts";
export const shortcutsEvent = "dory:shortcuts";
export const modelKey = "dory.dictation-model";
export const modelEvent = "dory:dictation-model";

export const emptyInput: InputSettings = { shortcuts: {}, dictation: { model: "" } };

const read = (key: string) => { try { return localStorage.getItem(key); } catch { return null; } };

// fromBrowser is what this browser had chosen before the choices moved to the
// node, or undefined when it never customised anything.
function fromBrowser(): InputSettings | undefined {
  const shortcuts = read(shortcutsKey), dictation = read(dictationShortcutKey), model = read(modelKey);
  if (!shortcuts && !dictation && !model) return undefined;
  const input: InputSettings = { shortcuts: {}, dictation: { model: model ?? "" } };
  try { const value = JSON.parse(shortcuts ?? "{}"); if (value && typeof value === "object" && !Array.isArray(value)) input.shortcuts = value; } catch { /* ignored */ }
  try { if (dictation) input.shortcuts.dictation = JSON.parse(dictation); } catch { /* ignored */ }
  return input;
}

function mirror(key: string, value: string | null, event: string) {
  if (read(key) === value) return;
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); } catch { return; }
  window.dispatchEvent(new Event(event));
}

function applyLocal(input: InputSettings) {
  const { dictation, ...rest } = input.shortcuts;
  mirror(shortcutsKey, Object.keys(rest).length ? JSON.stringify(rest) : null, shortcutsEvent);
  mirror(dictationShortcutKey, dictation ? JSON.stringify(dictation) : null, dictationShortcutEvent);
  mirror(modelKey, input.dictation.model || null, modelEvent);
}

let migrating = false;

// useInputSettingsSync keeps the browser copy equal to the local node's
// settings. A node that has never saved them takes what this browser had
// chosen, once; the other nodes keep the defaults.
export function useInputSettingsSync() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ["node-settings", "local"], queryFn: () => api.nodeSettings("local"), staleTime: 30_000, retry: false });
  useEffect(() => {
    if (!data) return;
    if (data.input) { applyLocal(data.input); return; }
    const old = fromBrowser();
    if (!old || migrating) return;
    migrating = true;
    api.setNodeSettings({ ...data, input: old }, "local")
      .then((saved) => qc.setQueryData(["node-settings", "local"], saved))
      .catch(() => {})
      .finally(() => { migrating = false; });
  }, [data, qc]);
}

// ValueDiff is the warning icon of a value that differs between nodes; the
// tooltip lists node → value. It renders nothing when they agree.
export function ValueDiff({ items }: { items: { label: string; text: string }[] }) {
  if (new Set(items.map((i) => i.text)).size < 2) return null;
  const tip = items.map((i) => `${i.label}: ${i.text}`).join(" · ");
  return <span title={tip} aria-label={`Difiere entre nodos. ${tip}`} className="shrink-0 text-amber-500"><Warning weight="fill" className="size-4" aria-hidden="true" /></span>;
}
