import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, GitBranch } from "@phosphor-icons/react";
import { api, COLUMN_LABELS } from "../api/client";
import { Button, ErrorText, Select, Textarea } from "../components/ui";
import { useLaunchForm, ProviderIcon } from "../components/launch-picker";
import { useProject } from "../project";
import { StatusDot } from "./thread";

// definePrompt turns a chat into the definition of a task (requirement 13.3).
const definePrompt = (taskId: string, text: string) =>
  `Modo plan. Vamos a definir la tarea de Dory ${taskId} antes de implementarla. No cambies código.\n` +
  `Haz las preguntas que necesites y propón el plan con la herramienta propose_plan del MCP de Dory (objetivo, alcance, dónde, diseño, requisitos, validación, riesgos). ` +
  `Solo yo acepto el plan.\n\n${text}`;

// NewThread is the chat box of the start screen (requirement 13.2): pick the
// project and the model, then write. With the plan toggle on, the chat defines
// a task instead.
export function NewThread({ onCreated }: { onCreated: (threadId: string) => void }) {
  const current = useProject();
  const [project, setProject] = useState(current);
  const [text, setText] = useState("");
  const [plan, setPlan] = useState(false);
  const box = useRef<HTMLTextAreaElement>(null);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const form = useLaunchForm({ section: "home" });
  useLayoutEffect(() => { const el = box.current; if (el) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 400)}px`; } }, [text]);
  const start = useMutation({
    mutationFn: async () => {
      form.remember();
      const body = text.trim();
      if (plan) {
        const task = await api.tasks.create({ project_id: project || null, title: body.split("\n")[0].slice(0, 80), description: body });
        return api.createThread({ ...form.value, project_id: project || null, task_id: task.id, title: `Definir: ${task.title}`, text: definePrompt(task.id, body) });
      }
      return api.createThread({ ...form.value, project_id: project || null, text: body });
    },
    onSuccess: (r) => { setText(""); onCreated(r.thread_id); },
  });
  const submit = () => { if (text.trim() && form.ready && !start.isPending) start.mutate(); };
  return (
    <div className="space-y-2">
      <div className="rounded-2xl border border-slate-300 bg-white p-3 shadow-sm focus-within:border-slate-500 dark:border-border dark:bg-surface">
        <Textarea bare ref={box} rows={3} autoFocus value={text} onChange={(e) => setText(e.target.value)} aria-label="Mensaje"
          placeholder={plan ? "Describe la tarea que quieres definir…" : "¿En qué trabajamos?"}
          onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); } }}
          className="max-h-[400px] min-h-16 w-full resize-none" />
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Select plain value={project} onChange={setProject} options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
          <label className="flex cursor-pointer items-center gap-1.5 rounded-md px-2 py-1 text-sm hover:bg-slate-100 dark:hover:bg-raised" title="Convierte la conversación en la definición de una tarea">
            <input type="checkbox" checked={plan} onChange={(e) => setPlan(e.target.checked)} />
            <ClipboardText className="size-4" />Modo plan
          </label>
          <Button className="ml-auto" title="Enviar" onClick={submit} disabled={!text.trim() || !form.ready || start.isPending}><ArrowUp className="size-4" weight="bold" /></Button>
        </div>
      </div>
      <details className="rounded-lg border border-slate-200 px-3 py-2 text-sm dark:border-border">
        <summary className="cursor-pointer text-slate-600 dark:text-muted">Agente: {form.value.provider || "—"} {form.value.model && `· ${form.value.model}`} {form.value.effort && `· ${form.value.effort}`}</summary>
        <div className="mt-2">{form.fields}</div>
      </details>
      <ErrorText error={start.error ?? form.error} />
    </div>
  );
}

// Home is the start screen: a chat box and, below it, what moved since you
// last looked.
export function Home() {
  const navigate = useNavigate();
  const project = useProject();
  const threads = useQuery({ queryKey: ["threads", "recent"], queryFn: () => api.threads() });
  const board = useQuery({ queryKey: ["board", project], queryFn: () => api.board(project || undefined) });
  const recent = (threads.data ?? []).filter((t) => !project || t.project_id === project)
    .sort((a, b) => (b.last_activity_at ?? b.created_at).localeCompare(a.last_activity_at ?? a.created_at)).slice(0, 8);
  const attention = (board.data?.cards ?? []).filter((c) => c.column === "needs_you" || c.column === "ready_to_merge");
  return (
    <div className="mx-auto max-w-3xl space-y-6 pt-6 sm:pt-16">
      <h1 className="text-center text-2xl font-semibold">¿Qué hacemos hoy?</h1>
      <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
      {attention.length > 0 && (
        <section>
          <h2 className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-400">Te esperan</h2>
          <div className="divide-y divide-slate-200 rounded-lg border border-slate-200 dark:divide-border dark:border-border">
            {attention.map((c) => (
              <Link key={c.id} to="/tasks" search={{ task: c.id } as never} className="flex items-center gap-2 px-3 py-2 text-sm hover:bg-slate-50 dark:hover:bg-raised">
                <span className="min-w-0 flex-1 truncate">{c.title}</span>
                <span className="text-xs text-slate-500">{COLUMN_LABELS[c.column]}</span>
              </Link>
            ))}
          </div>
        </section>
      )}
      {recent.length > 0 && (
        <section>
          <h2 className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-400">Hilos recientes</h2>
          <div className="divide-y divide-slate-200 rounded-lg border border-slate-200 dark:divide-border dark:border-border">
            {recent.map((t) => (
              <Link key={t.id} to="/threads/$threadId" params={{ threadId: t.id }} className="flex items-center gap-2 px-3 py-2 text-sm hover:bg-slate-50 dark:hover:bg-raised">
                <ProviderIcon driver={t.provider} model={t.model} />
                <span className="min-w-0 flex-1 truncate">{t.title}</span>
                {t.branch && <span className="hidden items-center gap-1 font-mono text-xs text-slate-500 sm:flex"><GitBranch className="size-3" />{t.branch}</span>}
                <StatusDot status={t.status} />
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
