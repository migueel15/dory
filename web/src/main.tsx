import { Threads, ThreadsHome } from "./pages/threads";
import { WindowClose, hasWindowClose } from "./lib/window-close";
import { UsageQuick } from "./components/usage-quick";
import { StrictMode, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Brain, ChartBar, ChatsCircle, FolderSimple, Gear, GithubLogo, HardDrives, House, IconContext, Kanban, Lightning, List, Moon, SidebarSimple, SignOut, Sun, X } from "@phosphor-icons/react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, useNavigate, RouterProvider, createRootRoute, createRoute, createRouter, lazyRouteComponent, redirect } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { Button, Input, Label, errMsg } from "./components/ui";
import { Environments, Memories, memorySearch } from "./pages/crud";
import { listenShortcuts, useCommand } from "./components/shortcuts";
import { CommandPalette, CommandsButton } from "./components/commands";
import { TooltipHost } from "./components/tooltip";
import { MeshUpdateButton } from "./components/mesh-update";
import { SettingsModal } from "./components/settings-modal";
import { ProjectSwitcher } from "./components/project-switcher";
import { NoticeAlerts, NoticeInbox, NoticesToggle } from "./components/notices";
import { Home } from "./pages/home";
import { Github } from "./pages/github";
import { Tasks } from "./pages/tasks";
import { applyTheme, useTheme } from "./theme";
import { applyScale, listenZoomKeys } from "./scale";
import "./index.css";

applyTheme();
applyScale();
listenZoomKeys();

const queryClient = new QueryClient({
  // Data counts as fresh for 30 s and stays cached for 30 min: going back to a
  // page shows it at once, and the live stream invalidates whatever changes.
  defaultOptions: { queries: { retry: (n, e) => !(e instanceof ApiError && e.status === 401) && n < 2, refetchOnWindowFocus: false, staleTime: 30_000, gcTime: 30 * 60_000 } },
});

function Login() {
  const qc = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [validationError, setValidationError] = useState<"short" | "mismatch" | null>(null);
  // A new node has no account: the first one is created from this machine.
  const setup = useQuery({ queryKey: ["setup"], queryFn: api.setupNeeded, retry: false });
  const first = !!setup.data?.needed;
  const minimumLength = 8;
  const action = first ? "Crear cuenta" : "Iniciar sesión";
  const login = useMutation({
    mutationFn: () => (first ? api.setup(username, password) : api.login(username, password)),
    onSuccess: (u) => {
      qc.setQueryData(["setup"], { needed: false });
      qc.setQueryData(["me"], u);
    },
  });
  const error = validationError === "short" ? `Escribe al menos ${minimumLength} caracteres.`
    : validationError === "mismatch" ? "Las contraseñas no coinciden."
    : login.error instanceof ApiError && login.error.status === 401 ? "Usuario o contraseña incorrectos."
    : errMsg(login.error);
  const inputClass = "h-11 px-3";
  return (
    <main className="grid min-h-dvh place-items-center bg-[radial-gradient(ellipse_at_top,var(--color-indigo-100),transparent_60%)] px-4 py-8 dark:bg-[radial-gradient(ellipse_at_top,var(--color-accent-subtle),transparent_60%)]">
      <section aria-labelledby="login-title" className="w-full max-w-sm rounded-2xl border border-slate-200 bg-white p-6 shadow-lg shadow-slate-900/5 sm:p-8 dark:border-white/10 dark:bg-surface dark:shadow-black/25">
        <header className="space-y-4">
          <div className="flex items-center gap-2.5 font-semibold">
            <img src="/icon-192.png" alt="" className="size-9 rounded-lg" width="36" height="36" />
            Dory<DevBadge />
          </div>
          <div>
            <h1 id="login-title" className="text-2xl font-semibold tracking-tight">{setup.isSuccess ? action : "Acceso a Dory"}</h1>
            {setup.isSuccess && (
              <p className="mt-1 text-sm text-slate-600 dark:text-text-secondary">
                {first ? "Crea la primera cuenta de este nodo." : "Tu entorno de trabajo con agentes."}
              </p>
            )}
          </div>
        </header>
        {setup.isPending && <p role="status" className="mt-6 text-sm text-slate-600 dark:text-text-secondary">Preparando el acceso…</p>}
        {setup.isError && (
          <div className="mt-6 space-y-3">
            <p role="alert" className="text-sm text-red-700 dark:text-red-300">No se pudo comprobar el estado de la cuenta.</p>
            <Button variant="ghost" onClick={() => void setup.refetch()} disabled={setup.isFetching}>Reintentar</Button>
          </div>
        )}
        {setup.isSuccess && (
          <form className="mt-6 space-y-4" onSubmit={(e) => {
            e.preventDefault();
            if (login.isPending) return;
            login.reset();
            if (first && Array.from(password).length < minimumLength) return setValidationError("short");
            if (first && password !== confirmation) return setValidationError("mismatch");
            setValidationError(null);
            login.mutate();
          }}>
            <Label text="Usuario">
              <Input className={inputClass} name="username" autoComplete="username" required autoFocus value={username} onChange={(e) => setUsername(e.target.value)} />
            </Label>
            <div>
              <Label text="Contraseña">
                <Input className={inputClass} name="password" type="password" autoComplete={first ? "new-password" : "current-password"} required value={password}
                  aria-invalid={validationError === "short"} aria-describedby={first ? "password-hint" : undefined}
                  onChange={(e) => { setPassword(e.target.value); setValidationError(null); }} />
              </Label>
              {first && <p id="password-hint" className="mt-1.5 text-xs text-slate-600 dark:text-text-secondary">Mínimo de {minimumLength} caracteres.</p>}
            </div>
            {first && (
              <Label text="Confirmar contraseña">
                <Input className={inputClass} name="password-confirmation" type="password" autoComplete="new-password" required value={confirmation}
                  aria-invalid={validationError === "mismatch"}
                  onChange={(e) => { setConfirmation(e.target.value); setValidationError(null); }} />
              </Label>
            )}
            {error && <p role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{error}</p>}
            <Button type="submit" className="h-11 w-full" disabled={login.isPending}>
              {login.isPending ? (first ? "Creando cuenta…" : "Iniciando sesión…") : action}
            </Button>
          </form>
        )}
      </section>
    </main>
  );
}

function ThemeToggle() {
  const [dark, setTheme] = useTheme();
  const label = dark ? "Cambiar a modo claro" : "Cambiar a modo oscuro";
  return (
    <Button variant="bare" aria-label={label} title={label} onClick={() => setTheme(dark ? "light" : "dark")}>
      {dark ? <Sun weight="duotone" className="size-5" aria-hidden="true" /> : <Moon weight="duotone" className="size-5" aria-hidden="true" />}
    </Button>
  );
}

function SettingsLink() {
  const [open, setOpen] = useState(false);
  return <>
    <button type="button" aria-label="Ajustes" title="Ajustes" aria-haspopup="dialog" onClick={() => setOpen(true)}
      className="inline-flex min-h-11 items-center justify-center rounded-md px-3 py-1.5 hover:bg-slate-200 dark:hover:bg-raised sm:min-h-0">
      <Gear weight="duotone" className="size-5" aria-hidden="true" />
    </button>
    {open && <SettingsModal onClose={() => setOpen(false)} />}
  </>;
}

function VersionBadge() {
  const v = useQuery({ queryKey: ["version"], queryFn: api.version, staleTime: Infinity });
  if (!v.data) return null;
  const { revision, short, time, modified } = v.data;
  const cls = "font-mono text-xs text-slate-400 dark:text-muted hover:underline";
  if (revision === "dev") return <span className={cls} title="Sin datos de VCS">dev</span>;
  const title = `${time ? new Date(time).toLocaleString() : revision}${modified ? " +cambios" : ""}`;
  return <a className={cls} href={`https://github.com/migueel15/dory/commit/${revision}`} target="_blank" rel="noreferrer" title={title}>{short}{modified && "+"}</a>;
}

function Layout() {
  const qc = useQueryClient();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  const logout = useMutation({ mutationFn: api.logout, onSuccess: () => qc.clear() });
  useLiveUpdates(!!me.data);
  // The board, threads and pull requests need the full width.
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/threads") || /^\/github\/[^/]+\/[^/]+\/(pulls|issues)\//.test(l.pathname) || l.pathname === "/tasks" });
  // Threads that wait for the user show as a count on their link.
  const threads = useQuery({ queryKey: ["threads", "attention"], queryFn: () => api.threads(), enabled: !!me.data, refetchInterval: 30_000 });
  const attention = (threads.data ?? []).filter((t) => t.status === "waiting" || t.status === "error").length;

  const path = useLocation({ select: (l) => l.pathname });
  const navigate = useNavigate();
  const [menu, setMenu] = useState(false);
  // Notifications mount once (sidebar on desktop, top bar on phones) so toasts never repeat.
  const [desktop, setDesktop] = useState(() => window.matchMedia("(min-width: 640px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 640px)");
    const on = () => setDesktop(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  const drawer = useRef<HTMLDialogElement>(null);
  // The desktop sidebar folds to icons; the choice is remembered in this browser.
  const [folded, setFolded] = useState(() => { try { return localStorage.getItem("dory.sidebar-folded") === "1"; } catch { return false; } });
  const fold = (v: boolean) => {
    setFolded(v);
    try { localStorage.setItem("dory.sidebar-folded", v ? "1" : "0"); } catch { /* per-browser only */ }
  };

  useCommand("sidebar", () => fold(!folded));
  const navigateWithoutLosingDraft = async (search: boolean) => {
    if (search && path.startsWith("/threads")) {
      window.dispatchEvent(new Event("dory:focus-thread-search"));
      return;
    }
    if (document.querySelector('[data-unsaved-draft="true"]')) {
      window.alert("Hay un borrador sin enviar. Envíalo o vacíalo antes de cambiar de vista.");
      return;
    }
    await navigate({ to: "/threads" });
    if (search) window.setTimeout(() => window.dispatchEvent(new Event("dory:focus-thread-search")), 0);
  };
  useCommand("new-thread", () => { void navigateWithoutLosingDraft(false); });
  useCommand("search-threads", () => { void navigateWithoutLosingDraft(true); });
  useEffect(() => {
    if (!me.data) return;
    window.addEventListener("keydown", listenShortcuts);
    return () => window.removeEventListener("keydown", listenShortcuts);
  }, [!!me.data]);
  useEffect(() => { setMenu(false); }, [path]);
  useEffect(() => {
    if (!menu || !me.data) return;
    const dialog = drawer.current;
    if (!dialog) return;
    const previousOverflow = document.body.style.overflow;
    dialog.showModal();
    document.body.style.overflow = "hidden";
    const desktop = window.matchMedia("(min-width: 640px)");
    const closeOnDesktop = () => { if (desktop.matches) setMenu(false); };
    closeOnDesktop();
    desktop.addEventListener("change", closeOnDesktop);
    return () => {
      dialog.close();
      document.body.style.overflow = previousOverflow;
      desktop.removeEventListener("change", closeOnDesktop);
    };
  }, [menu, !!me.data]);
  if (me.isPending) return null;
  if (!me.data) return <Login />;
  const ico = "size-[1.125rem] shrink-0";
  // On desktop a link is 32 px tall with the icon 7 px in, so folded (32 px wide)
  // the selected link is a square and the icon never moves.
  const link = "flex min-h-11 shrink-0 items-center gap-2.5 overflow-hidden rounded px-3 text-sm font-medium text-slate-800 dark:text-text sm:h-8 sm:min-h-0 sm:px-[0.4375rem] sm:py-0 hover:bg-slate-200 dark:hover:bg-raised [&.active]:bg-slate-900 dark:[&.active]:bg-accent [&.active]:text-white dark:[&.active]:text-white";
  // Group headings share the links' left padding and a rule above, so each section reads as a block.
  // A section heading is a divider plus its name. Folded, the name fades and its
  // row collapses smoothly, so the links below move up and only the divider stays.
  const group = (name: string, small = false) => (
    <div className={`border-t border-slate-200 transition-[margin,padding] duration-200 first:mt-1 first:border-t-0 first:pt-0 dark:border-border ${small ? "mt-2 mb-1 pt-2" : "mt-3 mb-1 pt-3"}`}>
      <div className={`grid transition-[grid-template-rows,opacity] duration-200 ${small ? "grid-rows-[0fr] opacity-0" : "grid-rows-[1fr]"}`}>
        <span className="overflow-hidden whitespace-nowrap px-3 text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-muted sm:px-[0.4375rem]">{name}</span>
      </div>
    </div>
  );
  // small: the folded sidebar shows icons only, with each name as a tooltip.
  const navLinks = (small = false) => (
    <>
      {group("Trabajo", small)}
      <Link to="/" activeOptions={{ exact: true }} className={link} title={small ? "Inicio" : undefined}><House weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Inicio</span></Link>
      <Link to="/tasks" className={link} title={small ? "Tareas" : undefined}><Kanban weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Tareas</span></Link>
      <Link to="/threads" className={`${link} relative`} title={small ? "Hilos" : undefined}><ChatsCircle weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Hilos</span>{attention > 0 && (small ? <span className="absolute right-1.5 top-1 size-2 rounded-full bg-orange-500" /> : <span className="ml-1.5 rounded-full bg-orange-500 px-1.5 text-xs font-semibold text-white" title="Te necesitan">{attention}</span>)}</Link>
      <Link to="/projects" className={link} title={small ? "Proyectos" : undefined}><FolderSimple weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Proyectos</span></Link>
      <Link to="/github" className={link} title={small ? "GitHub" : undefined}><GithubLogo weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>GitHub</span></Link>
      {group("Sistema", small)}
      <Link to="/memories" className={link} title={small ? "Memorias" : undefined}><Brain weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Memorias</span></Link>
      <Link to="/skills" className={link} title={small ? "Skills" : undefined}><Lightning weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Skills</span></Link>
      <Link to="/environments" className={link} title={small ? "Entornos" : undefined}><HardDrives weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Entornos</span></Link>
      <Link to="/usage" className={link} title={small ? "Uso" : undefined}><ChartBar weight="duotone" className={ico} aria-hidden="true" /><span className={`whitespace-nowrap transition-opacity duration-150 ${small ? "opacity-0" : ""}`}>Uso</span></Link>
    </>
  );
  return (
    <div className="min-h-screen bg-slate-50 dark:bg-canvas text-slate-900 dark:text-text sm:flex sm:h-screen sm:bg-white sm:dark:bg-surface">
      <aside className={`sticky top-0 hidden h-screen shrink-0 flex-col overflow-hidden bg-white transition-[width] duration-200 ease-out dark:bg-surface sm:flex ${folded ? "w-14" : "w-56"}`}>
        {/* The toggle sits first, centered on the icon column, so it never moves. */}
        <div className="flex w-56 items-center pl-3 pr-2">
          <button type="button" onClick={() => fold(!folded)} aria-label={folded ? "Expandir barra lateral" : "Plegar barra lateral"} title={folded ? "Expandir barra lateral" : "Plegar barra lateral"}
            className="grid size-8 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
            <SidebarSimple weight="regular" className="size-[1.125rem]" aria-hidden="true" />
          </button>
          <div className={`flex min-w-0 flex-1 items-center transition-opacity duration-150 ${folded ? "pointer-events-none opacity-0" : ""}`}>
            <Link to="/" tabIndex={folded ? -1 : undefined} className="flex items-center gap-2 px-2 py-3 font-bold">Dory<DevBadge /></Link>
            <div className="ml-auto"><CommandsButton /></div>
          </div>
        </div>
        <nav aria-label="Navegación principal" className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto overflow-x-hidden px-3 pb-4">{navLinks(folded)}</nav>
        <div className="shrink-0 px-3 pb-2"><ProjectSwitcher folded={folded} /></div>
        <SidebarFooter folded={folded} username={me.data.username} onLogout={() => logout.mutate()} update={<MeshUpdateButton />}
          controls={[<NoticeInbox key="i" />, <NoticesToggle key="n" />, <ThemeToggle key="t" />, <SettingsLink key="settings" />, <UsageQuick key="u" />]} />
      </aside>
      {/* On desktop the current view is a card framed by the sidebar background. */}
      <div className="min-w-0 flex-1 sm:my-2 sm:mr-2 sm:overflow-auto sm:rounded-lg sm:border sm:border-slate-200 sm:bg-slate-50 sm:dark:border-border sm:dark:bg-canvas">
      <nav className="sticky top-0 z-30 border-b border-slate-200 dark:border-border bg-white dark:bg-surface px-2 sm:hidden">
        <div className="flex h-12 items-center gap-1">
          <Link to="/" className="mr-1 flex shrink-0 items-center gap-2 font-bold">Dory<DevBadge /></Link>
          {/* The project is one tap away on phones, without opening the menu. */}
          <div className="min-w-0 flex-1"><ProjectSwitcher compact /></div>
          <button type="button" aria-label="Menú" aria-controls="mobile-navigation" aria-expanded={menu} onClick={() => setMenu(!menu)} className="grid size-11 place-items-center rounded hover:bg-slate-200 dark:hover:bg-raised sm:hidden">
            {menu ? <X weight="regular" className="size-5" aria-hidden="true" /> : <List weight="regular" className="size-5" aria-hidden="true" />}
          </button>
        </div>
      </nav>
      <dialog
        ref={drawer}
        id="mobile-navigation"
        aria-labelledby="mobile-navigation-title"
        className="mobile-drawer fixed inset-y-0 left-auto right-0 m-0 h-dvh max-h-none w-80 max-w-[calc(100%-2rem)] flex-col border-l border-slate-200 bg-white p-0 text-slate-900 shadow-2xl dark:border-border dark:bg-surface dark:text-text open:flex sm:hidden"
        onCancel={() => setMenu(false)}
        onClose={() => setMenu(false)}
        onClick={(event) => {
          if (event.target !== event.currentTarget) return;
          const rect = event.currentTarget.getBoundingClientRect();
          if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) setMenu(false);
        }}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-slate-200 px-4 py-3 dark:border-border">
          <div>
            <h2 id="mobile-navigation-title" className="flex items-center gap-2 font-bold">Dory<DevBadge /></h2>
          </div>
          <button type="button" autoFocus aria-label="Cerrar menú" onClick={() => setMenu(false)} className="grid size-11 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
            <X weight="regular" className="size-5" aria-hidden="true" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-4">
          <nav aria-label="Navegación móvil" className="flex flex-col gap-1" onClick={(event) => {
            if ((event.target as HTMLElement).closest("a")) setMenu(false);
          }}>{navLinks()}</nav>
        </div>
        <div className="shrink-0 space-y-3 border-t border-slate-200 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] dark:border-border">
          {/* Theme, settings and usage live here on phones so the top bar keeps room for the project. */}
          <div className="flex items-center gap-1 [&>*]:flex [&>*]:flex-1 [&>*]:items-center [&>*]:justify-center">
            <NoticeInbox />
            <NoticesToggle />
            <ThemeToggle />
            <SettingsLink />
            <UsageQuick />
          </div>
          <div className="flex items-center justify-between gap-2 rounded-md border border-slate-200 py-1 pl-3 pr-1 dark:border-border">
            <span className="truncate text-sm text-slate-500 dark:text-muted">{me.data.username}</span>
            <Button variant="bare" aria-label="Cerrar sesión" title="Cerrar sesión" onClick={() => logout.mutate()}><SignOut weight="regular" className="size-5" aria-hidden="true" /></Button>
          </div>
          <VersionBadge />
        </div>
      </dialog>
      <main className={`mx-auto p-2 sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"} ${hasWindowClose && !path.startsWith("/threads") ? "sm:px-14" : ""}`}><Outlet /></main><WindowClose /></div><CommandPalette /><TooltipHost /><NoticeAlerts />
    </div>
  );
}

// Pages other than home, the thread list and attention load on demand, so the
// first paint downloads only what it shows; the rest is fetched when the
// browser is idle (and on hover, see defaultPreload) to keep navigation instant.
const pages = {
  thread: () => import("./pages/thread"),
  githubDetail: () => import("./pages/github-detail"),
  usage: () => import("./pages/usage"),
  projects: () => import("./pages/projects"),
  skills: () => import("./pages/skills"),
};
const ThreadView = lazyRouteComponent(pages.thread, "ThreadView");
const ExternalView = lazyRouteComponent(pages.thread, "ExternalView");
const GithubView = lazyRouteComponent(pages.githubDetail, "GithubView");
const Usage = lazyRouteComponent(pages.usage, "Usage");
const Projects = lazyRouteComponent(pages.projects, "Projects");
const ProjectDetail = lazyRouteComponent(pages.projects, "ProjectDetail");
const Skills = lazyRouteComponent(pages.skills, "Skills");
const idle = (fn: () => void) => ("requestIdleCallback" in window ? window.requestIdleCallback(fn, { timeout: 4000 }) : setTimeout(fn, 1500));
idle(() => { for (const load of Object.values(pages)) load().catch(() => {}); });

const root = createRootRoute({ component: Layout });
// The threads section keeps its list on the left; home and each thread fill the middle.
const threadsRoute = createRoute({ getParentRoute: () => root, path: "/threads", component: Threads });
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: "/", component: Home }),
  createRoute({ getParentRoute: () => root, path: "/inbox", beforeLoad: () => { throw redirect({ to: "/tasks" }); } }),
  createRoute({ getParentRoute: () => root, path: "/usage", component: Usage }),
  createRoute({ getParentRoute: () => root, path: "/skills", component: Skills }),
  createRoute({ getParentRoute: () => root, path: "/github", component: Github }),
  createRoute({ getParentRoute: () => root, path: "/github/$owner/$repo/$kind/$number", component: GithubView }),
  createRoute({ getParentRoute: () => root, path: "/tasks", component: Tasks }),
  threadsRoute.addChildren([
    createRoute({ getParentRoute: () => threadsRoute, path: "/", component: ThreadsHome }),
    createRoute({ getParentRoute: () => threadsRoute, path: "$threadId", component: ThreadView }),
    createRoute({ getParentRoute: () => threadsRoute, path: "external/$node/$provider/$id", component: ExternalView }),
  ]),
  createRoute({ getParentRoute: () => root, path: "/projects", component: Projects }),
  createRoute({ getParentRoute: () => root, path: "/projects/$id", component: ProjectDetail }),
  createRoute({ getParentRoute: () => root, path: "/environments", component: Environments }),
  createRoute({ getParentRoute: () => root, path: "/memories", component: Memories, validateSearch: memorySearch }),
]);
const router = createRouter({ routeTree, defaultPreload: "intent", defaultPreloadStaleTime: 0 });
declare module "@tanstack/react-router" {
  interface Register { router: typeof router }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/* Every icon uses the duotone style (tinted background) unless it sets its own weight. The context replaces Phosphor's defaults as a whole, so it repeats size and color: without them icons with no class lose their 1em size. */}
    <IconContext.Provider value={{ color: "currentColor", size: "1em", mirrored: false, weight: "duotone" }}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </IconContext.Provider>
  </StrictMode>,
);

if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(() => {});

function DevBadge() {
  if (!import.meta.env.DEV) return null;
  return <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-semibold text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">dev</span>;
}

// SidebarFooter follows the sidebar width as it folds, so nothing pops: the
// controls fold into a column one by one (theme under notifications, then
// settings, then usage), and the user card narrows until only the sign-out
// button is left, when its frame fades out.
function SidebarFooter({ folded, username, onLogout, controls, update }: { folded: boolean; username: string; onLogout: () => void; controls: React.ReactElement[]; update: React.ReactElement | null }) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(200);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  // The layout is a pure function of the measured width, recomputed on every
  // frame of the fold, with no CSS transition of its own: nothing can lag behind
  // the bar. t runs from 1 (open) to 0 (folded). The row of controls stays on the
  // bottom line: its leftmost control rises into a stack that grows upward
  // (notifications first, then theme, then settings), the rest of the row slides
  // left one place each time, and usage ends at the bottom without moving up.
  // Every rise is one step, so no control travels further than another.
  const cell = 32, gap = 4, n = controls.length, full = 200, row = cell + gap;
  const t = Math.max(0, Math.min(1, (width - cell) / (full - cell)));
  const rise = (i: number) => (i >= n - 1 ? 0 : Math.max(0, Math.min(1, (1 - i / (n - 1) - t) * (n - 1))));
  const base = ((full / n - cell) / 2) * t;
  // k: a control's slot in the bottom row (continuous); risen ones leave it.
  let sum = 0;
  const k = controls.map((_, i) => { sum += rise(i); return i - sum; });
  const last = k[n - 1];
  const step = last > 0.001 ? (width - 2 * base - cell) / last : 0;
  const pos = (i: number) => {
    let up = 0;
    for (let j = i; j < n - 1; j++) up += rise(j); // risen controls are pushed up by each one rising after them
    return { x: base + Math.max(0, k[i]) * step, y: -up * row };
  };
  const height = cell + controls.reduce((h, _, i) => h + rise(i), 0) * row;
  const narrow = width < 56; // only the sign-out button fits
  return (
    <div className="shrink-0 space-y-2 border-t border-slate-200 px-3 py-3 dark:border-border">
      <div ref={box} className="sidebar-track relative" style={{ height }}>
        {controls.map((c, i) => {
          const { x, y } = pos(i);
          return (
            <div key={c.key} className="sidebar-track absolute bottom-0 left-0 grid size-8 place-items-center [&>*]:!min-h-0 [&>*]:!min-w-0 [&>*]:!size-8 [&>*]:!p-0 [&>*]:grid [&>*]:place-items-center"
              style={{ transform: `translate(${x}px, ${y}px)` }}>{c}</div>
          );
        })}
      </div>
      <div className={`flex items-center justify-end gap-2 overflow-hidden rounded-md border py-1 transition-[border-color,padding] duration-200 ${narrow ? "border-transparent pl-0 pr-0" : "border-slate-200 pl-3 pr-1 dark:border-border"}`}>
        <span className={`min-w-0 flex-1 truncate whitespace-nowrap text-sm text-slate-500 transition-opacity duration-150 dark:text-muted ${width < 110 ? "opacity-0" : ""}`}>{username}</span>
        <Button variant="bare" aria-label="Cerrar sesión" title={`Cerrar sesión (${username})`} onClick={onLogout} className="grid size-8 shrink-0 place-items-center !p-0"><SignOut weight="regular" className="size-5" aria-hidden="true" /></Button>
      </div>
      {/* Below the account: the running commit and, to its right, the update button. */}
      <div className={`flex items-center transition-[gap] duration-200 ${folded ? "gap-0" : "gap-2"}`}>
        <div className={`grid min-w-0 flex-1 transition-[grid-template-rows,opacity] duration-200 ${folded ? "grid-rows-[0fr] opacity-0" : "grid-rows-[1fr]"}`}><div className="overflow-hidden whitespace-nowrap"><VersionBadge /></div></div>
        {update}
      </div>
    </div>
  );
}

