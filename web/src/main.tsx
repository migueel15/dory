import { StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, RouterProvider, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { ApiError, api } from "./api/client";
import { Button, ErrorText, Input, Label } from "./components/ui";
import { Environments, Memories } from "./pages/crud";
import { Inbox } from "./pages/items";
import { ProjectDetail, Projects } from "./pages/projects";
import "./index.css";

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
      className="mx-auto mt-24 w-72 space-y-3 rounded-lg border border-slate-200 bg-white p-5 shadow-sm"
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

function Layout() {
  const qc = useQueryClient();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  const logout = useMutation({ mutationFn: api.logout, onSuccess: () => qc.clear() });
  if (me.isPending) return null;
  if (!me.data) return <Login />;
  const link = "rounded px-2 py-1 text-sm hover:bg-slate-200 [&.active]:bg-slate-900 [&.active]:text-white";
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <nav className="flex items-center gap-1 border-b border-slate-200 bg-white px-4 py-2">
        <span className="mr-3 font-bold">Dory</span>
        <Link to="/" className={link}>Inbox</Link>
        <Link to="/projects" className={link}>Proyectos</Link>
        <Link to="/environments" className={link}>Entornos</Link>
        <Link to="/memories" className={link}>Memorias</Link>
        <span className="ml-auto text-sm text-slate-500">{me.data.username}</span>
        <Button variant="ghost" onClick={() => logout.mutate()}>Salir</Button>
      </nav>
      <main className="mx-auto max-w-6xl p-4"><Outlet /></main>
    </div>
  );
}

const root = createRootRoute({ component: Layout });
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: "/", component: Inbox }),
  createRoute({ getParentRoute: () => root, path: "/projects", component: Projects }),
  createRoute({ getParentRoute: () => root, path: "/projects/$id", component: ProjectDetail }),
  createRoute({ getParentRoute: () => root, path: "/environments", component: Environments }),
  createRoute({ getParentRoute: () => root, path: "/memories", component: Memories }),
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
