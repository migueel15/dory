import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, Folder, GitBranch } from "@phosphor-icons/react";
import { api, COLUMN_LABELS, type LaunchChoice } from "../api/client";
import { DictationMic, DictationPreview, useLocalDictation } from "../components/local-dictation";
import { ErrorText, iconButton } from "../components/ui";
import { useLaunchForm, ProviderIcon } from "../components/launch-picker";
import { modelEffort, modelId } from "../effort";
import { useProject } from "../project";
import { Chip, ModelPicker, composerArea, composerFrame, runtimeModes } from "./composer";
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
  const dictation = useLocalDictation(text, setText, box, { panelShortcut: true, onEnter: (draft) => latestSendDraft.current(draft) });
  const showingDictation = !!dictation.preview;
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const form = useLaunchForm({ section: "home" , project });
  useLayoutEffect(() => { const el = box.current; if (el && !showingDictation) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [dictation.value, showingDictation]);
  const start = useMutation({
    mutationFn: async (draft: string) => {
      form.remember();
      const body = draft.trim();
      if (plan) {
        const task = await api.tasks.create({ project_id: project || null, title: body.split("\n")[0].slice(0, 80), description: body });
        return api.createThread({ ...form.value, project_id: project || null, task_id: task.id, title: `Definir: ${task.title}`, text: definePrompt(task.id, body) });
      }
      return api.createThread({ ...form.value, project_id: project || null, text: body });
    },
    onSuccess: (r) => { setText(""); onCreated(r.thread_id); },
  });
  const sendDraft = (draft: string) => {
    if (draft.trim() && form.ready && !start.isPending) start.mutate(draft);
  };
  const latestSendDraft = useRef(sendDraft);
  useLayoutEffect(() => { latestSendDraft.current = sendDraft; });
  const submit = () => { if (!dictation.active) sendDraft(text); };
  const models = form.providers.flatMap((provider) =>
    provider.models.map((model) => ({
      value: JSON.stringify([provider.id, modelId(model)]),
      label: model.name,
      modelId: modelId(model),
      icon: <ProviderIcon driver={provider.id} model={model.slug} />,
      provider: provider.id,
      model,
    })),
  );
  return (
    <div className="space-y-2">
      <form data-dictation-panel className={composerFrame} onSubmit={(e) => { e.preventDefault(); submit(); }}>
        {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} className="min-h-16" />}
        <textarea ref={box} data-dictation-disabled={start.isPending || undefined} rows={2} autoFocus value={dictation.value} readOnly={dictation.active} onChange={(e) => setText(e.target.value)} aria-label="Mensaje"
          placeholder={plan ? "Describe la tarea que quieres definir…" : "¿En qué trabajamos?"}
          onKeyDown={(e) => {
            if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
            if (dictation.active || window.matchMedia("(min-width: 640px)").matches) {
              e.preventDefault();
              if (e.repeat) return;
              if (dictation.active) {
                dictation.finishAndSubmit((draft) => latestSendDraft.current(draft));
              } else submit();
            }
          }}
          className={showingDictation ? "sr-only" : `${composerArea} min-h-16`} />
        <div className="flex items-end gap-0.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            <ModelPicker models={models} providers={form.providers} modelId={form.value.model} selected={form.value.provider ? JSON.stringify([form.value.provider, form.value.model]) : ""} effort={form.value.effort}
              effortsOf={(value) => {
                const option = models.find((m) => m.value === value);
                const efforts = option?.model.efforts;
                return option && efforts?.length ? { values: efforts, initial: modelEffort(option.provider, option.model) } : undefined;
              }}
              onChange={(value, effort) => {
                const model = models.find((m) => m.value === value);
                if (model) form.selectModel(model.provider, model.model, effort);
              }} />
            {form.nodes.length > 1 && <Chip below title="Nodo" value={form.value.node} onChange={form.setNode} options={form.nodes} />}
            <Chip below title="Proyecto" icon={<Folder className="size-4 shrink-0" />} value={project} onChange={setProject}
              options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
            <Chip below title="Permisos" value={form.value.runtime_mode} options={runtimeModes}
              onChange={(value) => form.setRuntimeMode(value as LaunchChoice["runtime_mode"])} />
            <button type="button" aria-pressed={plan} onClick={() => setPlan(!plan)}
              className="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium hover:bg-slate-100 aria-pressed:bg-slate-200 dark:hover:bg-raised dark:aria-pressed:bg-raised">
              <ClipboardText className="size-4" />Modo plan
            </button>
          </div>
          <DictationMic dictation={dictation} disabled={start.isPending} />
          <button type="submit" className={iconButton} title="Enviar" aria-label="Enviar" disabled={!text.trim() || !form.ready || start.isPending || dictation.active}>
            {start.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp className="size-4" weight="regular" />}
          </button>
        </div>
      </form>
      <ErrorText error={start.error ?? dictation.error ?? form.error ?? projects.error} />
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
