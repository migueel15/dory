import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { events } from "../../lib/store";
import { storage, storageKeys } from "../../lib/storage";

export type Chord = { code: string; ctrl: boolean; alt: boolean; shift: boolean; meta: boolean };
const defaultChord: Chord = { code: "KeyD", ctrl: true, alt: false, shift: false, meta: false };
export const dictationShortcutKey = storageKeys.dictationShortcut;
const key = dictationShortcutKey;
const validCode = /^(Key[A-Z]|Digit[0-9])$/;
const priorities = ["active", "selection", "editor", "panel"] as const;
type Context = typeof priorities[number];
type Target = (event: KeyboardEvent, context: Context) => boolean;
const targets = new Set<Target>();

function readSetting() {
  try { return storage.get(key) ?? ""; } catch { return ""; }
}

function readChord(raw: string): Chord {
  if (!raw) return defaultChord;
  try {
    const value = JSON.parse(raw) as Chord;
    if (typeof value.code === "string" && validCode.test(value.code) &&
      [value.ctrl, value.alt, value.shift, value.meta].every((modifier) => typeof modifier === "boolean") &&
      (value.ctrl || value.alt || value.meta)) return value;
  } catch {
    // Invalid or missing preferences use Ctrl+D.
  }
  return defaultChord;
}

export function chordFromEvent(event: Pick<KeyboardEvent, "code" | "ctrlKey" | "altKey" | "shiftKey" | "metaKey">): Chord | undefined {
  if (!validCode.test(event.code) || !(event.ctrlKey || event.altKey || event.metaKey)) return undefined;
  return { code: event.code, ctrl: event.ctrlKey, alt: event.altKey, shift: event.shiftKey, meta: event.metaKey };
}

export function formatChord(chord: Chord) {
  return [chord.ctrl && "Ctrl", chord.alt && "Alt", chord.shift && "Mayús", chord.meta && "⌘", chord.code.replace(/^(Key|Digit)/, "")].filter(Boolean).join("+");
}

const subscribe = (notify: () => void) => {
  const off = events.dictationShortcutChanged.on(notify);
  window.addEventListener("storage", notify);
  return () => { off(); window.removeEventListener("storage", notify); };
};
export const readDictationShortcut = () => readChord(readSetting());

export const useDictationShortcutSetting = () => readChord(useSyncExternalStore(subscribe, readSetting));

export function isTextInput(element: Element) {
  return !!element.closest("input, textarea, select, [contenteditable=true], [role=combobox]");
}

export function openShortcutPopups() {
  // Closed popovers retain their ARIA roles in the DOM.
  return Array.from(document.querySelectorAll('dialog[open], [role="dialog"], [role="listbox"]'))
    .filter((popup) => !popup.hasAttribute("popover") || popup.matches(":popover-open"));
}

export function hasDictationPopupOutside(input?: Element) {
  return openShortcutPopups().some((popup) => !input || !popup.contains(input));
}

function dispatchShortcut(event: KeyboardEvent) {
  if (event.defaultPrevented || event.repeat || event.isComposing || event.getModifierState("AltGraph")) return;
  if (event.target instanceof Element && event.target.closest("[data-dictation-shortcut-capture]")) return;
  const escape = event.key === "Escape" && !event.ctrlKey && !event.altKey && !event.shiftKey && !event.metaKey;
  if (!escape) {
    const pressed = chordFromEvent(event);
    if (!pressed) return;
    const chord = readChord(readSetting());
    if (pressed.code !== chord.code || pressed.ctrl !== chord.ctrl ||
      pressed.alt !== chord.alt || pressed.shift !== chord.shift || pressed.meta !== chord.meta) return;
  }
  const contexts = escape ? (["active"] as const) : priorities;
  for (const context of contexts) {
    for (const target of targets) {
      if (target(event, context)) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }
    }
  }
}

export function useDictationShortcut(target: Target) {
  const latest = useRef(target);
  useLayoutEffect(() => { latest.current = target; });
  useEffect(() => {
    const entry: Target = (event, context) => latest.current(event, context);
    if (!targets.size) window.addEventListener("keydown", dispatchShortcut, true);
    targets.add(entry);
    return () => {
      targets.delete(entry);
      if (!targets.size) window.removeEventListener("keydown", dispatchShortcut, true);
    };
  }, []);
}
