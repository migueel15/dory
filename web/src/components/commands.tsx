import { useEffect, useRef, useState, type ComponentType } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Task } from "../api/client";
import { useNavigate } from "@tanstack/react-router";
import { Brain, ChartBar, Command as CommandIcon, FolderSimple, GithubLogo, HardDrives, House, Kanban, Lightbulb, MagnifyingGlass, Robot, type IconProps } from "@phosphor-icons/react";
import { IdeaModal } from "./capture";
import { Input } from "./ui";
import { commands as shortcutCommands, useCommand, useShortcuts, available, runCommand } from "./shortcuts";
import { formatChord } from "./dictation-shortcut";
import { qk } from "../api/queries/keys";
import { useMesh, useNode, useProjects } from "../api/queries/hooks";

type Command = { id: string; group: string; label: string; icon: ComponentType<IconProps>; hint?: string; run: () => void };

const OPEN_EVENT = "dory:commands";
const kbd = "rounded border border-slate-200 bg-slate-50 px-1 font-sans text-[0.625rem] font-medium text-slate-500 dark:border-border dark:bg-raised dark:text-muted";

// CommandsButton opens the palette and shows its shortcut; it sits next to the app name.
export function CommandsButton() {
  const get = useShortcuts();
  return (
    <button type="button" onClick={() => window.dispatchEvent(new Event(OPEN_EVENT))} title="Comandos" aria-label="Abrir comandos"
      className="ml-auto flex items-center gap-1 rounded px-1.5 py-1 font-normal text-slate-500 hover:bg-slate-200 dark:text-muted dark:hover:bg-raised">
      <CommandIcon aria-hidden="true" className="size-4" />
      <kbd className={kbd}>{formatChord(get("palette"))}</kbd>
    </button>
  );
}

// CommandPalette opens with Ctrl+K (Cmd+K on macOS), or from CommandsButton, and runs app commands.
export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [idea, setIdea] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const onOpen = () => setOpen(true);
    window.addEventListener(OPEN_EVENT, onOpen);
    return () => window.removeEventListener(OPEN_EVENT, onOpen);
  }, []);
  useCommand("palette", () => setOpen((o) => !o));
  useCommand("new-idea", () => { setOpen(false); setIdea(true); });
  const get = useShortcuts();
  const go = (to: string) => () => {
    if (document.querySelector('[data-unsaved-draft="true"]')) {
      window.alert("Hay un borrador sin enviar. Envíalo o vacíalo antes de cambiar de vista.");
      return;
    }
    const [path, queryString] = to.split("?");
    const search = Object.fromEntries(new URLSearchParams(queryString ?? ""));
    void navigate({ to: path, search: search as never });
  };
  const nav = "Ir a";
  const commands: Command[] = [
    { id: "idea", group: "Crear", label: "Nueva idea", icon: Lightbulb, hint: formatChord(get("new-idea")), run: () => setIdea(true) },
    { id: "home", group: nav, label: "Inicio", icon: House, run: go("/") },
    { id: "tasks", group: nav, label: "Tareas", icon: Kanban, run: go("/tasks") },
    { id: "threads", group: nav, label: "Hilos", icon: Robot, run: go("/threads") },
    { id: "projects", group: nav, label: "Proyectos", icon: FolderSimple, run: go("/projects") },
    { id: "usage", group: nav, label: "Uso", icon: ChartBar, run: go("/usage") },
    { id: "github", group: nav, label: "GitHub", icon: GithubLogo, run: go("/github") },
    { id: "environments", group: nav, label: "Entornos", icon: HardDrives, run: go("/environments") },
    { id: "memories", group: nav, label: "Memorias", icon: Brain, run: go("/memories") },
  ];
  const contextual: Command[] = shortcutCommands.filter((c) => c.id !== "palette" && c.id !== "dictation" && c.id !== "new-idea" && available(c.id)).map((c) => ({ id: c.id, group: "Acciones", label: c.label, icon: CommandIcon, hint: formatChord(get(c.id)), run: () => { runCommand(c.id); } }));
  return (
    <>
      {open && <Palette commands={[commands[0], ...contextual, ...commands.slice(1)]} go={go} onClose={() => setOpen(false)} /> }
      {idea && <IdeaModal onClose={() => setIdea(false)} />}
    </>
  );
}

function Palette({ commands, go, onClose }: { commands: Command[]; go: (path: string) => () => void; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const previousFocus = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  useEffect(() => () => { if (!document.activeElement || document.activeElement === document.body) previousFocus.current?.focus(); }, []);
  const [selected, setSelected] = useState<string>();
  const [debounced, setDebounced] = useState("");
  useEffect(() => { const timer = window.setTimeout(() => setDebounced(query.trim()), 250); return () => clearTimeout(timer); }, [query]);
  const active = !!debounced && debounced === query.trim();
  const tasks = useQuery({ queryKey: qk.paletteTasks(debounced), queryFn: ({ signal }) => api.searchTasks(debounced, signal), enabled: active, retry: false });
  const github = useQuery({ queryKey: qk.paletteGithub(debounced), queryFn: ({ signal }) => api.searchGithub(debounced, signal), enabled: active, retry: false });
  const projects = useProjects({ enabled: !!query.trim(), retry: false });
  const node = useNode({ enabled: !!query.trim(), retry: false });
  const mesh = useMesh({ enabled: !!query.trim(), retry: false });
  const get = useShortcuts();
  const dictation: Command[] = previousFocus.current?.matches("textarea, input, [contenteditable=true]") ? [{
    id: "dictation", group: "Acciones", label: "Dictado", icon: CommandIcon, hint: formatChord(get("dictation")), run: () => {
      const target = previousFocus.current;
      const chord = get("dictation");
      window.setTimeout(() => {
        if (!target?.isConnected) return;
        target.focus();
        target.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, code: chord.code, key: chord.code.replace(/^(Key|Digit)/, "").toLowerCase(), ctrlKey: chord.ctrl, altKey: chord.alt, shiftKey: chord.shift, metaKey: chord.meta }));
      }, 0);
    },
  }] : [];
  const list = useRef<HTMLDivElement>(null);
  const q = query.trim().toLowerCase();
  const words = q.split(/\s+/).filter(Boolean);
  const matches = (text: string) => words.every((w) => text.toLowerCase().includes(w));
  const results: Command[] = q ? [
    ...(active ? tasks.data ?? [] : []).map((h) => ({ id: `task:${h.id}`, group: "Tareas", label: h.title, hint: (h.record as Task).project_id ? projects.data?.find((p) => p.id === (h.record as Task).project_id)?.name : "Sin proyecto", icon: Kanban, run: go(`/tasks?task=${encodeURIComponent(h.id)}`) })),
    ...(projects.data ?? []).filter((p) => matches(`${p.name} ${p.description}`)).map((p) => ({ id: `project:${p.id}`, group: "Proyectos", label: p.name, hint: p.description, icon: FolderSimple, run: go(`/projects/${encodeURIComponent(p.id)}`) })),
    ...[...(node.data ? [{ id: node.data.node.id, name: node.data.node.name, hint: "Local" }] : []), ...(mesh.data?.peers ?? []).filter((p) => !p.revoked).map((p) => ({ id: p.id, name: p.name, hint: p.connected ? "Conectado" : "Desconectado" }))].filter((n) => matches(n.name)).map((n) => ({ id: `node:${n.id}`, group: "Entornos", label: n.name, hint: n.hint, icon: HardDrives, run: go(`/environments?node=${encodeURIComponent(n.id)}`) })),
    ...(active ? github.data?.items ?? [] : []).map((h) => ({ id: `github:${h.repo}:${h.kind}:${h.number}`, group: "GitHub", label: h.title, hint: `${h.kind === "pulls" ? "PR" : "Issue"} ${h.repo} #${h.number} · ${{open: "abierta", closed: "cerrada", merged: "fusionada", draft: "borrador"}[h.state] ?? h.state}`, icon: GithubLogo, run: go(`/github/${h.repo}/${h.kind}/${h.number}`) })),
  ] : [];
  const shown = [commands[0], ...dictation, ...commands.slice(1)].filter((c) => !q || matches(`${c.group} ${c.label}`)).concat(results);
  const current = shown.find((c) => c.id === selected) ?? (!selected ? shown[0] : undefined);
  const hi = current ? shown.indexOf(current) : -1;
  useEffect(() => { setSelected(undefined); }, [q]);
  useEffect(() => { if (!selected && shown[0]) setSelected(shown[0].id); }, [selected, shown]);
  useEffect(() => { list.current?.querySelector<HTMLElement>("[aria-selected=true]")?.scrollIntoView({ block: "nearest" }); }, [selected, shown.length]);

  const run = (c?: Command) => { if (!c) return; onClose(); c.run(); };
  const key = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setSelected(shown[Math.min(shown.length - 1, hi + 1)]?.id); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setSelected(shown[Math.max(0, hi - 1)]?.id); }
    else if (e.key === "Enter") { e.preventDefault(); run(current); }
    else if (e.key === "Escape") onClose();
  };
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/40 p-2 pt-[15vh] sm:p-4 sm:pt-[15vh]" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Comandos" className="notif-in w-full max-w-lg overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl dark:border-border dark:bg-surface" onClick={(e) => e.stopPropagation()}>
        <div className="relative border-b border-slate-200 p-1.5 dark:border-border">
          <MagnifyingGlass weight="regular" aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
          <Input bare autoFocus value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={key} placeholder="Buscar comandos y elementos…" aria-label="Buscar comandos y elementos" className="py-2 pl-8 pr-2" />
        </div>
        <div ref={list} role="listbox" className="max-h-80 overflow-auto p-1">
          {shown.length === 0 && active && !tasks.isFetching && !github.isFetching && <p className="px-2 py-2 text-sm text-slate-400">Ninguna coincidencia.</p>}
          {q && (!active || tasks.isFetching || github.isFetching) && <p className="px-2 py-1 text-xs text-slate-400">Buscando…</p>}
          {q && (tasks.isError || projects.isError || node.isError || mesh.isError) && <p className="px-2 py-1 text-xs text-red-500">No se pudieron consultar todos los datos locales.</p>}
          {active && (github.isError || github.data?.incomplete) && <p className="px-2 py-1 text-xs text-red-500">GitHub no pudo completar la búsqueda.</p>}
          {shown.map((c, i) => (
            <div key={c.id}>
              {c.group !== shown[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-slate-400 dark:text-muted">{c.group}</p>}
              <button type="button" role="option" aria-selected={i === hi} onMouseEnter={() => setSelected(c.id)} onClick={() => run(c)}
                className={`flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm sm:min-h-0 ${i === hi ? "bg-slate-100 dark:bg-raised" : ""}`}>
                <c.icon weight="duotone" aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
                <span className="min-w-0 flex-1 truncate">{c.label}</span>
                {c.hint && <span className="text-xs text-slate-400">{c.hint}</span>}
              </button>
            </div>
          ))}
        </div>
        <div className="hidden items-center gap-3 border-t border-slate-200 px-3 py-1.5 text-[0.6875rem] text-slate-400 dark:border-border sm:flex">
          <span><kbd className={kbd}>↑↓</kbd> moverse</span><span><kbd className={kbd}>Enter</kbd> ejecutar</span><span><kbd className={kbd}>Esc</kbd> cerrar</span>
        </div>
      </div>
    </div>
  );
}
