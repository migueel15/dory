import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { CaretDown, CaretRight, CircleNotch, ClipboardText, GitBranch, GitMerge, GitPullRequest, Plus, Rocket, X } from "@phosphor-icons/react";
import { api, COLUMN_LABELS, repoName, type Card, type Column, type Plan, type Stage } from "../api/client";
import { Button, ErrorText, Input, Label, Md, Modal, Select, Textarea } from "../components/ui";
import { useLaunchForm } from "../components/launch-picker";
import { useProject } from "../project";
import { StatusDot } from "./thread";

// Fixed meaning of each state color (requirement RF-48).
const TONE = {
  working: "text-blue-500", attention: "text-orange-500", ci: "text-yellow-500", review: "text-amber-500",
  ready: "text-emerald-500", merged: "text-purple-500", failed: "text-red-500", idle: "text-slate-400",
};

// slot is the single state icon of a card (requirement RF-47), with its meaning for the tooltip.
function slot(c: Card): { icon: React.ReactNode; label: string } {
  if (c.threads.some((t) => t.status === "working")) return { icon: <CircleNotch className={`size-4 animate-spin ${TONE.working}`} />, label: "Un agente está trabajando" };
  const pull = c.pull;
  if (c.pr_state === "merged") return { icon: <GitMerge className={`size-4 ${TONE.merged}`} weight="fill" />, label: "PR fusionada" };
  if (c.pr_state === "closed") return { icon: <GitPullRequest className={`size-4 ${TONE.idle}`} />, label: "PR cerrada sin fusionar" };
  if (pull) {
    if (pull.ci === "FAILURE" || pull.ci === "ERROR") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "CI fallido" };
    if (pull.ci === "PENDING" || pull.ci === "EXPECTED") return { icon: <GitPullRequest className={`size-4 ${TONE.ci}`} weight="fill" />, label: "CI en ejecución" };
    if (pull.mergeable === "CONFLICTING") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "Conflictos de merge" };
    if (pull.review_decision === "CHANGES_REQUESTED" || pull.review_requests.length > 0) return { icon: <GitPullRequest className={`size-4 ${TONE.review}`} weight="fill" />, label: pull.review_decision === "CHANGES_REQUESTED" ? "Cambios pedidos" : "Review pendiente" };
    if (pull.draft) return { icon: <GitPullRequest className={`size-4 ${TONE.working}`} />, label: "PR en draft" };
    return { icon: <GitPullRequest className={`size-4 ${TONE.ready}`} weight="fill" />, label: "Lista para mergear" };
  }
  if (c.threads.some((t) => t.status === "waiting")) return { icon: <span className="size-2.5 rounded-full bg-orange-500" />, label: "Necesita tu respuesta" };
  if (c.threads.some((t) => t.status === "error")) return { icon: <span className="size-2.5 rounded-full bg-red-500" />, label: "El hilo falló" };
  return { icon: <span className="size-2.5 rounded-full bg-slate-300 dark:bg-slate-600" />, label: "Inactivo" };
}

// Only these columns can be chosen by hand; the rest come from threads and pull requests.
const MANUAL: Partial<Record<Column, Stage>> = { inbox: "inbox", ready: "ready", cancelled: "cancelled" };

// Tasks is the kanban, the main view (requirement RF-28).
export function Tasks() {
  const project = useProject();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { task?: string };
  const board = useQuery({ queryKey: ["board", project], queryFn: () => api.board(project || undefined), refetchInterval: 30_000 });
  const [adding, setAdding] = useState(false);
  const [fold, setFold] = useState<Record<string, boolean>>({ done: true, cancelled: true });
  const [drag, setDrag] = useState<string>();
  const move = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: Stage }) => api.tasks.update(id, { stage }),
    onSettled: () => qc.invalidateQueries({ queryKey: ["board"] }),
  });
  const cards = board.data?.cards ?? [];
  const open = (id?: string) => navigate({ to: "/tasks", search: (id ? { task: id } : {}) as never });
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold">Tareas</h1>
        <Button className="ml-auto" onClick={() => setAdding(true)}><Plus className="size-4" />Nueva tarea</Button>
      </div>
      <ErrorText error={board.error ?? move.error} />
      <div className="flex gap-3 overflow-x-auto pb-4">
        {(board.data?.columns ?? []).map((col) => {
          const list = cards.filter((c) => c.column === col);
          const folded = fold[col] ?? false;
          const shown = folded ? list.slice(0, 0) : col === "done" || col === "cancelled" ? list.slice(-10) : list;
          return (
            <section key={col} className={`flex w-72 shrink-0 flex-col rounded-lg bg-slate-100 p-2 dark:bg-surface ${drag && MANUAL[col] ? "outline-dashed outline-1 outline-slate-400" : ""}`}
              onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }}
              onDrop={() => { if (drag && MANUAL[col]) move.mutate({ id: drag, stage: MANUAL[col]! }); setDrag(undefined); }}>
              <button type="button" onClick={() => setFold({ ...fold, [col]: !folded })} className="mb-2 flex items-center gap-1.5 px-1 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-muted">
                {folded ? <CaretRight className="size-3" /> : <CaretDown className="size-3" />}{COLUMN_LABELS[col]}<span className="ml-auto font-mono">{list.length}</span>
              </button>
              <div className="space-y-2">
                {shown.map((c) => <TaskCard key={c.id} c={c} onOpen={() => open(c.id)} onDrag={() => setDrag(c.id)} />)}
              </div>
            </section>
          );
        })}
      </div>
      {adding && <NewTask onClose={() => setAdding(false)} onCreated={(id) => { setAdding(false); open(id); }} />}
      {search.task && <TaskDetail id={search.task} onClose={() => open()} />}
    </div>
  );
}

function TaskCard({ c, onOpen, onDrag }: { c: Card; onOpen: () => void; onDrag: () => void }) {
  const s = slot(c);
  return (
    <button type="button" draggable onDragStart={onDrag} onClick={onOpen}
      className="block w-full rounded-md border border-slate-200 bg-white p-2.5 text-left text-sm hover:border-slate-400 dark:border-border dark:bg-canvas dark:hover:border-border-strong">
      <div className="flex items-start gap-2">
        <span className="min-w-0 flex-1 font-medium leading-snug">{c.title}</span>
        <span className="mt-0.5 grid size-4 shrink-0 place-items-center" title={s.label} aria-label={s.label}>{s.icon}</span>
      </div>
      {(c.branch || c.pr_number) && (
        <div className="mt-1.5 flex min-w-0 items-center gap-2 font-mono text-[11px] text-slate-500 dark:text-muted">
          {c.branch && <span className="flex min-w-0 items-center gap-1"><GitBranch className="size-3 shrink-0" /><span className="truncate">{c.branch}</span></span>}
          {c.pr_number && (c.pull?.url
            ? <a href={c.pull.url} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()} className="shrink-0 hover:underline">#{c.pr_number}</a>
            : <span className="shrink-0">#{c.pr_number}</span>)}
        </div>
      )}
      <div className="mt-1.5 flex items-center gap-1.5 text-[11px] text-slate-500 dark:text-muted">
        {c.issue_number && <span className="font-mono">issue #{c.issue_number}</span>}
        {c.has_plan && <span className="flex items-center gap-0.5" title="Tiene plan"><ClipboardText className="size-3" />plan</span>}
        {c.blockers.length > 0 && <span className="text-orange-600" title="Depende de tareas sin terminar">bloqueada</span>}
        {c.threads.length > 0 && <span className="ml-auto">{c.threads.length} hilo{c.threads.length > 1 ? "s" : ""}</span>}
      </div>
    </button>
  );
}

function NewTask({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const current = useProject();
  const [project, setProject] = useState(current);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [issue, setIssue] = useState("");
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const create = useMutation({
    mutationFn: () => api.tasks.create({ project_id: project || null, title: title.trim(), description, issue_number: issue ? Number(issue) : null }),
    onSuccess: (t) => onCreated(t.id),
  });
  return (
    <Modal title="Nueva tarea" onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (title.trim()) create.mutate(); }}>
        <Label text="Título"><Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} /></Label>
        <Label text="Descripción"><Textarea rows={5} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Qué cambiar y cómo comprobarlo" /></Label>
        <div className="grid grid-cols-2 gap-2">
          <Label text="Proyecto"><Select value={project} onChange={setProject} options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} /></Label>
          <Label text="Issue"><Input inputMode="numeric" value={issue} onChange={(e) => setIssue(e.target.value.replace(/\D/g, ""))} placeholder="Opcional" /></Label>
        </div>
        <ErrorText error={create.error} />
        <Button disabled={!title.trim() || create.isPending}>Crear</Button>
      </form>
    </Modal>
  );
}

const SECTIONS = [["goal", "Objetivo"], ["scope", "Alcance"], ["where", "Dónde"], ["design", "Diseño"], ["requirements", "Requisitos"], ["validation", "Validación"], ["risks", "Riesgos"]] as const;

function TaskDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const card = useQuery({ queryKey: ["card", id], queryFn: () => api.card(id) });
  const plans = useQuery({ queryKey: ["plans", id], queryFn: () => api.plans(id) });
  const deps = useQuery({ queryKey: ["dependencies", id], queryFn: () => api.dependencies(id) });
  const board = useQuery({ queryKey: ["board", ""], queryFn: () => api.board() });
  const repos = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list() });
  const [draft, setDraft] = useState<{ title: string; description: string; issue: string; pr: string; repo: string }>();
  const c = card.data;
  useEffect(() => { if (c && !draft) setDraft({ title: c.title, description: c.description, issue: c.issue_number?.toString() ?? "", pr: c.pr_number?.toString() ?? "", repo: c.repository_id ?? "" }); }, [c]); // eslint-disable-line react-hooks/exhaustive-deps
  const refresh = () => { qc.invalidateQueries({ queryKey: ["card", id] }); qc.invalidateQueries({ queryKey: ["board"] }); };
  const save = useMutation({
    mutationFn: () => api.tasks.update(id, { title: draft!.title, description: draft!.description, issue_number: draft!.issue ? Number(draft!.issue) : null, pr_number: draft!.pr ? Number(draft!.pr) : null, repository_id: draft!.repo || null }),
    onSuccess: refresh,
  });
  const stage = useMutation({ mutationFn: (s: Stage) => api.tasks.update(id, { stage: s }), onSuccess: refresh });
  const dep = useMutation({
    mutationFn: ({ other, add }: { other: string; add: boolean }) => (add ? api.addDependency(id, other) : api.removeDependency(id, other)).then(() => undefined),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["dependencies", id] }); refresh(); },
  });
  const archive = useMutation({ mutationFn: () => api.tasks.archive(id), onSuccess: () => { refresh(); onClose(); } });
  const form = useLaunchForm({ section: "task" });
  const launch = useMutation({
    mutationFn: () => { form.remember(); return api.launch(id, form.value); },
    onSuccess: (r) => navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }),
  });
  const define = useMutation({
    mutationFn: () => {
      form.remember();
      const text = `Modo plan. Define la tarea de Dory ${id} antes de implementarla; no cambies código. Pregúntame lo que necesites y propón el plan con propose_plan.\n\n# ${c!.title}\n\n${c!.description}`;
      return api.createThread({ ...form.value, project_id: c!.project_id, task_id: id, title: `Definir: ${c!.title}`, text });
    },
    onSuccess: (r) => navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }),
  });
  if (!c || !draft) return <Modal title="Tarea" onClose={onClose}><ErrorText error={card.error} /></Modal>;
  const dirty = draft.title !== c.title || draft.description !== c.description || draft.issue !== (c.issue_number?.toString() ?? "") || draft.pr !== (c.pr_number?.toString() ?? "") || draft.repo !== (c.repository_id ?? "");
  const others = (board.data?.cards ?? []).filter((o) => o.id !== id && (!c.project_id || o.project_id === c.project_id));
  const latest = [...(plans.data ?? [])].sort((a, b) => b.version - a.version)[0];
  const projectRepos = (repos.data ?? []).filter((r) => !c.project_id || r.project_id === c.project_id);
  return (
    <Modal title={COLUMN_LABELS[c.column]} onClose={onClose} wide>
      <div className="space-y-4">
        <Input value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} className="!text-base font-semibold" aria-label="Título" />
        <Textarea rows={6} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} aria-label="Descripción" placeholder="Qué cambiar y cómo comprobarlo" />
        <div className="grid gap-2 sm:grid-cols-3">
          <Label text="Repositorio"><Select value={draft.repo} onChange={(v) => setDraft({ ...draft, repo: v })} options={[{ value: "", label: projectRepos.length === 1 ? "El del proyecto" : "—" }, ...projectRepos.map((r) => ({ value: r.id, label: repoName(r.remote_url) }))]} /></Label>
          <Label text="Issue"><Input inputMode="numeric" value={draft.issue} onChange={(e) => setDraft({ ...draft, issue: e.target.value.replace(/\D/g, "") })} /></Label>
          <Label text="Pull request"><Input inputMode="numeric" value={draft.pr} onChange={(e) => setDraft({ ...draft, pr: e.target.value.replace(/\D/g, "") })} placeholder="Se detecta por la rama" /></Label>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {dirty && <Button onClick={() => save.mutate()} disabled={save.isPending}>Guardar</Button>}
          {c.stage !== "ready" && c.column === "inbox" && <Button variant="ghost" onClick={() => stage.mutate("ready")}>Marcar como lista</Button>}
          {c.stage === "ready" && c.column === "ready" && <Button variant="ghost" onClick={() => stage.mutate("inbox")}>Volver a inbox</Button>}
          {c.stage !== "cancelled" ? <Button variant="ghost" onClick={() => stage.mutate("cancelled")}>Cancelar tarea</Button> : <Button variant="ghost" onClick={() => stage.mutate("inbox")}>Reabrir</Button>}
          <Button variant="ghost" className="ml-auto" onClick={() => confirm("¿Archivar la tarea?") && archive.mutate()}>Archivar</Button>
        </div>
        <ErrorText error={save.error ?? stage.error ?? archive.error} />

        {c.branch && <p className="flex items-center gap-1.5 font-mono text-xs text-slate-500"><GitBranch className="size-3.5" />{c.branch}{c.pull && <a className="hover:underline" href={c.pull.url} target="_blank" rel="noreferrer">· PR #{c.pull.number} {c.pull.draft ? "(draft)" : ""}</a>}{c.pr_state && !c.pull && c.pr_number && <span>· PR #{c.pr_number} ({c.pr_state})</span>}</p>}

        <section>
          <h3 className="mb-1 text-sm font-semibold">Hilos</h3>
          {c.threads.length === 0 && <p className="text-sm text-slate-500 dark:text-muted">Ninguno todavía.</p>}
          {c.threads.map((t) => (
            <Link key={t.id} to="/threads/$threadId" params={{ threadId: t.id }} className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-slate-100 dark:hover:bg-raised">
              <StatusDot status={t.status} /><span className="min-w-0 flex-1 truncate">{t.title}</span><span className="text-xs text-slate-500">{t.provider}</span>
            </Link>
          ))}
        </section>

        <section className="rounded-lg border border-slate-200 p-3 dark:border-border">
          <h3 className="mb-2 flex items-center gap-1.5 text-sm font-semibold"><Rocket className="size-4" />Lanzar en este nodo</h3>
          {form.fields}
          <div className="mt-2 flex flex-wrap gap-2">
            <Button onClick={() => launch.mutate()} disabled={!form.ready || launch.isPending || c.blockers.length > 0} title={c.blockers.length ? "Depende de tareas sin terminar" : "Crea la rama y el worktree y lanza el agente"}>Implementar</Button>
            <Button variant="ghost" onClick={() => define.mutate()} disabled={!form.ready || define.isPending}>Definir con un agente</Button>
          </div>
          <ErrorText error={launch.error ?? define.error} />
        </section>

        <PlanEditor taskId={id} latest={latest} versions={plans.data?.length ?? 0} />

        <section>
          <h3 className="mb-1 text-sm font-semibold">Depende de</h3>
          {(deps.data ?? []).map((d) => {
            const o = others.find((x) => x.id === d.depends_on_id);
            return (
              <div key={d.depends_on_id} className="flex items-center gap-2 py-1 text-sm">
                <span className={`min-w-0 flex-1 truncate ${c.blockers.includes(d.depends_on_id) ? "" : "text-slate-400 line-through"}`}>{o?.title ?? d.depends_on_id}</span>
                <button type="button" aria-label="Quitar dependencia" onClick={() => dep.mutate({ other: d.depends_on_id, add: false })}><X className="size-4" /></button>
              </div>
            );
          })}
          <Select value="" onChange={(v) => v && dep.mutate({ other: v, add: true })} placeholder="Añadir dependencia…"
            options={others.filter((o) => !deps.data?.some((d) => d.depends_on_id === o.id)).map((o) => ({ value: o.id, label: o.title }))} />
          <ErrorText error={dep.error} />
        </section>
      </div>
    </Modal>
  );
}

function PlanEditor({ taskId, latest, versions }: { taskId: string; latest?: Plan; versions: number }) {
  const qc = useQueryClient();
  const [edit, setEdit] = useState<Record<string, string>>();
  const done = () => { qc.invalidateQueries({ queryKey: ["plans", taskId] }); qc.invalidateQueries({ queryKey: ["card", taskId] }); qc.invalidateQueries({ queryKey: ["board"] }); };
  const create = useMutation({ mutationFn: (sections: Record<string, string>) => api.createPlan(taskId, { sections, open_questions: latest?.open_questions ?? [] }), onSuccess: () => { setEdit(undefined); done(); } });
  const accept = useMutation({ mutationFn: () => api.acceptPlan(taskId), onSuccess: done });
  return (
    <section className="rounded-lg border border-slate-200 p-3 dark:border-border">
      <div className="mb-2 flex items-center gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-semibold"><ClipboardText className="size-4" />Plan</h3>
        {latest && <span className="text-xs text-slate-500">v{latest.version} de {versions} · {latest.author === "agent" ? "agente" : "tú"}{latest.accepted_at ? " · aceptado" : ""}</span>}
        <div className="ml-auto flex gap-2">
          {!edit && <Button variant="ghost" className="text-xs" onClick={() => setEdit({ ...(latest?.sections ?? {}) })}>{latest ? "Editar" : "Escribir plan"}</Button>}
          {latest && !latest.accepted_at && !edit && <Button className="text-xs" onClick={() => accept.mutate()} disabled={accept.isPending}>Aceptar plan</Button>}
        </div>
      </div>
      {edit ? (
        <div className="space-y-2">
          {SECTIONS.map(([k, label]) => <Label key={k} text={label}><Textarea rows={2} value={edit[k] ?? ""} onChange={(e) => setEdit({ ...edit, [k]: e.target.value })} /></Label>)}
          <div className="flex gap-2"><Button onClick={() => create.mutate(edit)} disabled={create.isPending}>Guardar versión</Button><Button variant="ghost" onClick={() => setEdit(undefined)}>Descartar</Button></div>
        </div>
      ) : latest ? (
        <div className="space-y-2 text-sm">
          {SECTIONS.filter(([k]) => latest.sections[k]).map(([k, label]) => <div key={k}><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</p><Md>{latest.sections[k]}</Md></div>)}
          {Object.entries(latest.sections).filter(([k]) => !SECTIONS.some(([s]) => s === k)).map(([k, v]) => <div key={k}><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{k}</p><Md>{String(v)}</Md></div>)}
          {latest.open_questions.length > 0 && <div><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">Preguntas abiertas</p><ul className="list-disc pl-5">{latest.open_questions.map((q) => <li key={q}>{q}</li>)}</ul></div>}
        </div>
      ) : <p className="text-sm text-slate-500 dark:text-muted">Sin plan. No es obligatorio: puedes lanzar la tarea sin él.</p>}
      <ErrorText error={create.error ?? accept.error} />
    </section>
  );
}
