import { StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useLocation, RouterProvider, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { Button, ErrorText, Input, Label, field } from "./components/ui";
import { Environments, Memories, memorySearch } from "./pages/crud";
import { Inbox } from "./pages/items";
import { ProjectDetail, Projects } from "./pages/projects";
import { applyTheme, useTheme, type Theme } from "./theme";
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

function ThemeSelect() {
  const [theme, setTheme] = useTheme();
  return (
    <select aria-label="Tema" className={`${field} w-auto`} value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
      <option value="system">Sistema</option>
      <option value="light">Claro</option>
      <option value="dark">Oscuro</option>
    </select>
  );
}

function Layout() {
  const qc = useQueryClient();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  const logout = useMutation({ mutationFn: api.logout, onSuccess: () => qc.clear() });
  useLiveUpdates(!!me.data);
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/projects/") });
  if (me.isPending) return null;
  if (!me.data) return <Login />;
  const link = "flex min-h-11 shrink-0 items-center rounded px-3 text-sm sm:min-h-0 sm:px-2 sm:py-1 hover:bg-slate-200 dark:hover:bg-slate-800 [&.active]:bg-slate-900 dark:[&.active]:bg-slate-100 [&.active]:text-white dark:[&.active]:text-slate-900";
  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100">
      <nav className="flex flex-wrap items-center gap-1 border-b border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2 py-2 sm:px-4">
        <span className="mr-3 font-bold">Dory</span>
        <div className="order-last -mx-2 flex w-full gap-1 overflow-x-auto px-2 sm:order-none sm:mx-0 sm:w-auto sm:px-0">
          <Link to="/" className={link}>Inbox</Link>
          <Link to="/projects" className={link}>Proyectos</Link>
          <Link to="/environments" className={link}>Entornos</Link>
          <Link to="/memories" className={link}>Memorias</Link>
        </div>
        <span className="ml-auto truncate text-sm text-slate-500 dark:text-slate-400">{me.data.username}</span>
        <ThemeSelect />
        <Button variant="ghost" onClick={() => logout.mutate()}>Salir</Button>
      </nav>
      <main className={`mx-auto p-2 sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"}`}><Outlet /></main>
    </div>
  );
}

const root = createRootRoute({ component: Layout });
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: "/", component: Inbox }),
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
