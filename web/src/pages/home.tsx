import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, Desktop, Folder, FolderOpen, GitBranch, GitFork } from "@phosphor-icons/react";
import { api } from "../api/client";
import { DictationMic, DictationPreview, useLocalDictation } from "../components/local-dictation";
import { ErrorText, iconButton } from "../components/ui";
import { useLaunchForm, ProviderIcon } from "../components/launch-picker";
import { modelEffort, modelId } from "../effort";
import { useProject } from "../project";
import { Chip, ModelPicker, composerArea, composerFrame } from "./composer";

// definePrompt turns a chat into the definition of a task (requirement 13.3).
const definePrompt = (taskId: string, text: string) =>
  `Modo plan. Vamos a definir la tarea de Dory ${taskId} antes de implementarla. No cambies código.\n` +
  `Haz las preguntas que necesites y propón el plan con la herramienta propose_plan del MCP de Dory (objetivo, alcance, dónde, diseño, requisitos, validación, riesgos). ` +
  `Solo yo acepto el plan.\n\n${text}`;

// NewThread is the chat box of the start screen (requirement 13.2): the
// title names the project, the box holds the model, and below it go the
// environment (node) and the project the thread works in. With the plan
// toggle on, the chat defines a task instead.
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
  const [worktree, setWorktree] = useState(false);
  const [base, setBase] = useState("");
  const branchList = useQuery({
    queryKey: ["project-branches", form.value.node, project], queryFn: () => api.projectBranches(form.value.node, project!),
    enabled: !!project, retry: false,
  });
  const checkoutBranch = branchList.data?.current ?? "";
  const branch = base && branchList.data?.branches.includes(base) ? base : checkoutBranch;
  // workspace is where the first message starts the thread: the checkout,
  // switched to branch, or a new worktree with its own branch made from it.
  const workspace = !project || !branch ? {} : worktree ? { worktree: true, base_branch: branch } : { branch };
  useLayoutEffect(() => { const el = box.current; if (el && !showingDictation) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [dictation.value, showingDictation]);
  const start = useMutation({
    mutationFn: async (draft: string) => {
      form.remember();
      const body = draft.trim();
      if (plan) {
        const task = await api.tasks.create({ project_id: project || null, title: body.split("\n")[0].slice(0, 80), description: body });
        return api.createThread({ ...form.value, ...workspace, project_id: project || null, task_id: task.id, title: `Definir: ${task.title}`, text: definePrompt(task.id, body) });
      }
      return api.createThread({ ...form.value, ...workspace, project_id: project || null, text: body });
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
  const projectName = (projects.data ?? []).find((p) => p.id === project)?.name;
  return (
    <div className="space-y-2">
      <div className="mb-6 space-y-2 text-center">
        <h1 className="text-3xl font-light tracking-tight">
          {projectName ? <>¿Qué construimos en <span className="font-semibold">{projectName}</span>?</> : "¿Qué hacemos hoy?"}
        </h1>
        {projectName && (
          <button type="button" onClick={() => setProject("")} className="text-sm font-medium text-slate-500 hover:text-slate-800 dark:text-muted dark:hover:text-slate-200">
            o empieza sin proyecto
          </button>
        )}
      </div>
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
      <div className="flex flex-wrap items-center gap-1 px-2 text-xs text-slate-500 dark:text-muted">
        <Chip below title="Entorno" icon={<Desktop className="size-4 shrink-0" />} value={form.value.node} onChange={form.setNode} options={form.nodes} />
        <span aria-hidden="true" className="h-4 w-px bg-slate-200 dark:bg-border" />
        <Chip below title="Proyecto" icon={<Folder className="size-4 shrink-0" />} value={project} onChange={setProject}
          options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
        {project && (
          <>
            <Chip below title="Espacio de trabajo" icon={worktree ? <GitFork className="size-4 shrink-0" /> : <FolderOpen className="size-4 shrink-0" />}
              value={worktree ? "worktree" : "checkout"} onChange={(v) => setWorktree(v === "worktree")}
              options={[{ value: "checkout", label: "Checkout actual" }, { value: "worktree", label: "Nuevo worktree" }]} />
            <span className="ml-auto" />
            {!!branchList.data?.branches.length && (
              <>
                <Chip below prefix={worktree ? "Desde" : undefined} title={worktree ? "Rama de la que sale el worktree" : "Rama del checkout"} icon={<GitBranch className="size-4 shrink-0" />} value={branch} onChange={setBase}
                  options={branchList.data.branches.map((b) => ({ value: b, label: b }))} />
              </>
            )}
          </>
        )}
      </div>
      <ErrorText error={start.error ?? dictation.error ?? form.error ?? projects.error ?? branchList.error} />
    </div>
  );
}

// Home is the start screen: only the chat box, centred on the page.
export function Home() {
  const navigate = useNavigate();
  return (
    <div className="mx-auto flex min-h-[70vh] max-w-3xl flex-col justify-center">
      <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
    </div>
  );
}
