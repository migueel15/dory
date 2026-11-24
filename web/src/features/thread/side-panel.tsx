import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { useShortcuts } from "../../components/shortcuts";
import { formatChord } from "../composer/dictation-shortcut";
import { hasWindowClose } from "../../lib/window-close";
import { ContextMenu } from "../../components/context-menu";
import { ResizeHandle } from "./thread";
import { Browser, ClipboardText, DeviceMobile, GitDiff, Plus, SidebarSimple, X } from "@phosphor-icons/react";
import { events } from "../../lib/store";
import { storage, storageKeys } from "../../lib/storage";

// A thread's side panel holds tabs of objects related to the thread: its
// changes and the plan of its task; new kinds join TAB_KINDS.
export type TabKind = "diff" | "plan" | "device" | "browser";
export const TAB_KINDS: Record<TabKind, { label: string; icon: ReactNode }> = {
  diff: { label: "Cambios", icon: <GitDiff className="size-4" /> },
  plan: { label: "Plan", icon: <ClipboardText className="size-4" /> },
  device: { label: "Dispositivo", icon: <DeviceMobile className="size-4" /> },
  browser: { label: "Navegador", icon: <Browser className="size-4" /> },
};

// A tab is a kind ("diff", "plan") or, for the kinds that allow several,
// "kind:<id>"; per-tab data (a custom name, the chosen device) lives in meta.
export type TabMeta = { name?: string; serial?: string; floating?: boolean };
type State = { open: boolean; tabs: string[]; active: string | null; meta: Record<string, TabMeta> };
const empty: State = { open: false, tabs: [], active: null, meta: {} };
export const kindOf = (key: string) => key.split(":")[0] as TabKind;
const MULTIPLE: TabKind[] = ["device"];
const read = (key: string): State => {
  try { const v = JSON.parse(storage.get(key) ?? "null"); return v && Array.isArray(v.tabs) ? { ...empty, ...v, tabs: v.tabs.filter((k: string) => kindOf(k) in TAB_KINDS) } : empty; } catch { return empty; }
};

// showPanelTab opens the side panel of a thread on a tab from anywhere in the
// interface, whether or not the thread is on screen.
export function showPanelTab(threadId: string, kind: TabKind) {
  const key = storageKeys.threadPanel(threadId);
  const s = read(key);
  const next: State = { ...s, open: true, tabs: s.tabs.includes(kind) ? s.tabs : [...s.tabs, kind], active: kind };
  try { storage.set(key, JSON.stringify(next)); } catch { /* per-viewer only */ }
  events.panelChanged.emit(key);
}

// useSidePanel keeps, per thread and in this browser, whether the panel is
// open, which tabs it has and which one is in front.
export function useSidePanel(threadId: string) {
  const key = storageKeys.threadPanel(threadId);
  const [s, setS] = useState(() => read(key));
  useEffect(() => setS(read(key)), [key]);
  useEffect(() => {
    return events.panelChanged.on((k) => { if (k === key) setS(read(key)); });
  }, [key]);
  // Updates apply to the latest state, so several in one event do not overwrite each other.
  const update = (f: (s: State) => State) => setS((prev) => { const next = f(prev); try { storage.set(key, JSON.stringify(next)); } catch { /* per-viewer only */ } return next; });
  return {
    ...s,
    toggle: () => update((s) => ({ ...s, open: !s.open })),
    // show opens the panel on a tab, adding it when the thread has none of that kind;
    // kinds that allow several always get a new tab.
    show: (kind: TabKind, meta?: TabMeta) => {
      const tab = MULTIPLE.includes(kind) ? `${kind}:${Math.random().toString(36).slice(2, 8)}` : kind;
      update((s) => ({ ...s, open: true, tabs: s.tabs.includes(tab) ? s.tabs : [...s.tabs, tab], active: tab, meta: meta ? { ...s.meta, [tab]: meta } : s.meta }));
    },
    focus: (tab: string) => update((s) => ({ ...s, active: tab, open: true })),
    setMeta: (tab: string, meta: TabMeta) => update((s) => ({ ...s, meta: { ...s.meta, [tab]: { ...s.meta[tab], ...meta } } })),
    label: (tab: string) => s.meta[tab]?.name || TAB_KINDS[kindOf(tab)].label,
    close: (tab: string) => update((s) => {
      const tabs = s.tabs.filter((k) => k !== tab);
      const { [tab]: _, ...meta } = s.meta;
      // Closing the last tab closes the panel too.
      return { ...s, open: tabs.length > 0 && s.open, tabs, meta, active: s.active === tab ? tabs[tabs.length - 1] ?? null : s.active };
    }),
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
export function SidePanel({ panel, render, available }: { panel: SidePanelState; render: (tab: string) => ReactNode; available: TabKind[] }) {
  const aside = useRef<HTMLElement>(null);
  // The content mounts on first open and then stays, so closing slides it out.
  const [opened, setOpened] = useState(panel.open);
  useEffect(() => { if (panel.open) setOpened(true); }, [panel.open]);
  const [width, setWidth] = useState(() => { try { return Number(storage.get(storageKeys.panelWidth)) || 560; } catch { return 560; } });
  const resize = (w: number) => { setWidth(w); try { storage.set(storageKeys.panelWidth, String(w)); } catch { /* per-viewer only */ } };
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  // Every kind is listed; the ones this thread cannot open (no changes, no task) show dimmed.
  const kinds = Object.keys(TAB_KINDS) as TabKind[];
  const [renaming, setRenaming] = useState<string | null>(null);
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
            {renaming === k ? (
              <input autoFocus defaultValue={panel.label(k)} aria-label="Nombre de la pestaña" className="mx-1 w-28 rounded bg-white px-1.5 py-0.5 text-sm outline-none ring-1 ring-slate-300 dark:bg-surface dark:ring-border-strong"
                onBlur={(e) => { panel.setMeta(k, { name: e.target.value.trim() }); setRenaming(null); }}
                onKeyDown={(e) => { if (e.key === "Enter") e.currentTarget.blur(); if (e.key === "Escape") setRenaming(null); }} />
            ) : (
              <button type="button" role="tab" aria-selected={panel.active === k} onClick={() => panel.focus(k)} onDoubleClick={() => MULTIPLE.includes(kindOf(k)) && setRenaming(k)} className="flex items-center gap-1.5 py-1 pl-2.5 pr-1">{TAB_KINDS[kindOf(k)].icon}{panel.label(k)}</button>
            )}
            <button type="button" onClick={() => panel.close(k)} aria-label={`Cerrar ${panel.label(k)}`} className="mr-1 grid size-5 place-items-center opacity-60 hover:opacity-100"><X weight="regular" className="size-3" /></button>
          </div>
        ))}
        <button type="button" aria-label="Abrir una pestaña" title="Abrir una pestaña" aria-haspopup="menu" aria-expanded={!!menu}
          onPointerDown={(e) => { if (menu) e.stopPropagation(); }}
          onClick={(e) => { const r = e.currentTarget.getBoundingClientRect(); setMenu(menu ? null : { x: r.left, y: r.bottom + 4 }); }}
          className="grid size-7 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><Plus weight="regular" className="size-4" /></button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {!opened ? null : panel.active ? render(panel.active) : (
          <div className="grid h-full place-items-center p-6">
            <div className="w-56">
              <p className="mb-2 text-center text-sm font-medium">Abrir una pestaña</p>
              <div className="space-y-0.5">{kinds.map((k) => <KindRow key={k} kind={k} enabled={available.includes(k)} onPick={() => panel.show(k)} />)}</div>
            </div>
          </div>
        )}
      </div>
      </div>
      </div>
      {menu && <ContextMenu x={menu.x} y={menu.y} label="Abrir una pestaña" onClose={() => setMenu(null)}
        actions={kinds.map((k) => ({ label: TAB_KINDS[k].label, content: <KindRow kind={k} enabled={available.includes(k)} onPick={() => panel.show(k)} menu /> }))} />}
    </aside>
  );
}

// KindRow is one thing the panel can open: its icon and name, dimmed when this thread cannot open it.
function KindRow({ kind, enabled, onPick, menu = false }: { kind: TabKind; enabled: boolean; onPick: () => void; menu?: boolean }) {
  return (
    <button type="button" role={menu ? "menuitem" : undefined} disabled={!enabled} onClick={onPick}
      className="flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm hover:bg-slate-100 focus:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent dark:hover:bg-raised dark:focus:bg-raised dark:disabled:hover:bg-transparent">
      {TAB_KINDS[kind].icon}{TAB_KINDS[kind].label}
    </button>
  );
}
