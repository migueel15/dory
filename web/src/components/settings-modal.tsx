import { useEffect, useRef, useState, type ComponentType } from "react";
import { createPortal } from "react-dom";
import { useRouterState } from "@tanstack/react-router";
import { MagnifyingGlass, Bell, Brain, Cpu, ChatsCircle, FolderSimple, GitBranch, GitFork, GithubLogo, HardDrives, Keyboard, Lightbulb, Lightning, Microphone, PaintBrush, SquaresFour, X, type IconProps } from "@phosphor-icons/react";
import { ProvidersSettings } from "./settings-providers";
import { DictationSettings } from "./settings-dictation";
import { ShortcutsSettings } from "./settings-shortcuts";
import { AppearanceSettings } from "./settings-appearance";
import { IdeasSettings } from "./settings-ideas";
import { GithubSettings } from "./settings-github";
import { NoticesSettings } from "./settings-notices";
import { TaskSettings } from "./settings-tasks";
import { ConnectionsSettings } from "./settings-connections";
import { ThreadsSettings } from "./settings-threads";
import { SourceControlSettings } from "./settings-source-control";
import { WorktreesSettings } from "./settings-worktrees";
import { ALL_NODES, type SectionProps } from "./settings-node";
import { useNodes } from "./launch-picker";
import { Select } from "./ui";
import { commands } from "./shortcuts";
import { Projects } from "../pages/projects";
import { Memories } from "../pages/crud";
import { Skills } from "../pages/skills";

// A section receives the selected node ("all" or a node id). perNode: false
// hides the selector (device-local or global sections).
type Section = { id: string; label: string; icon: ComponentType<IconProps>; component: ComponentType<SectionProps>; perNode?: false };

// GROUPS are the titled blocks of the side bar, in order. To add a section, add
// a file with its component and one line here.
const GROUPS: { title: string; sections: Section[] }[] = [
  { title: "General", sections: [
    { id: "appearance", label: "Apariencia", icon: PaintBrush, component: AppearanceSettings, perNode: false },
    { id: "shortcuts", label: "Atajos", icon: Keyboard, component: ShortcutsSettings },
    { id: "dictation", label: "Dictado", icon: Microphone, component: DictationSettings },
    { id: "notices", label: "Avisos", icon: Bell, component: NoticesSettings },
    { id: "connections", label: "Conexiones", icon: HardDrives, component: ConnectionsSettings },
  ] },
  { title: "Agentes", sections: [
    { id: "providers", label: "Proveedores", icon: Cpu, component: ProvidersSettings },
    { id: "threads", label: "Hilos", icon: ChatsCircle, component: ThreadsSettings },
    { id: "skills", label: "Skills", icon: Lightning, component: Skills },
    { id: "memories", label: "Memorias", icon: Brain, component: Memories, perNode: false },
  ] },
  { title: "Código", sections: [
    { id: "projects", label: "Proyectos", icon: FolderSimple, component: Projects, perNode: false },
    { id: "source-control", label: "Control de versiones", icon: GitBranch, component: SourceControlSettings },
    { id: "worktrees", label: "Worktrees", icon: GitFork, component: WorktreesSettings },
    { id: "github", label: "GitHub", icon: GithubLogo, component: GithubSettings },
  ] },
  { title: "Tareas", sections: [
    { id: "tasks", label: "Tareas", icon: SquaresFour, component: TaskSettings },
    { id: "ideas", label: "Ideas", icon: Lightbulb, component: IdeasSettings },
  ] },
];
const SECTIONS = GROUPS.flatMap((g) => g.sections);

// OPTIONS are the searchable titles inside each section, as they read on
// screen, so the search box can open the section and point at the option.
// Keep them in sync when a section adds or renames an option.
const OPTIONS: Record<string, string[]> = {
  appearance: ["Tema", "Claro", "Oscuro", "Sistema", "Escala de la interfaz", "Zoom"],
  shortcuts: commands.map((c) => c.label),
  dictation: ["Dictado local", "Modelos de dictado", "Vista previa en directo", "Transcribe al terminar", "Descargar modelo"],
  notices: ["Avisos", "Notificaciones push", "Siempre push", "Solo en la app", "Automático", "Desactivado"],
  connections: ["Entornos", "Nodos", "Emparejar", "Nodos encontrados en la red", "mDNS", "Activar o desactivar un nodo", "Eliminar nodo"],
  providers: ["Cuentas", "Añadir cuenta", "Nombre visible", "Carpeta de configuración", "Iniciar sesión", "Claude Code", "Codex", "OpenCode"],
  threads: ["Modelo por defecto", "Modelo de respaldo", "Workspace por defecto", "Rama actual (checkout)", "Nuevo worktree", "Archivar hilos al mergear su rama"],
  skills: ["Skills del sistema", "Conflictos de skills"],
  memories: ["Memorias", "Decisiones", "Lecciones"],
  projects: ["Proyectos", "Repositorios"],
  "source-control": ["Actualizar la rama del hilo automáticamente", "Auto-pull", "Sincronizar rama"],
  worktrees: ["Ubicación por defecto", "Convención de nombres de rama", "Eliminar el worktree al borrar o archivar el hilo", "Eliminar el worktree al mergear su rama"],
  github: ["Conectar GitHub", "Cerrar sesión"],
  tasks: ["Bloquear el túnel del navegador", "Cerrar los servicios de los hilos al reiniciar Dory", "Dispositivos", "Los agentes pueden usar dispositivos"],
  ideas: ["Títulos de las ideas", "Sin títulos automáticos"],
};

const fold = (t: string) => t.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();

type Hit = { section: Section; group: string; option?: string };

// search matches the query against group titles, section labels and options.
function search(q: string): Hit[] {
  const words = fold(q).split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  const has = (t: string) => { const f = fold(t); return words.every((w) => f.includes(w)); };
  const hits: Hit[] = [];
  for (const g of GROUPS) for (const s of g.sections) {
    if (has(`${g.title} ${s.label}`)) hits.push({ section: s, group: g.title });
    for (const o of OPTIONS[s.id] ?? []) if (has(o)) hits.push({ section: s, group: g.title, option: o });
  }
  return hits;
}

// highlight scrolls to the first element of the section body whose own text
// contains the option and flashes it.
function highlight(root: HTMLElement | null, option: string) {
  if (!root) return;
  const want = fold(option);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!fold(n.textContent ?? "").includes(want)) continue;
    const el = (n.parentElement?.closest("label, li, section > div, div.flex") ?? n.parentElement) as HTMLElement | null;
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    el.classList.add("settings-hit");
    setTimeout(() => el.classList.remove("settings-hit"), 1600);
    return;
  }
}

// SettingsModal covers most of the screen over a dimmed background. A click
// outside it or Escape closes it. It renders on the body, so no ancestor of
// the button that opens it can clip or position it.
export function SettingsModal({ onClose }: { onClose: () => void }) {
  const [active, setActive] = useState(SECTIONS[0].id);
  const [node, setNode] = useState(ALL_NODES);
  const nodes = useNodes();
  const [query, setQuery] = useState("");
  const hits = search(query);
  const body = useRef<HTMLDivElement>(null);
  const [target, setTarget] = useState<string | null>(null);
  // After opening a hit, wait for the section to render (some load data) and point at the option.
  useEffect(() => {
    if (!target) return;
    let tries = 0;
    const id = setInterval(() => {
      const found = body.current && fold(body.current.textContent ?? "").includes(fold(target));
      if (found || ++tries > 20) { clearInterval(id); if (found) highlight(body.current, target); setTarget(null); }
    }, 100);
    return () => clearInterval(id);
  }, [target, active]);
  const open = (h: Hit) => { setActive(h.section.id); setQuery(""); if (h.option) setTarget(h.option); };
  // Following a link inside a section (a project, say) leaves the modal.
  const path = useRouterState({ select: (r) => r.location.pathname });
  const first = useRef(path);
  useEffect(() => { if (path !== first.current) onClose(); }, [path, onClose]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  const section = SECTIONS.find((s) => s.id === active) ?? SECTIONS[0];
  const Body = section.component;
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-2 sm:p-6" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Ajustes" onClick={(e) => e.stopPropagation()}
        className="flex h-full max-h-[56rem] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-2xl dark:border-border dark:bg-surface md:flex-row">
        <nav aria-label="Secciones de ajustes" className="flex shrink-0 gap-1 overflow-x-auto border-b border-slate-200 p-2 dark:border-border md:w-60 md:flex-col md:overflow-y-auto md:border-b-0 md:border-r md:p-3">
          <label className="relative mb-1 flex shrink-0 items-center md:mx-0">
            <MagnifyingGlass className="pointer-events-none absolute left-2.5 size-4 text-slate-400 dark:text-muted" aria-hidden="true" />
            <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Buscar ajustes" aria-label="Buscar ajustes"
              onKeyDown={(e) => { if (e.key === "Enter" && hits[0]) open(hits[0]); if (e.key === "Escape" && query) { e.stopPropagation(); setQuery(""); } }}
              className="w-44 rounded-md border border-slate-200 bg-transparent py-1.5 pl-8 pr-2 text-sm outline-none focus:border-accent dark:border-border md:w-full" />
          </label>
          {query.trim() ? (
            <div className="flex gap-1 md:flex-col" role="list" aria-label="Resultados">
              {hits.length === 0 && <span className="px-3 py-2 text-sm text-slate-500 dark:text-muted">Sin resultados</span>}
              {hits.map((h, i) => (
                <button key={`${h.section.id}-${h.option ?? ""}-${i}`} type="button" role="listitem" onClick={() => open(h)}
                  className="flex min-w-0 flex-col items-start rounded-md px-3 py-2 text-left text-sm hover:bg-slate-50 dark:hover:bg-raised">
                  <span className="truncate font-medium">{h.option ?? h.section.label}</span>
                  <span className="truncate text-xs text-slate-500 dark:text-muted">{h.group} · {h.section.label}</span>
                </button>
              ))}
            </div>
          ) : GROUPS.map((g) => (
            <div key={g.title} className="flex gap-1 md:flex-col">
              <span className="hidden px-3 pb-1 pt-3 text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-muted md:block">{g.title}</span>
              {g.sections.map(({ id, label, icon: Icon }) => (
                <button key={id} type="button" onClick={() => setActive(id)} aria-current={id === active ? "page" : undefined}
                  className={`flex items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm ${id === active ? "bg-slate-100 font-semibold dark:bg-raised" : "text-slate-600 hover:bg-slate-50 dark:text-muted dark:hover:bg-raised"}`}>
                  <Icon className="size-4" aria-hidden="true" />{label}
                </button>
              ))}
            </div>
          ))}
        </nav>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <header className="flex items-center gap-2 border-b border-slate-200 px-5 py-3 text-sm dark:border-border">
            <span className="text-slate-500 dark:text-muted">Ajustes</span>
            <span className="text-slate-400">/</span>
            <span className="font-semibold">{section.label}</span>
            {section.perNode !== false && <Select className="ml-auto w-52" value={node} onChange={setNode} options={[{ value: ALL_NODES, label: "Todos los nodos" }, ...nodes]} />}
            <button type="button" aria-label="Cerrar" onClick={onClose} className={`${section.perNode === false ? "ml-auto " : ""}grid size-8 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white`}>
              <X weight="regular" className="size-4" aria-hidden="true" />
            </button>
          </header>
          <div className="min-h-0 flex-1 overflow-auto p-5 sm:p-8">
            <div ref={body} className="mx-auto max-w-4xl"><Body node={node} /></div>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
