import { useEffect, useRef } from "react";

// A minimal typed event emitter and store, without dependencies. They replace
// the old event bus on window: every cross-component notification is a
// typed emitter declared here.

export type Emitter<T> = { emit: (payload: T) => void; on: (fn: (payload: T) => void) => () => void };

export function createEmitter<T = void>(): Emitter<T> {
  const listeners = new Set<(payload: T) => void>();
  return {
    emit: (payload) => { for (const fn of [...listeners]) fn(payload); },
    on: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; },
  };
}

// useEmitter listens to an emitter while the component is mounted, always
// calling the latest handler.
export function useEmitter<T>(emitter: Emitter<T>, handler: (payload: T) => void) {
  const latest = useRef(handler);
  latest.current = handler;
  useEffect(() => emitter.on((payload) => latest.current(payload)), [emitter]);
}

export type DeviceEvent = { thread: string; action: "open" | "close"; serial: string; floating?: boolean };

export type TerminalEvent = { thread: string; action: "open" | "close"; terminal: string };

// events are the typed actions of the interface: who emits them and who
// listens is explicit in the imports.
export const events = {
  /** An agent opened or closed a device of a thread. */
  deviceChanged: createEmitter<DeviceEvent>(),
  /** An agent opened or closed a terminal of a thread. */
  terminalChanged: createEmitter<TerminalEvent>(),
  /** An agent asked the desktop app to open the browser of a thread (thread id). */
  browserOpened: createEmitter<string>(),
  /** Ask a thread's message box to take a text. */
  fillComposer: createEmitter<{ threadId: string; text: string }>(),
  /** The stored state of a thread's side panel changed (storage key). */
  panelChanged: createEmitter<string>(),
  /** The turn filter of a thread changed (thread id). */
  turnFilterChanged: createEmitter<string>(),
  commandsOpen: createEmitter(),
  focusThreadSearch: createEmitter(),
  fontsChanged: createEmitter(),
  scaleChanged: createEmitter(),
  themeChanged: createEmitter(),
  shortcutsChanged: createEmitter(),
  dictationShortcutChanged: createEmitter(),
  dictationModelChanged: createEmitter(),
  dictationActivityChanged: createEmitter(),
};
