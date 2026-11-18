import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, Folder } from "@phosphor-icons/react";
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

// Home is the start screen: only the chat box, centred on the page.
export function Home() {
  const navigate = useNavigate();
  return (
    <div className="mx-auto flex min-h-[70vh] max-w-3xl flex-col justify-center space-y-6">
      <h1 className="text-center text-2xl font-semibold">¿Qué hacemos hoy?</h1>
      <NewThread onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
    </div>
  );
}
