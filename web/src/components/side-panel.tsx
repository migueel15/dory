import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { useShortcuts } from "./shortcuts";
import { formatChord } from "./dictation-shortcut";
import { hasWindowClose } from "../lib/window-close";
import { ResizeHandle } from "../pages/thread";
import { Browser, ClipboardText, GitDiff, Plus, SidebarSimple, X } from "@phosphor-icons/react";

// A thread's side panel holds tabs of objects related to the thread: its
// changes and the plan of its task; new kinds join TAB_KINDS.
export type TabKind = "diff" | "plan" | "browser";
export const TAB_KINDS: Record<TabKind, { label: string; icon: ReactNode }> = {
  diff: { label: "Cambios", icon: <GitDiff className="size-4" /> },
  plan: { label: "Plan", icon: <ClipboardText className="size-4" /> },
  browser: { label: "Navegador", icon: <Browser className="size-4" /> },
};

type State = { open: boolean; tabs: TabKind[]; active: TabKind | null };
const empty: State = { open: false, tabs: [], active: null };
const read = (key: string): State => {
  try { const v = JSON.parse(localStorage.getItem(key) ?? "null"); return v && Array.isArray(v.tabs) ? { ...empty, ...v, tabs: v.tabs.filter((k: string) => k in TAB_KINDS) } : empty; } catch { return empty; }
};

const PANEL_EVENT = "dory-side-panel";

// showPanelTab opens the side panel of a thread on a tab from anywhere in the
// interface, whether or not the thread is on screen.
export function showPanelTab(threadId: string, kind: TabKind) {
  const key = `dory-thread-panel:${threadId}`;
  const s = read(key);
  const next: State = { open: true, tabs: s.tabs.includes(kind) ? s.tabs : [...s.tabs, kind], active: kind };
  try { localStorage.setItem(key, JSON.stringify(next)); } catch { /* per-viewer only */ }
  window.dispatchEvent(new CustomEvent(PANEL_EVENT, { detail: key }));
}

// useSidePanel keeps, per thread and in this browser, whether the panel is
// open, which tabs it has and which one is in front.
export function useSidePanel(threadId: string) {
  const key = `dory-thread-panel:${threadId}`;
  const [s, setS] = useState(() => read(key));
  useEffect(() => setS(read(key)), [key]);
  useEffect(() => {
    const on = (e: Event) => { if ((e as CustomEvent).detail === key) setS(read(key)); };
    window.addEventListener(PANEL_EVENT, on);
    return () => window.removeEventListener(PANEL_EVENT, on);
  }, [key]);
  const save = (next: State) => { setS(next); try { localStorage.setItem(key, JSON.stringify(next)); } catch { /* per-viewer only */ } };
  return {
    ...s,
    toggle: () => save({ ...s, open: !s.open }),
    // show opens the panel on a tab, adding it when the thread has none of that kind.
    show: (kind: TabKind) => save({ open: true, tabs: s.tabs.includes(kind) ? s.tabs : [...s.tabs, kind], active: kind }),
    focus: (kind: TabKind) => save({ ...s, active: kind }),
    close: (kind: TabKind) => {
      const tabs = s.tabs.filter((k) => k !== kind);
      // Closing the last tab closes the panel too.
      save({ ...s, open: tabs.length > 0 && s.open, tabs, active: s.active === kind ? tabs[tabs.length - 1] ?? null : s.active });
    },
  };
}
export type SidePanelState = ReturnType<typeof useSidePanel>;

export function SidePanelToggle({ panel }: { panel: SidePanelState }) {
  const getShortcut = useShortcuts();
  return (
    <button type="button" onClick={panel.toggle} title={`${panel.open ? "Cerrar el panel lateral" : "Abrir el panel lateral"} (${formatChord(getShortcut("thread-panel"))})`} aria-label="Panel lateral" aria-pressed={panel.open}
      className={`grid size-8 place-items-center rounded-md text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised ${panel.open ? "bg-slate-100 dark:bg-raised" : ""}`}>
      <SidebarSimple weight="regular" className="size-4 -scale-x-100" />
    </button>
  );
}

// SidePanel draws the tabs and the active one; with no tabs it offers what
// can be created.
export function SidePanel({ panel, render, available }: { panel: SidePanelState; render: (kind: TabKind) => ReactNode; available: TabKind[] }) {
  const aside = useRef<HTMLElement>(null);
  // The content mounts on first open and then stays, so closing slides it out.
  const [opened, setOpened] = useState(panel.open);
  useEffect(() => { if (panel.open) setOpened(true); }, [panel.open]);
  const [width, setWidth] = useState(() => { try { return Number(localStorage.getItem("dory.thread-panel-width")) || 560; } catch { return 560; } });
  const resize = (w: number) => { setWidth(w); try { localStorage.setItem("dory.thread-panel-width", String(w)); } catch { /* per-viewer only */ } };
  const missing = available.filter((k) => !panel.tabs.includes(k));
  return (
    // As on main: always mounted, its width slides between 0 and a fixed
    // pixel width while the content stays pinned right at full width, so it
    // enters from the right edge and the chat slides aside.
    <aside ref={aside} aria-label="Panel lateral" aria-hidden={!panel.open} inert={!panel.open}
      className={`hidden min-h-0 overflow-hidden lg:-mt-2 lg:flex lg:shrink-0 lg:transition-[width] lg:duration-300 lg:ease-out motion-reduce:transition-none ${panel.open ? "lg:w-[var(--panel-w)]" : "lg:w-0"}`}
      style={{ "--panel-w": `${width}px` } as CSSProperties}>
      <div className="ml-auto flex min-h-0 w-[var(--panel-w)] shrink-0 [contain:layout_paint]">
      <ResizeHandle panel={aside} width={width} onResize={resize} />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col border-l border-slate-200 dark:border-border">
      <div role="tablist" className={`flex shrink-0 items-center gap-1 h-10 border-b border-slate-200 px-2 pb-2 dark:border-border ${hasWindowClose ? "pr-[4.5rem]" : "pr-10"}`}>
        {panel.tabs.map((k) => (
          <div key={k} className={`group flex items-center rounded-md text-sm ${panel.active === k ? "bg-slate-100 font-medium dark:bg-raised" : "text-slate-500 hover:bg-slate-50 dark:text-muted dark:hover:bg-raised/60"}`}>
            <button type="button" role="tab" aria-selected={panel.active === k} onClick={() => panel.focus(k)} className="flex items-center gap-1.5 py-1 pl-2.5 pr-1">{TAB_KINDS[k].icon}{TAB_KINDS[k].label}</button>
            <button type="button" onClick={() => panel.close(k)} aria-label={`Cerrar ${TAB_KINDS[k].label}`} className="mr-1 grid size-5 place-items-center opacity-60 hover:opacity-100"><X weight="regular" className="size-3" /></button>
          </div>
        ))}
        {missing.map((k) => panel.tabs.length > 0 && (
          <button key={k} type="button" onClick={() => panel.show(k)} title={`Añadir ${TAB_KINDS[k].label}`} aria-label={`Añadir ${TAB_KINDS[k].label}`}
            className="grid size-7 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white"><Plus weight="regular" className="size-3.5" /></button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {!opened ? null : panel.active ? render(panel.active) : (
          <div className="grid h-full place-items-center p-6">
            <div className="space-y-2 text-center">
              <p className="text-sm text-slate-500 dark:text-muted">Este panel está vacío.</p>
              {missing.map((k) => (
                <button key={k} type="button" onClick={() => panel.show(k)}
                  className="inline-flex items-center gap-1.5 rounded-md border border-slate-200 px-3 py-1.5 text-sm hover:bg-slate-50 dark:border-border dark:hover:bg-raised">
                  <Plus weight="regular" className="size-3.5" />{TAB_KINDS[k].icon}{TAB_KINDS[k].label}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
      </div>
      </div>
    </aside>
  );
}
