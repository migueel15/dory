import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient, useQueries } from "@tanstack/react-query";
import { api, type Job, type Provider } from "../api/client";
import { ProviderIcon, useNodes } from "./launch-picker";
import { ALL_NODES, type SectionProps } from "./settings-node";
import { Button, ErrorText, Input, Select } from "./ui";

// Providers are unique per node: they need a login there. With "all" this
// shows what every node has and asks to pick one to manage.
export function ProvidersSettings({ node }: SectionProps) {
  const [picked, setPicked] = useState<string>();
  if (node !== ALL_NODES) return <NodeProviders node={node} />;
  if (picked) return <NodeProviders node={picked} onBack={() => setPicked(undefined)} />;
  return <ProvidersOverview onPick={setPicked} />;
}

function ProvidersOverview({ onPick }: { onPick: (node: string) => void }) {
  const nodes = useNodes();
  const lists = useQueries({ queries: nodes.map((n) => ({ queryKey: ["node-providers", n.value], queryFn: () => api.nodeProviders(n.value) })) });
  return (
    <div className="space-y-5">
      <h2 className="text-lg font-semibold">Proveedores</h2>
      <p className="text-sm text-slate-600 dark:text-muted">Los proveedores necesitan iniciar sesión en cada nodo. Elige un nodo arriba, o pulsa Gestionar, para cambiarlos.</p>
      <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-border dark:border-border">
        {nodes.map((n, i) => (
          <li key={n.value} className="flex flex-wrap items-center gap-3 px-4 py-3">
            <span className="min-w-32 font-semibold">{n.label}</span>
            <span className="min-w-0 flex-1 text-sm text-slate-600 dark:text-muted">
              {lists[i].isPending ? "Buscando…" : lists[i].error ? "No disponible" : (lists[i].data ?? []).filter((p) => p.installed).map((p) => p.name).join(", ") || "Sin proveedores instalados"}
            </span>
            <Button variant="ghost" onClick={() => onPick(n.value)}>Gestionar</Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

// NodeProviders lists the harnesses one node detects. Each can be turned
// on or off on that node, and signed in or updated there. Calls for another
// node go through Dory to it, so it has to be connected.
function NodeProviders({ node, onBack }: { node: string; onBack?: () => void }) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<string>();
  const list = useQuery({ queryKey: ["node-providers", node], queryFn: () => api.nodeProviders(node) });
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.setProviderEnabled(node, id, enabled),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["node-providers"] });
      qc.invalidateQueries({ queryKey: ["node"] });
    },
  });
  const providers = list.data ?? [];
  const current = providers.find((p) => p.id === selected) ?? providers.find((p) => p.installed) ?? providers[0];
  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <h2 className="text-lg font-semibold">Proveedores</h2>
        {onBack && <Button variant="ghost" onClick={onBack}>Todos los nodos</Button>}
      </div>
      <AddAccount node={node} onAdded={(id) => setSelected(id)} />
      {list.isPending && <p className="text-sm text-slate-500 dark:text-muted">Buscando proveedores…</p>}
      <ErrorText error={list.error ?? toggle.error} />
      {providers.length > 0 && (
        <div className="grid overflow-hidden rounded-xl border border-slate-200 dark:border-border md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]">
          <ul aria-label="Proveedores" className="divide-y divide-slate-200 border-b border-slate-200 dark:divide-border dark:border-border md:border-b-0 md:border-r">
            {providers.map((p) => (
              <li key={p.id} className={`flex items-center gap-3 px-4 py-3 ${p.id === current?.id ? "bg-slate-100 dark:bg-raised" : ""}`}>
                <button type="button" onClick={() => setSelected(p.id)} className="flex min-w-0 flex-1 items-start gap-3 text-left">
                  <ProviderIcon driver={p.id} />
                  <span className="min-w-0">
                    <span className="flex items-baseline gap-2">
                      <span className="font-semibold">{p.name}</span>
                      {p.installed && <span className="font-mono text-xs text-slate-500 dark:text-muted">{version(p)}</span>}
                    </span>
                    <span className="block text-xs text-slate-500 dark:text-muted">{summary(p)}</span>
                  </span>
                </button>
                {p.installed && (
                  <Switch label={`${p.enabled === false ? "Activar" : "Desactivar"} ${p.name}`} checked={p.enabled !== false}
                    disabled={toggle.isPending} onChange={(enabled) => toggle.mutate({ id: p.id, enabled })} />
                )}
              </li>
            ))}
          </ul>
          {current && <ProviderDetail key={`${node}/${current.id}`} node={node} provider={current} />}
        </div>
      )}
    </div>
  );
}

const driver = (p: Provider) => p.driver ?? p.id;
const multi = (p: Provider) => driver(p) === "claude" || driver(p) === "codex";

const version = (p: Provider) => p.version.split(" ")[0] || "instalado";

const summary = (p: Provider) => {
  if (!p.installed) return "No instalado";
  if (p.enabled === false) return "Desactivado";
  if (p.logged_in === undefined) return "Instalado";
  return p.logged_in ? `Con sesión${p.account ? ` · ${p.account}` : ""}` : "Sin sesión";
};

function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={label} title={label} disabled={disabled} onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${checked ? "bg-accent" : "bg-slate-300 dark:bg-raised"}`}>
      <span className={`inline-block size-5 rounded-full bg-white shadow transition-transform ${checked ? "translate-x-5" : "translate-x-0.5"}`} />
    </button>
  );
}

// ProviderDetail shows the state of one harness on a node and signs in or
// updates it there.
function ProviderDetail({ node, provider: p }: { node: string; provider: Provider }) {
  const qc = useQueryClient();
  const [job, setJob] = useState<string>();
  const start = useMutation({
    mutationFn: (kind: Job["kind"]) => api.providerJob(node, p.id, kind),
    onSuccess: (j) => setJob(j.id),
  });
  const state = useQuery({
    queryKey: ["job", job], queryFn: () => api.job(node, job!), enabled: !!job,
    refetchInterval: (q) => (q.state.data?.done ? false : 1000),
  });
  useEffect(() => {
    if (state.data?.done) {
      qc.invalidateQueries({ queryKey: ["node"] });
      qc.invalidateQueries({ queryKey: ["node-providers"] });
    }
  }, [state.data?.done]); // eslint-disable-line react-hooks/exhaustive-deps
  const admin = p.installed && p.capabilities.includes("login");
  return (
    <div className="min-w-0 space-y-4 p-4">
      <div className="flex items-center gap-3">
        <ProviderIcon driver={p.id} />
        <h3 className="text-lg font-semibold">{p.name}</h3>
        {p.installed && <span className="ml-auto font-mono text-xs text-slate-500 dark:text-muted">{version(p)}</span>}
      </div>
      {multi(p) && <AccountForm key={p.id + (p.home ?? "")} node={node} provider={p} />}
      <dl className="divide-y divide-slate-200 text-sm dark:divide-border">
        <Row label="Estado" value={summary(p)} />
        <Row label="Modelos" value={p.models.length ? p.models.map((m) => m.name).join(", ") : "—"} />
        <Row label="Uso" value={p.capabilities.includes("usage") ? "Informa de sus límites" : "No informa de sus límites"} />
      </dl>
      {admin && (
        <div className="flex flex-wrap gap-2">
          {!p.logged_in && <Button variant="ghost" disabled={start.isPending} onClick={() => start.mutate("login")}>Iniciar sesión</Button>}
          <Button variant="ghost" disabled={start.isPending} onClick={() => start.mutate("update")}>Actualizar</Button>
        </div>
      )}
      {p.capabilities.includes("install") && (
        <div className="flex flex-wrap gap-2">
          <Button variant="ghost" disabled={start.isPending || (!!job && !state.data?.done)} onClick={() => start.mutate("install")}>Instalar</Button>
        </div>
      )}
      <ErrorText error={start.error ?? state.error} />
      {job && state.data && (
        <div className="rounded-md bg-slate-100 p-2 dark:bg-raised">
          <p className="mb-1 flex items-center gap-2 text-xs font-medium">{{ login: "Inicio de sesión", update: "Actualización", install: "Instalación" }[state.data.kind]} · {state.data.done ? (state.data.error ? `terminó con error (${state.data.error})` : "terminado") : "en curso…"}
            <button type="button" className="ml-auto text-slate-500" onClick={() => setJob(undefined)}>cerrar</button></p>
          <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all font-mono text-xs"><Linkify text={state.data.output || "Esperando salida…"} linkLabel={(driver(p) === "claude" || driver(p) === "antigravity") && state.data.kind === "login" && !state.data.done ? `Abrir ${p.name} en el navegador` : undefined} /></pre>
          {!state.data.done && (driver(p) === "claude" || driver(p) === "antigravity") && state.data.kind === "login" && state.data.accepts_code !== undefined && (
            <LoginCodeForm key={job} node={node} id={job} acceptsCode={state.data.accepts_code} />
          )}
        </div>
      )}
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1 py-2 sm:grid-cols-[8rem_1fr]">
      <dt className="font-medium">{label}</dt>
      <dd className="text-slate-600 dark:text-muted">{value}</dd>
    </div>
  );
}

function LoginCodeForm({ node, id, acceptsCode }: { node: string; id: string; acceptsCode: boolean }) {
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>();
  if (sent) return <p role="status" className="mt-2 text-sm">Código enviado. Esperando la confirmación de Claude…</p>;
  if (!acceptsCode) return <ErrorText error={error} />;
  return (
    <form className="mt-3 space-y-2" onSubmit={async (event) => {
      event.preventDefault();
      const form = event.currentTarget;
      const code = String(new FormData(form).get("code") ?? "").trim();
      if (!code || sending) return;
      setSending(true);
      setError(undefined);
      try {
        await api.submitLoginCode(node, id, code);
        form.reset();
        setSent(true);
      } catch (error) {
        setError(error);
      } finally {
        setSending(false);
      }
    }}>
      <label className="block space-y-1 text-sm">
        <span>Código de autorización</span>
        <Input name="code" type="password" autoComplete="off" autoCapitalize="none" spellCheck={false} maxLength={2048} required />
      </label>
      <p className="text-sm">Abre el enlace de Claude arriba. Si Claude te muestra un código, pégalo aquí.</p>
      <Button type="submit" disabled={sending}>Confirmar código</Button>
      <ErrorText error={error} />
    </form>
  );
}

// Linkify makes the URLs a login prints clickable.
function Linkify({ text, linkLabel }: { text: string; linkLabel?: string }) {
  return <>{text.split(/(https?:\/\/\S+)/g).map((part, i) => (/^https?:\/\//.test(part) ? <a key={i} href={part} target="_blank" rel="noreferrer" className="text-blue-600 underline dark:text-accent-text">{linkLabel ?? part}</a> : part))}</>;
}

const defaultDirs: Record<string, string> = { claude: "CLAUDE_CONFIG_DIR", codex: "CODEX_HOME" };

// AddAccount signs a harness in a second time with its own configuration directory.
function AddAccount({ node, onAdded }: { node: string; onAdded: (id: string) => void }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [drv, setDrv] = useState("claude");
  const [name, setName] = useState("");
  const [home, setHome] = useState("");
  const add = useMutation({
    mutationFn: () => api.addAccount(node, { driver: drv, name, home }),
    onSuccess: (a) => {
      qc.invalidateQueries({ queryKey: ["node-providers"] });
      qc.invalidateQueries({ queryKey: ["node"] });
      setOpen(false); setName(""); setHome("");
      onAdded(a.id);
    },
  });
  if (!open) return <Button variant="ghost" onClick={() => setOpen(true)}>Añadir cuenta</Button>;
  return (
    <form className="space-y-2 rounded-xl border border-slate-200 p-4 dark:border-border" onSubmit={(e) => { e.preventDefault(); if (name.trim()) add.mutate(); }}>
      <div className="flex flex-wrap gap-2">
        <Select className="w-44" value={drv} onChange={setDrv} options={[{ value: "claude", label: "Claude Code" }, { value: "codex", label: "Codex" }]} />
        <Input className="min-w-40 flex-1" placeholder="Nombre visible (p. ej. Trabajo)" value={name} onChange={(e) => setName(e.target.value)} required />
      </div>
      <Input placeholder={`Carpeta (${defaultDirs[drv]}); vacío = una propia de Dory`} value={home} onChange={(e) => setHome(e.target.value)} />
      <div className="flex gap-2">
        <Button type="submit" disabled={add.isPending}>Añadir</Button>
        <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancelar</Button>
      </div>
      <ErrorText error={add.error} />
    </form>
  );
}

// AccountForm edits the display name and configuration directory of one
// account and removes it when it is not the default one.
function AccountForm({ node, provider: p }: { node: string; provider: Provider }) {
  const qc = useQueryClient();
  const extra = p.id !== driver(p);
  const [name, setName] = useState(p.name === (driver(p) === "claude" ? "Claude Code" : "Codex") && !extra ? "" : p.name);
  const [home, setHome] = useState(p.home ?? "");
  const done = () => { qc.invalidateQueries({ queryKey: ["node-providers"] }); qc.invalidateQueries({ queryKey: ["node"] }); };
  const save = useMutation({ mutationFn: () => api.updateAccount(node, p.id, { name, home }), onSuccess: done });
  const remove = useMutation({ mutationFn: () => api.removeAccount(node, p.id), onSuccess: done });
  return (
    <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <label className="block space-y-1 text-sm">
        <span>Nombre visible</span>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={extra ? "" : "Por defecto"} required={extra} />
      </label>
      <label className="block space-y-1 text-sm">
        <span>Carpeta de configuración ({defaultDirs[driver(p)]})</span>
        <Input value={home} onChange={(e) => setHome(e.target.value)} placeholder={extra ? "" : "Por defecto del arnés"} required={extra} />
      </label>
      <div className="flex gap-2">
        <Button type="submit" disabled={save.isPending}>Guardar</Button>
        {extra && <Button type="button" variant="ghost" disabled={remove.isPending} onClick={() => remove.mutate()}>Eliminar cuenta</Button>}
      </div>
      <ErrorText error={save.error ?? remove.error} />
    </form>
  );
}
