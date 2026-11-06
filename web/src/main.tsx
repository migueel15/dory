import { StrictMode, useEffect, useRef, useState } from "react";
import { List, Moon, Sun, X } from "@phosphor-icons/react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, RouterProvider, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { Button, Input, Label, Select, errMsg } from "./components/ui";
import { setProject, useProject } from "./project";
import { Environments, Memories, memorySearch } from "./pages/crud";
import { CommandPalette } from "./components/commands";
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

function ThemeToggle() {
  const [dark, setTheme] = useTheme();
  const label = dark ? "Cambiar a modo claro" : "Cambiar a modo oscuro";
  return (
    <Button variant="bare" aria-label={label} title={label} onClick={() => setTheme(dark ? "light" : "dark")}>
      {dark ? <Sun className="size-5" aria-hidden="true" /> : <Moon className="size-5" aria-hidden="true" />}
    </Button>
  );
}

// ProjectFilter sets the global project every page filters by. Unset it reads as a
// plain "Filtrar por proyecto" label; set it becomes a removable badge.
function ProjectFilter() {
  const project = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  // Forget a selection that no longer exists (archived or from another database).
  const gone = !!project && !!projects.data && !projects.data.some((p) => p.id === project);
  useEffect(() => { if (gone) setProject(""); }, [gone]);
  const list = (projects.data ?? []).map((p) => ({ value: p.id, label: p.name }));
  if (!project || gone) return <Select plain className="w-auto" value="" onChange={setProject} placeholder="Filtrar por proyecto" options={list} />;
  return (
    <span className="inline-flex items-center rounded-full bg-blue-50 text-sm text-blue-800 dark:bg-accent-subtle dark:text-accent-text">
      <Select plain className="w-auto" value={project} onChange={setProject} options={[{ value: "", label: "Todos los proyectos" }, ...list]} />
      <button type="button" aria-label="Quitar filtro de proyecto" title="Quitar filtro" onClick={() => setProject("")} className="mr-1 grid size-6 place-items-center rounded-full hover:bg-blue-100 dark:hover:bg-accent-border/40"><X aria-hidden="true" className="size-3.5" /></button>
    </span>
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
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/projects/") || l.pathname.startsWith("/items/") || l.pathname === "/tasks" });
  const attention = useAttentionCount(!!me.data);

  const path = useLocation({ select: (l) => l.pathname });
  const [menu, setMenu] = useState(false);
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
  const link = "flex min-h-11 shrink-0 items-center rounded px-3 text-sm sm:min-h-0 sm:px-2 sm:py-1 hover:bg-slate-200 dark:hover:bg-raised [&.active]:bg-slate-900 dark:[&.active]:bg-accent [&.active]:text-white dark:[&.active]:text-white";
  const group = (name: string) => <span className="mt-4 mb-1 px-3 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:text-muted sm:hidden">{name}</span>;
  const sep = <span className="mx-1 hidden h-5 w-px self-center bg-slate-200 dark:bg-border sm:block" aria-hidden="true" />;
  const links = (
    <>
      {group("Trabajo")}
      <Link to="/" activeOptions={{ exact: true }} className={link}>Inicio</Link>
      <Link to="/inbox" className={link}>Inbox</Link>
      <Link to="/projects" className={link}>Proyectos</Link>
      {sep}
      {group("Agentes")}
      <Link to="/tasks" className={link}>Tareas</Link>
      <Link to="/attention" className={link}>Atención{attention > 0 && <span className="ml-1.5 rounded-full bg-amber-500 px-1.5 text-xs font-semibold text-white">{attention}</span>}</Link>
      <Link to="/jobs" className={link}>Trabajos</Link>
      <Link to="/usage" className={link}>Uso</Link>
      {sep}
      {group("Sistema")}
      <Link to="/github" className={link}>GitHub</Link>
      <Link to="/environments" className={link}>Entornos</Link>
      <Link to="/memories" className={link}>Memorias</Link>
      <Link to="/settings" className={link}>Ajustes</Link>
    </>
  );
  return (
    <div className="min-h-screen bg-slate-50 dark:bg-canvas text-slate-900 dark:text-text">
      <nav className="sticky top-0 z-30 border-b border-slate-200 dark:border-border bg-white dark:bg-surface px-2 sm:static sm:px-4">
        <div className="flex h-12 items-center gap-1 sm:h-auto sm:py-2">
          <span className="mr-3 flex items-center gap-2 font-bold">Dory<DevBadge /></span>
          <div className="hidden items-center gap-1 sm:flex">{links}</div>
          <div className="ml-auto flex items-center gap-1">
            <div className="hidden items-center gap-1 sm:flex">
              <ProjectFilter />
              <span className="truncate text-sm text-slate-500 dark:text-muted">{me.data.username}</span>
              <VersionBadge />
            </div>
            <Notifications />
            <ThemeToggle />
            <Button variant="ghost" className="hidden sm:inline-flex" onClick={() => logout.mutate()}>Salir</Button>
          </div>
          <button type="button" aria-label="Menú" aria-controls="mobile-navigation" aria-expanded={menu} onClick={() => setMenu(!menu)} className="grid size-11 place-items-center rounded hover:bg-slate-200 dark:hover:bg-raised sm:hidden">
            {menu ? <X className="size-5" aria-hidden="true" /> : <List className="size-5" aria-hidden="true" />}
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
            <p className="text-xs text-slate-500 dark:text-muted">Navegación</p>
          </div>
          <button type="button" autoFocus aria-label="Cerrar menú" onClick={() => setMenu(false)} className="grid size-11 place-items-center rounded-lg hover:bg-slate-100 dark:hover:bg-raised">
            <X className="size-5" aria-hidden="true" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-4">
          <nav aria-label="Navegación móvil" className="flex flex-col gap-1" onClick={(event) => {
            if ((event.target as HTMLElement).closest("a")) setMenu(false);
          }}>{links}</nav>
        </div>
        <div className="shrink-0 space-y-3 border-t border-slate-200 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] dark:border-border">
          <ProjectFilter />
          <div className="flex items-center justify-between gap-3">
            <span className="truncate text-sm text-slate-500 dark:text-muted">{me.data.username}</span>
            <Button variant="ghost" onClick={() => logout.mutate()}>Salir</Button>
          </div>
          <VersionBadge />
        </div>
      </dialog>
      <main className={`mx-auto p-2 sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"}`}><Outlet /><NotificationItem /></main><CommandPalette />
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
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);

if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(() => {});

function DevBadge() {
  if (!import.meta.env.DEV) return null;
  return <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-semibold text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">dev</span>;
}
