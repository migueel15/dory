import { ThreadOpen, ThreadView } from "./pages/thread";
import { StrictMode, useEffect, useRef, useState } from "react";
import { Brain, ChartBar, ChatsCircle, FolderSimple, Gear, GithubLogo, HardDrives, House, IconContext, Kanban, List, Moon, Robot, SignOut, Sun, Tray, BellRinging, X } from "@phosphor-icons/react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, RouterProvider, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { Button, Input, Label, errMsg } from "./components/ui";
import { Environments, Memories, memorySearch } from "./pages/crud";
import { CommandPalette, CommandsButton } from "./components/commands";
import { UsageQuick } from "./components/usage-quick";
import { ProjectSwitcher } from "./components/project-switcher";
import { GithubView } from "./pages/github-detail";
import { Github } from "./pages/github";
import { Notifications, NotificationItem } from "./components/notifications";
import { Attention, useAttentionCount } from "./pages/attention";
import { Jobs } from "./pages/jobs";
import { Home } from "./pages/home";
import { Usage } from "./pages/usage";
import { Settings } from "./pages/settings";
import { Inbox } from "./pages/items";
import { Define } from "./pages/define";
import { ProjectDetail, Projects, Tasks } from "./pages/projects";
import { applyTheme, useTheme } from "./theme";
import "./index.css";

applyTheme();

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: (n, e) => !(e instanceof ApiError && e.status === 401) && n < 2, refetchOnWindowFocus: false } },
});

function Login() {
  const qc = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: (u) => qc.setQueryData(["me"], u),
  });
  return (
    <form
      className="mx-auto mt-24 w-80 max-w-[calc(100%-2rem)] space-y-3 rounded-lg border border-slate-200 dark:border-border bg-white dark:bg-surface p-5 shadow-sm"
      onSubmit={(e) => { e.preventDefault(); login.mutate(); }}
    >
      <div>
        <h1 className="flex items-center gap-2 text-xl font-bold">Dory<DevBadge /></h1>
        <p className="text-sm text-slate-500 dark:text-muted">Memoria y tareas para tus agentes</p>
      </div>
      <Label text="Usuario"><Input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus /></Label>
      <Label text="Contraseña"><Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></Label>
      {login.error && <p role="alert" className="rounded-md bg-red-50 px-2 py-1.5 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{login.error instanceof ApiError && login.error.status === 401 ? "Usuario o contraseña incorrectos." : errMsg(login.error)}</p>}
      <Button className="w-full" disabled={login.isPending}>Entrar</Button>
    </form>
  );
}

function SettingsLink() {
  return (
    <Link to="/settings" aria-label="Ajustes" title="Ajustes" className="grid min-h-11 min-w-11 place-items-center rounded-md hover:bg-slate-200 sm:min-h-0 sm:min-w-0 sm:px-3 sm:py-1.5 dark:hover:bg-raised [&.active]:bg-slate-200 dark:[&.active]:bg-raised">
      <Gear weight="duotone" className="size-5" aria-hidden="true" />
    </Link>
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
  // The project board needs the full width.
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/projects/") || l.pathname.startsWith("/items/") || l.pathname.startsWith("/threads/") || l.pathname === "/tasks" });
  const attention = useAttentionCount(!!me.data);

  const path = useLocation({ select: (l) => l.pathname });
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
  const ico = "mr-2.5 size-[18px] shrink-0 text-slate-900 dark:text-accent-text [.active_&]:text-white";
  const link = "flex min-h-11 shrink-0 items-center rounded px-3 text-sm font-medium text-slate-800 dark:text-text sm:min-h-0 sm:px-2 sm:py-1 hover:bg-slate-200 dark:hover:bg-raised [&.active]:bg-slate-900 dark:[&.active]:bg-accent [&.active]:text-white dark:[&.active]:text-white";
  // Group headings share the links' left padding and a rule above, so each section reads as a block.
  const group = (name: string) => <span className="mt-3 mb-1 border-t border-slate-200 px-3 pt-3 text-xs font-semibold uppercase tracking-wider text-slate-400 first:mt-1 first:border-t-0 first:pt-0 dark:border-border dark:text-muted sm:px-2">{name}</span>;
  const links = (
    <>
      {group("Trabajo")}
      <Link to="/" activeOptions={{ exact: true }} className={link}><House weight="duotone" className={ico} aria-hidden="true" />Inicio</Link>
      <Link to="/inbox" className={link}><Tray weight="duotone" className={ico} aria-hidden="true" />Inbox</Link>
      <Link to="/projects" className={link}><FolderSimple weight="duotone" className={ico} aria-hidden="true" />Proyectos</Link>
      {group("Agentes")}
      <Link to="/tasks" className={link}><Kanban weight="duotone" className={ico} aria-hidden="true" />Tareas</Link>
      <Link to="/attention" className={link}><BellRinging weight="duotone" className={ico} aria-hidden="true" />Atención{attention > 0 && <span className="ml-1.5 rounded-full bg-amber-500 px-1.5 text-xs font-semibold text-white">{attention}</span>}</Link>
      <Link to="/jobs" className={link}><Robot weight="duotone" className={ico} aria-hidden="true" />Trabajos</Link>
      <Link to="/threads" className={link}><ChatsCircle weight="duotone" className={ico} aria-hidden="true" />Hilos</Link>
      <Link to="/usage" className={link}><ChartBar weight="duotone" className={ico} aria-hidden="true" />Uso</Link>
      {group("Sistema")}
      <Link to="/github" className={link}><GithubLogo weight="duotone" className={ico} aria-hidden="true" />GitHub</Link>
      <Link to="/environments" className={link}><HardDrives weight="duotone" className={ico} aria-hidden="true" />Entornos</Link>
      <Link to="/memories" className={link}><Brain weight="duotone" className={ico} aria-hidden="true" />Memorias</Link>
    </>
  );
  return (
    <div className="min-h-screen bg-slate-50 dark:bg-canvas text-slate-900 dark:text-text sm:flex sm:h-screen sm:bg-white sm:dark:bg-surface">
      <aside className="sticky top-0 hidden h-screen w-56 shrink-0 flex-col bg-white dark:bg-surface sm:flex">
        <div className="flex items-center pr-2"><Link to="/" className="flex items-center gap-2 px-4 py-3 font-bold">Dory<DevBadge /></Link><CommandsButton /></div>
        <nav aria-label="Navegación principal" className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 pb-4">{links}</nav>
        <div className="shrink-0 px-2 pb-2"><ProjectSwitcher /></div>
        <div className="shrink-0 space-y-2 border-t border-slate-200 p-3 dark:border-border">
          <div className="flex items-center gap-1 [&>*]:flex [&>*]:flex-1 [&>*]:items-center [&>*]:justify-center">
            {desktop && <Notifications />}
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
      </aside>
      {/* On desktop the current view is a card framed by the sidebar background. */}
      <div className="min-w-0 flex-1 sm:my-2 sm:mr-2 sm:overflow-auto sm:rounded-lg sm:border sm:border-slate-200 sm:bg-slate-50 sm:dark:border-border sm:dark:bg-canvas">
      <nav className="sticky top-0 z-30 border-b border-slate-200 dark:border-border bg-white dark:bg-surface px-2 sm:hidden">
        <div className="flex h-12 items-center gap-1">
          <Link to="/" className="mr-1 flex shrink-0 items-center gap-2 font-bold">Dory<DevBadge /></Link>
          {/* The project is one tap away on phones, without opening the menu. */}
          <div className="min-w-0 flex-1"><ProjectSwitcher compact /></div>
          <div className="flex shrink-0 items-center gap-1">
            {!desktop && <Notifications />}
          </div>
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
          <button type="button" autoFocus aria-label="Cerrar menú" onClick={() => setMenu(false)} className="grid size-11 place-items-center rounded-lg hover:bg-slate-100 dark:hover:bg-raised">
            <X weight="regular" className="size-5" aria-hidden="true" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-4">
          <nav aria-label="Navegación móvil" className="flex flex-col gap-1" onClick={(event) => {
            if ((event.target as HTMLElement).closest("a")) setMenu(false);
          }}>{links}</nav>
        </div>
        <div className="shrink-0 space-y-3 border-t border-slate-200 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] dark:border-border">
          {/* Theme, settings and usage live here on phones so the top bar keeps room for the project. */}
          <div className="flex items-center gap-1 [&>*]:flex [&>*]:flex-1 [&>*]:items-center [&>*]:justify-center">
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
      <main className={`mx-auto p-2 sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"}`}><Outlet /><NotificationItem /></main></div><CommandPalette />
    </div>
  );
}

const root = createRootRoute({ component: Layout });
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: "/", component: Home }),
  createRoute({ getParentRoute: () => root, path: "/inbox", component: Inbox }),
  createRoute({ getParentRoute: () => root, path: "/attention", component: Attention }),
  createRoute({ getParentRoute: () => root, path: "/jobs", component: Jobs }),
  createRoute({ getParentRoute: () => root, path: "/usage", component: Usage }),
  createRoute({ getParentRoute: () => root, path: "/settings", component: Settings }),
  createRoute({ getParentRoute: () => root, path: "/github", component: Github }),
  createRoute({ getParentRoute: () => root, path: "/github/$owner/$repo/$kind/$number", component: GithubView }),
  createRoute({ getParentRoute: () => root, path: "/tasks", component: Tasks }),
  createRoute({ getParentRoute: () => root, path: "/threads", component: ThreadOpen }),
  createRoute({ getParentRoute: () => root, path: "/threads/$threadId", component: ThreadView }),
  createRoute({ getParentRoute: () => root, path: "/projects", component: Projects }),
  createRoute({ getParentRoute: () => root, path: "/projects/$id", component: ProjectDetail }),
  createRoute({ getParentRoute: () => root, path: "/items/$id/define", component: Define }),
  createRoute({ getParentRoute: () => root, path: "/environments", component: Environments }),
  createRoute({ getParentRoute: () => root, path: "/memories", component: Memories, validateSearch: memorySearch }),
]);
const router = createRouter({ routeTree });
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
