import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, RouterProvider, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { Button, ErrorText, Input, Label, Select } from "./components/ui";
import { setProject, useProject } from "./project";
import { Environments, Memories, memorySearch } from "./pages/crud";
import { CommandPalette } from "./components/commands";
import { GithubView } from "./pages/github-detail";
import { Github } from "./pages/github";
import { Attention, useAttentionCount } from "./pages/attention";
import { Home } from "./pages/home";
import { Usage } from "./pages/usage";
import { Inbox } from "./pages/items";
import { ProjectDetail, Projects } from "./pages/projects";
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
      className="mx-auto mt-24 w-72 space-y-3 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-5 shadow-sm"
      onSubmit={(e) => { e.preventDefault(); login.mutate(); }}
    >
      <h1 className="text-xl font-bold">Dory</h1>
      <Label text="Usuario"><Input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus /></Label>
      <Label text="Contraseña"><Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></Label>
      <ErrorText error={login.error} />
      <Button className="w-full" disabled={login.isPending}>Entrar</Button>
    </form>
  );
}

function ThemeToggle() {
  const [dark, setTheme] = useTheme();
  const label = dark ? "Cambiar a modo claro" : "Cambiar a modo oscuro";
  return (
    <Button variant="ghost" aria-label={label} title={label} onClick={() => setTheme(dark ? "light" : "dark")}>
      <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        {dark ? (
          <>
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
          </>
        ) : (
          <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
        )}
      </svg>
    </Button>
  );
}

// ProjectPicker sets the global project that every page filters by.
function ProjectPicker() {
  const project = useProject();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  // Forget a selection that no longer exists (archived or from another database).
  const gone = !!project && !!projects.data && !projects.data.some((p) => p.id === project);
  useEffect(() => { if (gone) setProject(""); }, [gone]);
  return <Select className="w-44 sm:w-52" value={project} onChange={setProject} options={[{ value: "", label: "Todos los proyectos" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />;
}

function Layout() {
  const qc = useQueryClient();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  const logout = useMutation({ mutationFn: api.logout, onSuccess: () => qc.clear() });
  useLiveUpdates(!!me.data);
  const project = useProject();
  // The project board needs the full width: on /projects/$id, or on /projects with a project selected.
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/projects/") || (l.pathname === "/projects" && !!project) });
  const attention = useAttentionCount(!!me.data);
  if (me.isPending) return null;
  if (!me.data) return <Login />;
  const link = "flex min-h-11 shrink-0 items-center rounded px-3 text-sm sm:min-h-0 sm:px-2 sm:py-1 hover:bg-slate-200 dark:hover:bg-slate-800 [&.active]:bg-slate-900 dark:[&.active]:bg-slate-100 [&.active]:text-white dark:[&.active]:text-slate-900";
  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100">
      <nav className="flex flex-wrap items-center gap-1 border-b border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2 py-2 sm:px-4">
        <span className="mr-3 font-bold">Dory</span>
        <div className="order-last -mx-2 flex w-full gap-1 overflow-x-auto px-2 sm:order-none sm:mx-0 sm:w-auto sm:px-0">
          <Link to="/" activeOptions={{ exact: true }} className={link}>Inicio</Link>
          <Link to="/inbox" className={link}>Inbox</Link>
          <Link to="/attention" className={link}>Atención{attention > 0 ? ` (${attention})` : ""}</Link>
          <Link to="/usage" className={link}>Uso</Link>
          <Link to="/github" className={link}>GitHub</Link>
          <Link to="/projects" className={link}>Proyectos</Link>
          <Link to="/environments" className={link}>Entornos</Link>
          <Link to="/memories" className={link}>Memorias</Link>
        </div>
        <div className="ml-auto"><ProjectPicker /></div>
        <span className="truncate text-sm text-slate-500 dark:text-slate-400">{me.data.username}</span>
        <ThemeToggle />
        <Button variant="ghost" onClick={() => logout.mutate()}>Salir</Button>
      </nav>
      <main className={`mx-auto p-2 sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"}`}><Outlet /></main><CommandPalette />
    </div>
  );
}

const root = createRootRoute({ component: Layout });
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: "/", component: Home }),
  createRoute({ getParentRoute: () => root, path: "/inbox", component: Inbox }),
  createRoute({ getParentRoute: () => root, path: "/attention", component: Attention }),
  createRoute({ getParentRoute: () => root, path: "/usage", component: Usage }),
  createRoute({ getParentRoute: () => root, path: "/github", component: Github }),
  createRoute({ getParentRoute: () => root, path: "/github/$owner/$repo/$kind/$number", component: GithubView }),
  createRoute({ getParentRoute: () => root, path: "/projects", component: Projects }),
  createRoute({ getParentRoute: () => root, path: "/projects/$id", component: ProjectDetail }),
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
