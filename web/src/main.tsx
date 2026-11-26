import { ChartBar,DotsThreeCircle,Gear,GitPullRequest,House,IconContext,Kanban,Moon,SidebarSimple,SignOut,Sun,X } from "@phosphor-icons/react";
import { QueryClient,QueryClientProvider,useQueryClient } from "@tanstack/react-query";
import { Link,Outlet,RouterProvider,createRootRoute,createRoute,createRouter,lazyRouteComponent,redirect,useLocation,useNavigate } from "@tanstack/react-router";
import { StrictMode,useEffect,useRef,useState } from "react";
import { createRoot } from "react-dom/client";
import { ApiError } from "./api/client";
import { useLiveUpdates } from "./api/live";
import { useLogin,useLogout,useMe,useSetupNeeded,useVersion } from "./api/queries/session";
import { useThreadsAttention } from "./api/queries/threads";
import { CommandPalette } from "./components/commands";
import { MeshUpdateButton } from "./components/mesh-update";
import { NoticeAlerts,NoticeInbox,NoticesToggle } from "./components/notices";
import { ProjectSwitcher } from "./components/project-switcher";
import { listenShortcuts,useCommand } from "./components/shortcuts";
import { TooltipHost } from "./components/tooltip";
import { Button,Input,Label,errMsg } from "./components/ui";
import { UsageQuick } from "./components/usage-quick";
import { Tasks } from "./features/board/tasks";
import { Github } from "./features/github/github";
import { SettingsModal } from "./features/settings/settings-modal";
import "./index.css";
import { events } from "./lib/store";
import { WindowClose } from "./lib/window-close";
import { Environments,Memories,memorySearch } from "./pages/crud";
import { Threads,ThreadsHome } from "./pages/threads";
import { applyUiFont } from "./lib/fonts";
import { applyScale,listenZoomKeys } from "./scale";
import { applyTheme,useTheme } from "./theme";

applyTheme();
applyScale();
applyUiFont();
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
  const setup = useSetupNeeded({ retry: false });
  const first = !!setup.data?.needed;
  const minimumLength = 8;
  const action = first ? "Crear cuenta" : "Iniciar sesión";
  const login = useLogin(first, username, password, {
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
  const v = useVersion({ staleTime: Infinity });
  if (!v.data) return null;
  const { revision, short, time, modified } = v.data;
  const cls = "font-mono text-xs text-slate-400 dark:text-muted hover:underline";
  if (revision === "dev") return <span className={cls} title="Sin datos de VCS">dev</span>;
  const title = `${time ? new Date(time).toLocaleString() : revision}${modified ? " +cambios" : ""}`;
  return <a className={cls} href={`https://github.com/migueel15/dory/commit/${revision}`} target="_blank" rel="noreferrer" title={title}>{short}{modified && "+"}</a>;
}

function Layout() {
  const qc = useQueryClient();
  const me = useMe({ retry: false });
  const logout = useLogout({ onSuccess: () => qc.clear() });
  useLiveUpdates(!!me.data);
  // The board, threads and pull requests need the full width.
  const wide = useLocation({ select: (l) => l.pathname.startsWith("/threads") || /^\/github\/[^/]+\/[^/]+\/(pulls|issues)\//.test(l.pathname) || l.pathname === "/tasks" });
  // Threads that wait for the user show as a count on their link.
  const threads = useThreadsAttention({ enabled: !!me.data, refetchInterval: 30_000 });
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

  const navigateWithoutLosingDraft = async (search: boolean) => {
    if (search && path.startsWith("/threads")) {
      events.focusThreadSearch.emit();
      return;
    }
    if (document.querySelector('[data-unsaved-draft="true"]')) {
      window.alert("Hay un borrador sin enviar. Envíalo o vacíalo antes de cambiar de vista.");
      return;
    }
    await navigate({ to: "/threads" });
    if (search) window.setTimeout(() => events.focusThreadSearch.emit(), 0);
  };
  useCommand("new-thread", () => { void navigateWithoutLosingDraft(false); });
  useCommand("search-threads", () => { void navigateWithoutLosingDraft(true); });
  useEffect(() => {
    if (!me.data) return;
    window.addEventListener("keydown", listenShortcuts);
    return () => window.removeEventListener("keydown", listenShortcuts);
  }, [!!me.data]);
  useEffect(() => { setMenu(false); }, [path]);
  // While the phone keyboard is up (the viewport shrinks well below its tallest
  // height) the tab bar steps aside so the composer keeps the room.
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    let tallest = vv.height, width = vv.width;
    const check = () => {
      // Rotating changes the width: start measuring again.
      if (vv.width !== width) { width = vv.width; tallest = vv.height; }
      tallest = Math.max(tallest, vv.height);
      document.documentElement.classList.toggle("typing", vv.height < tallest - 150);
    };
    vv.addEventListener("resize", check);
    return () => { vv.removeEventListener("resize", check); document.documentElement.classList.remove("typing"); };
  }, []);
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
  const railIco = "size-6 shrink-0";
  const link = "flex min-h-11 shrink-0 items-center gap-2.5 overflow-hidden rounded px-3 text-sm font-medium text-slate-800 dark:text-text hover:bg-slate-200 dark:hover:bg-raised [&.active]:bg-slate-900 dark:[&.active]:bg-accent [&.active]:text-white dark:[&.active]:text-white";
  // Desktop left rail: always vertical icons, darker muted gray at rest,
  // lighter hover background + whiter border when active.
  const railLink = "relative grid size-10 shrink-0 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-200/70 hover:text-slate-900 dark:text-zinc-500 dark:hover:bg-raised dark:hover:text-text [&.active]:bg-slate-200/80 [&.active]:text-slate-900 dark:[&.active]:bg-hover dark:[&.active]:text-white";
  const railDivider = <div className="my-1.5 w-6 border-t border-slate-200 dark:border-border" />;
  const railLinks = (
    <>
      <Link to="/threads" className={railLink} aria-label="Inicio" title="Inicio"><House weight="duotone" className={railIco} aria-hidden="true" />{attention > 0 && <span className="absolute right-1.5 top-1.5 size-2 rounded-full bg-orange-500" />}</Link>
      <Link to="/tasks" className={railLink} aria-label="Tareas" title="Tareas"><Kanban weight="duotone" className={railIco} aria-hidden="true" /></Link>
      <Link to="/github" className={railLink} aria-label="GitHub" title="GitHub"><GitPullRequest weight="duotone" className={railIco} aria-hidden="true" /></Link>
      {railDivider}
      <Link to="/usage" className={railLink} aria-label="Uso" title="Uso"><ChartBar weight="duotone" className={railIco} aria-hidden="true" /></Link>
    </>
  );
  // Phone tabs: the main sections are one tap away; the rest sits under "Más".
  const tab = "flex h-14 flex-col items-center justify-center gap-0.5 text-[0.6875rem] font-medium text-slate-500 dark:text-muted [&.active]:text-slate-900 dark:[&.active]:text-white";
  const moreActive = path.startsWith("/github") || path.startsWith("/usage");
  const moreLinks = (
    <>
      <Link to="/github" className={link}><GitPullRequest weight="duotone" className={ico} aria-hidden="true" />GitHub</Link>
      <Link to="/usage" className={link}><ChartBar weight="duotone" className={ico} aria-hidden="true" />Uso</Link>
    </>
  );
  return (
    <div className="min-h-dvh bg-slate-50 dark:bg-canvas text-slate-900 dark:text-text sm:flex sm:h-screen sm:flex-col sm:bg-white sm:dark:bg-surface">
      <header className="hidden h-12 shrink-0 items-center justify-between pl-5 pr-3 pt-1 sm:flex">
        <div className="flex items-center gap-2">
          <Link to="/threads" className="flex items-center gap-2 font-bold">Dory<DevBadge /></Link>
          {path.startsWith("/threads") && (
            <button type="button" onClick={() => events.toggleThreadList.emit()} aria-label="Mostrar u ocultar barra lateral" title="Mostrar u ocultar barra lateral"
              className="grid size-9 place-items-center rounded-lg text-slate-500 hover:bg-slate-200/70 hover:text-slate-900 dark:text-zinc-500 dark:hover:bg-raised dark:hover:text-text">
              <SidebarSimple weight="regular" className="size-6" aria-hidden="true" />
            </button>
          )}
        </div>
        <WindowClose />
      </header>
      <div className="min-h-0 flex-1 sm:flex">
      <aside className="hidden h-full w-14 shrink-0 flex-col items-center overflow-hidden bg-white dark:bg-surface sm:flex">
        <nav aria-label="Navegación principal" className="flex min-h-0 flex-1 flex-col items-center gap-1 overflow-y-auto overflow-x-hidden pt-3 pb-4">{railLinks}</nav>
        <div className="shrink-0 pb-2"><ProjectSwitcher folded /></div>
        <SidebarFooter username={me.data.username} onLogout={() => logout.mutate()} update={<MeshUpdateButton />}
          controls={[<NoticeInbox key="i" />, <SettingsLink key="settings" />, <UsageQuick key="u" />]} />
      </aside>
      {/* On desktop the current view is a card framed by the sidebar background. */}
      <div className="min-w-0 flex-1 sm:mb-2 sm:mr-2 sm:overflow-auto sm:rounded-lg sm:border sm:border-slate-200 sm:bg-slate-50 sm:dark:border-border sm:dark:bg-canvas">
      <nav className="sticky top-0 z-30 border-b border-slate-200 dark:border-border bg-white dark:bg-surface px-2 sm:hidden">
        <div className="flex h-12 items-center gap-1">
          <Link to="/" className="mr-1 flex shrink-0 items-center gap-2 font-bold">Dory<DevBadge /></Link>
          {/* The project is one tap away on phones, without opening the menu. */}
          <div className="min-w-0 flex-1"><ProjectSwitcher compact /></div>
        </div>
      </nav>
      <dialog
        ref={drawer}
        id="mobile-navigation"
        aria-labelledby="mobile-navigation-title"
        className="mobile-drawer fixed inset-x-0 bottom-0 top-auto m-0 max-h-[85dvh] w-full max-w-none flex-col rounded-t-2xl border-t border-slate-200 bg-white p-0 text-slate-900 shadow-2xl dark:border-border dark:bg-surface dark:text-text open:flex sm:hidden"
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
            <h2 id="mobile-navigation-title" className="font-bold">Más</h2>
          </div>
          <button type="button" autoFocus aria-label="Cerrar menú" onClick={() => setMenu(false)} className="grid size-11 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
            <X weight="regular" className="size-5" aria-hidden="true" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-4">
          <nav aria-label="Navegación móvil" className="flex flex-col gap-1" onClick={(event) => {
            if ((event.target as HTMLElement).closest("a")) setMenu(false);
          }}>{moreLinks}</nav>
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
      <main className={`mx-auto p-2 pb-[calc(0.5rem_+_var(--tabbar))] sm:p-4 ${wide ? "max-w-none" : "max-w-6xl"}`}><Outlet /></main></div>
      </div>
      <nav aria-label="Navegación móvil" className="mobile-tabbar fixed inset-x-0 bottom-0 z-30 grid grid-cols-3 border-t border-slate-200 bg-white pb-[env(safe-area-inset-bottom)] dark:border-border dark:bg-surface sm:hidden">
        <Link to="/threads" className={`${tab} relative`}><House className="size-6" aria-hidden="true" />Inicio{attention > 0 && <span className="absolute left-1/2 top-1 ml-2 min-w-4 rounded-full bg-orange-500 px-1 text-center text-[0.625rem] font-semibold leading-4 text-white" aria-label={`${attention} te necesitan`}>{attention}</span>}</Link>
        <Link to="/tasks" className={tab}><Kanban className="size-6" aria-hidden="true" />Tareas</Link>
        <button type="button" aria-controls="mobile-navigation" aria-expanded={menu} onClick={() => setMenu(!menu)} className={`${tab} ${menu || moreActive ? "text-slate-900 dark:text-white" : ""}`}><DotsThreeCircle className="size-6" aria-hidden="true" />Más</button>
      </nav><CommandPalette /><TooltipHost /><NoticeAlerts />
    </div>
  );
}

// Pages other than home, the thread list and attention load on demand, so the
// first paint downloads only what it shows; the rest is fetched when the
// browser is idle (and on hover, see defaultPreload) to keep navigation instant.
const pages = {
  thread: () => import("./features/thread/thread"),
  githubDetail: () => import("./features/github/github-detail"),
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
  createRoute({ getParentRoute: () => root, path: "/", beforeLoad: () => { throw redirect({ to: "/threads" }); } }),
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

function SidebarFooter({ username, onLogout, controls, update }: { username: string; onLogout: () => void; controls: React.ReactElement[]; update: React.ReactElement | null }) {
  return (
    <div className="flex w-full shrink-0 flex-col items-center gap-1 border-t border-slate-200 py-3 dark:border-border [&_svg]:size-6">
      {controls.map((c) => (
        <div key={c.key} className="grid size-10 place-items-center text-slate-500 dark:text-zinc-500 [&>*]:!min-h-0 [&>*]:!min-w-0 [&>*]:!size-10 [&>*]:!p-0 [&>*]:grid [&>*]:place-items-center [&>*]:!rounded-lg [&>*]:hover:text-slate-900 dark:[&>*]:hover:text-text">
          {c}
        </div>
      ))}
      {update}
      <Button variant="bare" aria-label="Cerrar sesión" title={`Cerrar sesión (${username})`} onClick={onLogout} className="grid size-10 shrink-0 place-items-center !rounded-lg !p-0 text-slate-500 hover:text-slate-900 dark:text-zinc-500 dark:hover:text-text">
        <SignOut weight="regular" className="size-6" aria-hidden="true" />
      </Button>
    </div>
  );
}

