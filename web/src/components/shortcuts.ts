import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { chordFromEvent, openShortcutPopups, readDictationShortcut, type Chord } from "../features/composer/dictation-shortcut";
import { shortcutsEvent, shortcutsKey, useInputSettingsSync } from "../features/settings/input-settings";

const mac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);
const chord = (code: string, shift = false): Chord => ({ code: `Key${code}`, ctrl: !mac, meta: mac, alt: false, shift });
export const commands = [
  { id: "new-thread", label: "Nuevo hilo", default: chord("N") },
  { id: "search-threads", label: "Buscar hilos", default: chord("F", true) },
  { id: "palette", label: "Paleta de comandos", default: chord("K") },
  { id: "new-idea", label: "Nueva idea", default: chord("I") },
  { id: "model", label: "Selector de modelo", default: chord("M", true) },
  { id: "plan", label: "Modo plan", default: chord("P", true) },
  { id: "branch", label: "Selector de rama", default: chord("B", true) },
  { id: "project", label: "Selector de proyecto", default: chord("O", true) },
  { id: "sidebar", label: "Barra lateral principal", default: chord("L", true) },
  { id: "thread-list", label: "Lista lateral de hilos", default: chord("H", true) },
  { id: "thread-panel", label: "Panel lateral del hilo", default: chord("J", true) },
  { id: "dictation", label: "Dictado", default: { code: "KeyD", ctrl: true, meta: false, alt: false, shift: false } },
] as const;
export type CommandId = typeof commands[number]["id"];
const storage = shortcutsKey;
const eventName = shortcutsEvent;
const read = () => { try { return localStorage.getItem(storage) ?? "{}"; } catch { return "{}"; } };
function preferences(): Partial<Record<CommandId, Chord>> {
  try { const value = JSON.parse(read()); return value && typeof value === "object" && !Array.isArray(value) ? value : {}; } catch { return {}; }
}
export function sameChord(a: Chord, b: Chord) {
  return a.code === b.code && a.ctrl === b.ctrl && a.alt === b.alt && a.shift === b.shift && a.meta === b.meta;
}
export function shortcut(id: CommandId): Chord {
  if (id === "dictation") return readDictationShortcut();
  const fallback = commands.find((c) => c.id === id)!.default;
  const value = preferences()[id];
  return value && /^Key[A-Z]$|^Digit[0-9]$/.test(value.code) && [value.ctrl, value.meta, value.alt, value.shift].every((x) => typeof x === "boolean") && (value.ctrl || value.meta || value.alt) ? value : fallback;
}
export function conflict(id: CommandId, value: Chord) {
  return commands.find((c) => c.id !== id && sameChord(shortcut(c.id), value));
}
export function reserved(value: Chord) {
  const primary = mac ? value.meta : value.ctrl;
  if (!primary || value.alt) return false;
  return value.code === "Digit0" || ["KeyA", "KeyC", "KeyV", "KeyX", "KeyZ", "KeyY"].includes(value.code);
}
// chordIn is the chord of a command in the shortcuts of a node, or its default.
export function chordIn(map: Record<string, Chord> | undefined, id: CommandId): Chord {
  const value = map?.[id];
  return value && /^Key[A-Z]$|^Digit[0-9]$/.test(value.code) && [value.ctrl, value.meta, value.alt, value.shift].every((x) => typeof x === "boolean") && (value.ctrl || value.meta || value.alt) ? value : commands.find((c) => c.id === id)!.default;
}
export function conflictIn(map: Record<string, Chord> | undefined, id: CommandId, value: Chord) {
  return commands.find((c) => c.id !== id && sameChord(chordIn(map, c.id), value));
}
const subscribe = (notify: () => void) => {
  for (const event of [eventName, "storage", "dory:dictation-shortcut"]) window.addEventListener(event, notify);
  return () => { for (const event of [eventName, "storage", "dory:dictation-shortcut"]) window.removeEventListener(event, notify); };
};
export function useShortcuts() {
  useInputSettingsSync();
  useSyncExternalStore(subscribe, () => read() + "|" + JSON.stringify(readDictationShortcut()) + "|" + registryVersion);
  return shortcut;
}
let registryVersion = 0;
const handlers = new Map<CommandId, Set<() => boolean | void>>();
export function available(id: CommandId) { return !!handlers.get(id)?.size; }
export function runCommand(id: CommandId) {
  for (const handler of handlers.get(id) ?? []) if (handler() !== false) return true;
  return false;
}
export function useCommand(id: CommandId, callback: () => boolean | void, enabled = true) {
  const latest = useRef(callback);
  useLayoutEffect(() => { latest.current = callback; });
  useEffect(() => {
    if (!enabled) return;
    const entries = handlers.get(id) ?? new Set<() => boolean | void>();
    const handler = () => latest.current();
    entries.add(handler); handlers.set(id, entries);
    registryVersion++; window.dispatchEvent(new Event(eventName));
    return () => { entries.delete(handler); registryVersion++; window.dispatchEvent(new Event(eventName)); };
  }, [id, enabled]);
}
export function listenShortcuts(e: KeyboardEvent) {
  if (e.defaultPrevented || e.repeat || e.isComposing || e.getModifierState("AltGraph")) return;
  if (e.target instanceof Element && e.target.closest("[data-dictation-shortcut-capture]")) return;
  const pressed = chordFromEvent(e);
  if (!pressed) return;
  const command = commands.find((c) => c.id !== "dictation" && sameChord(shortcut(c.id), pressed));
  if (!command || sameChord(shortcut("dictation"), pressed)) return;
  const [dialog] = openShortcutPopups();
  if (dialog && command.id !== "new-idea" && !(command.id === "palette" && dialog.getAttribute("aria-label") === "Comandos")) return;
  if (runCommand(command.id)) { e.preventDefault(); e.stopPropagation(); }
}
