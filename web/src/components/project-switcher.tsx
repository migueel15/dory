import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { CaretUpDown, Check, FolderSimple, MagnifyingGlass, SquaresFour } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { setProject, useProject } from "../project";
import { Input } from "./ui";
import { useCommand } from "./shortcuts";

// ProjectSwitcher picks the global project. It looks like a sidebar link and opens a
// searchable menu; "compact" fits the phone top bar.
// folded: the sidebar is folded to icons. As it narrows the name truncates away,
// then the caret is clipped, and the icon stays in the icon column.
export function ProjectSwitcher({ compact = false, folded = false }: { compact?: boolean; folded?: boolean }) {
  const project = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const current = projects.data?.find((p) => p.id === project);
  // Forget a selection that no longer exists (archived or from another database).
  const gone = !!project && !!projects.data && !current;
  useEffect(() => { if (gone) setProject(""); }, [gone]);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const [pos, setPos] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  useCommand("project", () => { if (!trigger.current?.getClientRects().length) return false; setHighlight(0); setOpen(true); });
  const name = current?.name ?? "Todos los proyectos";
  // Folded, the name is hidden: an instant tooltip to the right says which project is on.
  const [tip, setTip] = useState<CSSProperties | null>(null);
  const showTip = () => { const t = trigger.current?.getBoundingClientRect(); if (folded && t && !open) setTip({ left: t.right + 8, top: t.top + t.height / 2 }); };

  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const t = trigger.current.getBoundingClientRect();
    // Open below the trigger when there is room, otherwise above it.
    const width = Math.min(Math.max(t.width, 260), window.innerWidth - 16);
    const left = Math.max(8, Math.min(t.left, window.innerWidth - width - 8));
    setPos(t.bottom + 320 < window.innerHeight ? { left, top: t.bottom + 4, width } : { left, bottom: window.innerHeight - t.top + 4, width });
  }, [open]);
  useEffect(() => {
    if (!open) { setQuery(""); return; }
    const away = (e: MouseEvent) => !trigger.current?.contains(e.target as Node) && !panel.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") { setOpen(false); trigger.current?.focus(); } };
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [open]);

  const q = query.trim().toLowerCase();
  const options = [{ id: "", name: "Todos los proyectos" }, ...(projects.data ?? [])].filter((p) => !q || p.name.toLowerCase().includes(q));
  const pick = (id: string) => { setProject(id); setOpen(false); trigger.current?.focus(); };

  const button = (
    <button ref={trigger} type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => { setTip(null); setOpen(!open); }} title={folded ? undefined : "Cambiar de proyecto"}
      onMouseEnter={showTip} onMouseLeave={() => setTip(null)} onFocus={showTip} onBlur={() => setTip(null)}
      className={`flex w-full items-center overflow-hidden rounded text-left ${compact ? "min-h-11 px-2" : "h-8 px-[0.4375rem]"} text-sm font-medium text-slate-800 hover:bg-slate-200 dark:text-text dark:hover:bg-raised ${open ? "bg-slate-200 dark:bg-raised" : ""}`}>
      {current ? <FolderSimple weight="duotone" aria-hidden="true" className="mr-2.5 size-[1.125rem] shrink-0 opacity-80" /> : <SquaresFour weight="duotone" aria-hidden="true" className="mr-2.5 size-[1.125rem] shrink-0 opacity-80" />}
      <span className="min-w-0 flex-1 truncate">{name}</span>
      <CaretUpDown weight="regular" aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
    </button>
  );

  return (
    <>
      {button}
      {tip && folded && !open && createPortal(
        <div role="tooltip" style={tip} className="notif-in pointer-events-none fixed z-[100] -translate-y-1/2 whitespace-nowrap rounded-md bg-slate-900 px-2.5 py-1.5 text-xs text-white shadow-lg dark:bg-raised dark:text-text">
          <div className="text-[0.625rem] uppercase tracking-wide opacity-60">Proyecto</div>
          <div className="font-medium">{name}</div>
        </div>,
        document.body,
      )}
      {open && createPortal(
        <div ref={panel} style={pos} className="notif-in fixed z-[100] overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl dark:border-border dark:bg-surface">
          <div className="relative border-b border-slate-200 p-1.5 dark:border-border">
            <MagnifyingGlass weight="regular" aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
            <Input bare autoFocus value={query} onChange={(e) => { setQuery(e.target.value); setHighlight(0); }} onKeyDown={(e) => { if (e.key === "ArrowDown") { e.preventDefault(); setHighlight((i) => Math.min(i + 1, options.length - 1)); } else if (e.key === "ArrowUp") { e.preventDefault(); setHighlight((i) => Math.max(i - 1, 0)); } else if (e.key === "Enter" && options[highlight]) { e.preventDefault(); pick(options[highlight].id); } }} placeholder="Buscar proyecto…" aria-label="Buscar proyecto" className="py-1.5 pl-8 pr-2" />
          </div>
          <div role="listbox" className="max-h-72 overflow-auto p-1">
            {options.map((p, i) => (
              <button key={p.id || "all"} type="button" role="option" aria-selected={p.id === project} onClick={() => pick(p.id)}
                className={`flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-100 sm:min-h-0 dark:hover:bg-raised ${i === highlight ? "bg-slate-100 dark:bg-raised" : ""}`}>
                {p.id ? <FolderSimple weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" /> : <SquaresFour weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" />}
                <span className={`min-w-0 flex-1 truncate ${p.id === project ? "font-medium text-blue-700 dark:text-accent-text" : ""}`}>{p.name}</span>
                {p.id === project && <Check weight="bold" aria-hidden="true" className="size-4 shrink-0 text-blue-700 dark:text-accent-text" />}
              </button>
            ))}
            {options.length === 0 && <p className="px-2 py-2 text-sm text-slate-400">Ningún proyecto coincide.</p>}
          </div>
        </div>,
        document.body,
      )}
    </>
  );
}
