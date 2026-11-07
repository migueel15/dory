import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { CaretUpDown, Check, FolderSimple, MagnifyingGlass, SquaresFour } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { setProject, useProject } from "../project";
import { Input } from "./ui";

// ProjectSwitcher picks the global project. "row" looks like a sidebar link,
// "card" like a workspace block; both open the same searchable menu.
export function ProjectSwitcher({ variant }: { variant: "row" | "card" }) {
  const project = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const current = projects.data?.find((p) => p.id === project);
  // Forget a selection that no longer exists (archived or from another database).
  const gone = !!project && !!projects.data && !current;
  useEffect(() => { if (gone) setProject(""); }, [gone]);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [pos, setPos] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const name = current?.name ?? "Todos los proyectos";

  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const t = trigger.current.getBoundingClientRect();
    // Open below the trigger when there is room, otherwise above it.
    setPos(t.bottom + 320 < window.innerHeight ? { left: t.left, top: t.bottom + 4, width: Math.max(t.width, 240) } : { left: t.left, bottom: window.innerHeight - t.top + 4, width: Math.max(t.width, 240) });
  }, [open]);
  useEffect(() => {
    if (!open) { setQuery(""); return; }
    const away = (e: MouseEvent) => !trigger.current?.contains(e.target as Node) && !panel.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [open]);

  const q = query.trim().toLowerCase();
  const options = [{ id: "", name: "Todos los proyectos" }, ...(projects.data ?? [])].filter((p) => !q || p.name.toLowerCase().includes(q));
  const pick = (id: string) => { setProject(id); setOpen(false); };

  const button = variant === "row" ? (
    <button ref={trigger} type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)} title="Cambiar de proyecto"
      className={`flex w-full items-center rounded px-2 py-1 text-left text-sm font-medium text-slate-800 hover:bg-slate-200 dark:text-text dark:hover:bg-raised ${open ? "bg-slate-200 dark:bg-raised" : ""}`}>
      {current ? <FolderSimple weight="duotone" aria-hidden="true" className="mr-2.5 size-[18px] shrink-0 opacity-80" /> : <SquaresFour weight="duotone" aria-hidden="true" className="mr-2.5 size-[18px] shrink-0 opacity-80" />}
      <span className="min-w-0 flex-1 truncate">{name}</span>
      <CaretUpDown aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
    </button>
  ) : (
    <button ref={trigger} type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)} title="Cambiar de proyecto"
      className={`flex w-full items-center gap-2.5 rounded-lg border border-slate-200 p-2 text-left hover:bg-slate-100 dark:border-border dark:hover:bg-raised ${open ? "bg-slate-100 dark:bg-raised" : ""}`}>
      <span className="grid size-8 shrink-0 place-items-center rounded-md bg-slate-900 text-sm font-bold text-white dark:bg-accent">{current ? current.name[0]?.toUpperCase() : <SquaresFour weight="bold" aria-hidden="true" className="size-4" />}</span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold">{name}</span>
        <span className="block text-[11px] text-slate-500 dark:text-muted">{current ? "Proyecto" : "Sin filtro"}</span>
      </span>
      <CaretUpDown aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
    </button>
  );

  return (
    <>
      {button}
      {open && createPortal(
        <div ref={panel} style={pos} className="notif-in fixed z-[100] overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl dark:border-border dark:bg-surface">
          <div className="relative border-b border-slate-200 p-1.5 dark:border-border">
            <MagnifyingGlass aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
            <Input bare autoFocus value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => e.key === "Enter" && options[0] && pick(options[0].id)} placeholder="Buscar proyecto…" aria-label="Buscar proyecto" className="py-1.5 pl-8 pr-2" />
          </div>
          <div role="listbox" className="max-h-72 overflow-auto p-1">
            {options.map((p) => (
              <button key={p.id || "all"} type="button" role="option" aria-selected={p.id === project} onClick={() => pick(p.id)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
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
