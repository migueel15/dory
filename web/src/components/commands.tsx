import { useEffect, useRef, useState, type ComponentType } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Brain, ChartBar, Command as CommandIcon, FolderSimple, GithubLogo, HardDrives, House, Kanban, Lightbulb, MagnifyingGlass, Robot, type IconProps } from "@phosphor-icons/react";
import { IdeaModal } from "./capture";
import { Input } from "./ui";

type Command = { id: string; group: string; label: string; icon: ComponentType<IconProps>; hint?: string; run: () => void };

const OPEN_EVENT = "dory:commands";
const mac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);
const kbd = "rounded border border-slate-200 bg-slate-50 px-1 font-sans text-[10px] font-medium text-slate-500 dark:border-border dark:bg-raised dark:text-muted";

// CommandsButton opens the palette and shows its shortcut; it sits next to the app name.
export function CommandsButton() {
  return (
    <button type="button" onClick={() => window.dispatchEvent(new Event(OPEN_EVENT))} title="Comandos" aria-label="Abrir comandos"
      className="ml-auto flex items-center gap-1 rounded px-1.5 py-1 font-normal text-slate-500 hover:bg-slate-200 dark:text-muted dark:hover:bg-raised">
      <CommandIcon aria-hidden="true" className="size-4" />
      <kbd className={kbd}>{mac ? "⌘K" : "Ctrl K"}</kbd>
    </button>
  );
}

// CommandPalette opens with Ctrl+K (Cmd+K on macOS), or from CommandsButton, and runs app commands.
export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [idea, setIdea] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") { e.preventDefault(); setOpen((o) => !o); }
    };
    const onOpen = () => setOpen(true);
    window.addEventListener("keydown", onKey);
    window.addEventListener(OPEN_EVENT, onOpen);
    return () => { window.removeEventListener("keydown", onKey); window.removeEventListener(OPEN_EVENT, onOpen); };
  }, []);
  const go = (to: string) => () => navigate({ to });
  const nav = "Ir a";
  const commands: Command[] = [
    { id: "idea", group: "Crear", label: "Nueva idea", icon: Lightbulb, hint: "Al inbox", run: () => setIdea(true) },
    { id: "home", group: nav, label: "Inicio", icon: House, run: go("/") },
    { id: "tasks", group: nav, label: "Tareas", icon: Kanban, run: go("/tasks") },
    { id: "threads", group: nav, label: "Hilos", icon: Robot, run: go("/threads") },
    { id: "projects", group: nav, label: "Proyectos", icon: FolderSimple, run: go("/projects") },
    { id: "usage", group: nav, label: "Uso", icon: ChartBar, run: go("/usage") },
    { id: "github", group: nav, label: "GitHub", icon: GithubLogo, run: go("/github") },
    { id: "environments", group: nav, label: "Entornos", icon: HardDrives, run: go("/environments") },
    { id: "memories", group: nav, label: "Memorias", icon: Brain, run: go("/memories") },
  ];
  return (
    <>
      {open && <Palette commands={commands} onClose={() => setOpen(false)} />}
      {idea && <IdeaModal onClose={() => setIdea(false)} />}
    </>
  );
}

function Palette({ commands, onClose }: { commands: Command[]; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const [hi, setHi] = useState(0);
  const list = useRef<HTMLDivElement>(null);
  const q = query.trim().toLowerCase();
  const shown = commands.filter((c) => !q || `${c.group} ${c.label}`.toLowerCase().includes(q));
  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => { list.current?.querySelector<HTMLElement>("[aria-selected=true]")?.scrollIntoView({ block: "nearest" }); }, [hi]);
  const run = (c?: Command) => { if (!c) return; onClose(); c.run(); };
  const key = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setHi((i) => Math.min(shown.length - 1, i + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setHi((i) => Math.max(0, i - 1)); }
    else if (e.key === "Enter") { e.preventDefault(); run(shown[hi]); }
    else if (e.key === "Escape") onClose();
  };
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/40 p-2 pt-[15vh] sm:p-4 sm:pt-[15vh]" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Comandos" className="notif-in w-full max-w-lg overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl dark:border-border dark:bg-surface" onClick={(e) => e.stopPropagation()}>
        <div className="relative border-b border-slate-200 p-1.5 dark:border-border">
          <MagnifyingGlass weight="regular" aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
          <Input bare autoFocus value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={key} placeholder="Buscar comando…" aria-label="Buscar comando" className="py-2 pl-8 pr-2" />
        </div>
        <div ref={list} role="listbox" className="max-h-80 overflow-auto p-1">
          {shown.length === 0 && <p className="px-2 py-2 text-sm text-slate-400">Ningún comando coincide.</p>}
          {shown.map((c, i) => (
            <div key={c.id}>
              {c.group !== shown[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:text-muted">{c.group}</p>}
              <button type="button" role="option" aria-selected={i === hi} onMouseEnter={() => setHi(i)} onClick={() => run(c)}
                className={`flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm sm:min-h-0 ${i === hi ? "bg-slate-100 dark:bg-raised" : ""}`}>
                <c.icon weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
                <span className="min-w-0 flex-1 truncate">{c.label}</span>
                {c.hint && <span className="text-xs text-slate-400">{c.hint}</span>}
              </button>
            </div>
          ))}
        </div>
        <div className="hidden items-center gap-3 border-t border-slate-200 px-3 py-1.5 text-[11px] text-slate-400 dark:border-border sm:flex">
          <span><kbd className={kbd}>↑↓</kbd> moverse</span><span><kbd className={kbd}>Enter</kbd> ejecutar</span><span><kbd className={kbd}>Esc</kbd> cerrar</span>
        </div>
      </div>
    </div>
  );
}
