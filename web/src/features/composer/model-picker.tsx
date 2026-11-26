import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, CaretDown, Check, MagnifyingGlass } from "@phosphor-icons/react";
import { claudeModelName } from "../../components/model-label";
import { ProviderIcon } from "./launch-picker";
import { useCommand, useShortcuts } from "../../components/shortcuts";
import { formatChord } from "./dictation-shortcut";

export type MenuOption = { value: string; label: string; group?: string; modelId?: string; icon?: React.ReactNode };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
export function Chip({ value, options, onChange, title, icon, below = false, align = "left", prefix, shortcutBranch = false }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode; below?: boolean; align?: "left" | "right"; prefix?: string; shortcutBranch?: boolean }) {
  const [open, setOpen] = useState(false);
  const getShortcut = useShortcuts();
  const box = useOutside(open, () => setOpen(false));
  const current = options.find((o) => o.value === value);
  const trigger = useRef<HTMLButtonElement>(null);
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const filtered = options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()));
  const close = () => { setOpen(false); setQuery(""); trigger.current?.focus(); };
  useCommand("branch", () => { if (!trigger.current?.getClientRects().length) return false; setQuery(""); setHighlight(0); setOpen(true); }, shortcutBranch);
  return (
    <div ref={box} className="relative">
      <button ref={trigger} type="button" title={shortcutBranch ? `${title} (${formatChord(getShortcut("branch"))})` : title} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[11rem] items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon ?? icon}<span className="truncate">{prefix && `${prefix} `}{current?.label ?? value}</span><CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className={`absolute ${align === "right" ? "right-0" : "left-0"} z-20 max-h-72 min-w-[12rem] max-w-[min(24rem,calc(100vw-2rem))] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface ${below ? "top-full mt-2" : "bottom-full mb-2"}`}>
          {shortcutBranch && <li><input autoFocus aria-label="Buscar rama" value={query} onChange={(e) => { setQuery(e.target.value); setHighlight(0); }} onKeyDown={(e) => {
            if (e.key === "Escape") { e.stopPropagation(); close(); }
            else if (e.key === "ArrowDown") { e.preventDefault(); setHighlight((i) => Math.min(i + 1, filtered.length - 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setHighlight((i) => Math.max(i - 1, 0)); }
            else if (e.key === "Enter" && filtered[highlight]) { e.preventDefault(); onChange(filtered[highlight].value); close(); }
          }} className="w-full rounded bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-2 focus:ring-blue-500" /></li>}
          {filtered.map((o, i) => (
            <li key={o.value}>
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); close(); }}
                className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised ${shortcutBranch && i === highlight ? "bg-slate-100 dark:bg-raised" : ""}`}>
                {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === value && <Check weight="regular" className="size-3.5" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function useOutside(open: boolean, close: () => void) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) close(); };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  return box;
}

export type ModelOption = MenuOption & { provider: string };
export type ModelProvider = { id: string; name: string; disabled?: boolean };

// ModelPicker filters the active provider's catalog and shows the selected model's reasoning levels.
export function ModelPicker({ models, providers, selected, effort, effortsOf, onChange, modelId = selected, reportedEffort, onSelectClose }: { models: ModelOption[]; providers: ModelProvider[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void; modelId?: string; reportedEffort?: string; onSelectClose?: () => void }) {
  const [open, setOpen] = useState(false);
  const getShortcut = useShortcuts();
  const [focus, setFocus] = useState(selected);
  const [provider, setProvider] = useState("");
  const [search, setSearch] = useState("");
  const [navigationIndex, setNavigationIndex] = useState<number>();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [placement, setPlacement] = useState<{ above: boolean; maxHeight: number }>();
  const gap = 8;
  const modelList = useRef<HTMLUListElement>(null);
  const effortList = useRef<HTMLDivElement>(null);
  const listId = useId();
  const box = useOutside(open, () => setOpen(false));
  useLayoutEffect(() => {
    const button = trigger.current;
    const menu = panel.current;
    if (!open || !button || !menu) return;

    const viewport = window.visualViewport;
    const place = () => {
      const anchor = button.getBoundingClientRect();
      const viewportTop = viewport?.offsetTop ?? 0;
      const viewportBottom = viewportTop + (viewport?.height ?? window.innerHeight);
      const aboveSpace = anchor.top - viewportTop - 2 * gap;
      const belowSpace = viewportBottom - anchor.bottom - 2 * gap;
      const menuHeight = menu.scrollHeight + (menu.offsetHeight - menu.clientHeight);
      const above = menuHeight <= aboveSpace || (menuHeight > belowSpace && aboveSpace > belowSpace);
      const maxHeight = Math.max(0, Math.floor(above ? aboveSpace : belowSpace));
      setPlacement((previous) => previous?.above === above && previous.maxHeight === maxHeight ? previous : { above, maxHeight });
    };
    place();
    const resize = new ResizeObserver(place);
    resize.observe(menu);
    resize.observe(button);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    viewport?.addEventListener("resize", place);
    viewport?.addEventListener("scroll", place);
    return () => {
      resize.disconnect();
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      viewport?.removeEventListener("resize", place);
      viewport?.removeEventListener("scroll", place);
    };
  }, [open]);
  const current = models.find((m) => m.value === selected);
  const activeProvider = providers.find((item) => item.id === provider && !item.disabled) ?? providers.find((item) => !item.disabled);
  const enabledProviders = new Set(providers.filter((item) => !item.disabled).map((item) => item.id));
  const providerNames = new Map(providers.map((item) => [item.id, item.name]));
  const query = search.trim().toLowerCase();
  const visibleModels = models.filter((model) =>
    query
      ? enabledProviders.has(model.provider) &&
        [model.label, model.modelId ?? model.value, providerNames.get(model.provider) ?? model.provider].some((name) => name.toLowerCase().includes(query))
      : model.provider === activeProvider?.id,
  );
  const highlightedIndex = navigationIndex ?? (query ? 0 : undefined);
  const highlightedModel = highlightedIndex === undefined ? undefined : visibleModels[highlightedIndex];
  const close = (confirmed = false) => {
    setOpen(false);
    if (confirmed && onSelectClose) {
      if (!touch) onSelectClose();
    } else {
      trigger.current?.focus();
    }
  };
  const toggle = () => {
    if (open) {
      close();
      return;
    }
    setProvider(current?.provider ?? activeProvider?.id ?? "");
    setFocus(selected);
    setSearch("");
    setNavigationIndex(undefined);
    setOpen(true);
  };
  useCommand("model", () => { if (!trigger.current?.getClientRects().length) return false; toggle(); });
  const claude = claudeModelName(modelId);
  const modelLabel = claude ||
    (modelId === (current?.modelId ?? selected) ? current?.label : undefined) ||
    (modelId && modelId !== "default" ? modelId : "Seleccionar modelo");
  const effortLabel = effort || reportedEffort;
  // On touch screens the search waits for a tap, so the keyboard does not cover the list.
  const touch = window.matchMedia("(pointer: coarse)").matches;
  const settingsLabel = effortLabel ? `${modelLabel} · ${effortLabel}` : modelLabel;
  const efforts = visibleModels.some((model) => model.value === focus) ? effortsOf(focus) : undefined;
  const pickModel = (value: string, mode: "stay" | "close" | "focusEfforts" = "stay") => {
    const picked = models.find((m) => m.value === value);
    if (picked) setProvider(picked.provider);
    const e = effortsOf(value);
    setFocus(value);
    onChange(value, value === selected && effort ? effort : e?.initial ?? "");
    if (!e || mode === "close") {
      close(true);
      return;
    }
    if (mode === "focusEfforts") {
      requestAnimationFrame(() => {
        const activeBtn = effortList.current?.querySelector<HTMLButtonElement>('[aria-pressed="true"]');
        (activeBtn ?? effortList.current?.querySelector<HTMLButtonElement>("button"))?.focus();
      });
    }
  };
  const row = "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div ref={box} className="relative flex">
      <button ref={trigger} type="button" title={`${settingsLabel}${claude ? ` (${modelId})` : ""} · ${formatChord(getShortcut("model"))}`} aria-label={`Modelo y razonamiento: ${settingsLabel}`} aria-haspopup="dialog" aria-expanded={open} onClick={toggle}
        className="inline-flex h-8 max-w-[20rem] items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon}<span className="truncate text-slate-900 dark:text-white">{modelLabel}</span>
        {effortLabel && <span className="shrink-0 text-slate-500 dark:text-muted">· {effortLabel}</span>}<CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div ref={panel} role="dialog" aria-label="Modelo y razonamiento"
          onKeyDown={(e) => { if (e.key === "Escape" && !e.nativeEvent.isComposing) { e.stopPropagation(); close(); } }}
          style={{
            [placement?.above === false ? "top" : "bottom"]: `calc(100% + ${gap}px)`,
            maxHeight: placement?.maxHeight,
          }}
          className="absolute left-0 z-20 w-[min(36rem,calc(100vw-4.5rem))] overflow-y-auto rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <div className="flex items-center gap-2 border-b border-slate-200 px-3 dark:border-border">
            <MagnifyingGlass weight="regular" className="size-4 shrink-0 text-slate-500 dark:text-muted" />
            <input autoFocus={!touch} type="search" role="combobox" aria-expanded={open} aria-controls={listId} aria-autocomplete="list"
              aria-activedescendant={highlightedModel ? `${listId}-${highlightedIndex}` : undefined}
              value={search} placeholder="Buscar modelos…" aria-label="Buscar modelos"
              onChange={(e) => {
                setSearch(e.currentTarget.value);
                setNavigationIndex(undefined);
                if (modelList.current) modelList.current.scrollTop = 0;
              }}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return;
                if (e.key === "Enter") {
                  e.preventDefault();
                  if (highlightedModel) pickModel(highlightedModel.value, e.shiftKey ? "focusEfforts" : "close");
                  return;
                }
                if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
                e.preventDefault();
                if (!visibleModels.length) return;
                const lastIndex = visibleModels.length - 1;
                const down = e.key === "ArrowDown";
                const next = highlightedModel && highlightedIndex !== undefined
                  ? Math.max(0, Math.min(lastIndex, highlightedIndex + (down ? 1 : -1)))
                  : down ? 0 : lastIndex;
                setNavigationIndex(next);
                modelList.current?.children.item(next)?.scrollIntoView({ block: "nearest" });
              }}
              className="min-w-0 flex-1 appearance-none border-0 bg-transparent py-2.5 text-sm outline-none! focus-visible:outline-none!" />
          </div>
          <div className="grid grid-cols-[3rem_minmax(0,1fr)] sm:grid-cols-[3rem_minmax(0,1fr)_7rem]">
            <ul aria-label="Proveedores" className="row-span-2 max-h-64 overflow-y-auto border-r border-slate-200 p-1 dark:border-border sm:row-span-1">
              {providers.map((item) => (
                <li key={item.id}>
                  <button type="button" disabled={item.disabled} aria-pressed={!query && activeProvider?.id === item.id} aria-label={item.name}
                    title={item.disabled ? "Espera a que termine el turno para cambiar de proveedor" : item.name}
                    onClick={() => {
                      setProvider(item.id);
                      setSearch("");
                      setNavigationIndex(undefined);
                      if (modelList.current) modelList.current.scrollTop = 0;
                    }}
                    className="grid size-10 place-items-center rounded-lg text-base hover:bg-slate-100 disabled:opacity-50 aria-pressed:bg-slate-200 dark:hover:bg-raised dark:aria-pressed:bg-raised">
                    <ProviderIcon driver={item.id} />
                  </button>
                </li>
              ))}
            </ul>
            <ul ref={modelList} id={listId} role="listbox" aria-label="Modelos" className="max-h-44 min-w-0 overflow-y-auto p-1 sm:max-h-64">
              {visibleModels.map((model, index) => (
                <li key={model.value} role="presentation">
                  <button id={`${listId}-${index}`} type="button" role="option" onClick={() => pickModel(model.value)} title={model.label}
                    aria-selected={model.value === selected}
                    className={`${row} ${model === highlightedModel ? "ring-1 ring-inset ring-slate-500 dark:ring-accent-text" : ""}`}>
                    {model.icon}<span className="min-w-0 flex-1 truncate">{model.label}</span>
                    {model.value === selected && <Check weight="thin" className="size-3.5 shrink-0" />}
                  </button>
                </li>
              ))}
              {!visibleModels.length && <li role="presentation" className="px-2 py-1.5 text-sm text-slate-500 dark:text-muted">{search.trim() ? "Sin resultados" : "Sin modelos disponibles"}</li>}
            </ul>
            {efforts && (
              <div ref={effortList} aria-label="Razonamiento"
                onKeyDown={(e) => {
                  if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
                  e.preventDefault();
                  const buttons = Array.from(e.currentTarget.querySelectorAll<HTMLButtonElement>("button"));
                  const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
                  if (index === -1 || !buttons.length) return;
                  const next = Math.max(0, Math.min(buttons.length - 1, index + (e.key === "ArrowDown" ? 1 : -1)));
                  buttons[next]?.focus();
                }}
                className="flex flex-wrap gap-1 border-t border-slate-200 p-1 dark:border-border sm:block sm:max-h-64 sm:overflow-y-auto sm:border-l sm:border-t-0">
                {efforts.values.map((level) => {
                  const active = level === (focus === selected ? effort : efforts.initial);
                  return (
                    <button key={level} type="button" aria-pressed={active} onClick={() => { onChange(focus, level); close(true); }}
                      className={`${row} !w-auto outline-none! focus-visible:outline-none! focus:bg-slate-100 focus:ring-1 focus:ring-inset focus:ring-slate-500 dark:focus:bg-raised dark:focus:ring-accent-text sm:!w-full`}>
                      <span className="min-w-0 flex-1 truncate">{level}</span>{active && <Check weight="thin" className="size-3.5 shrink-0" />}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

