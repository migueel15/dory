import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, Desktop, Folder, FolderOpen, GitBranch, GitFork } from "@phosphor-icons/react";
import { api } from "../api/client";
import { DictationMic, DictationPreview, useLocalDictation } from "../features/composer/local-dictation";
import { ErrorText, iconButton } from "../components/ui";
import { useLaunchForm, ProviderIcon } from "../features/composer/launch-picker";
import { modelEffort, modelId } from "../effort";
import { useProject } from "../project";
import { Chip, ModelPicker, composerArea, composerFrame, useSuggest } from "../features/composer/composer";
import { useCommand, useShortcuts } from "../components/shortcuts";
import { formatChord } from "../features/composer/dictation-shortcut";
import { useComposerFiles } from "../features/composer/composer-files";
import { planMessage } from "../lib/auto-message";
import { qk } from "../api/queries/keys";
import { useProjects } from "../api/queries/hooks";


// NewThread is the chat box of the start screen (requirement 13.2): the
// title names the project, the box holds the model, and below it go the
// environment (node) and the project the thread works in. With the plan
// toggle on, the chat defines a task instead.
export function NewThread({ onCreated, centered = false }: { onCreated: (threadId: string) => void; centered?: boolean }) {
  const current = useProject();
  const getShortcut = useShortcuts();
  const [project, setProject] = useState(current);
  const [text, setText] = useState("");
  const [plan, setPlan] = useState(false);
  // Files wait in the box until the thread exists: then they are uploaded to
  // it and go out with the first message.
  const attach = useComposerFiles();
  const files = attach.files;
  const box = useRef<HTMLTextAreaElement>(null);
  useCommand("plan", () => { if (!box.current?.getClientRects().length) return false; setPlan((value) => !value); });
  const dictation = useLocalDictation(text, setText, box, { panelShortcut: true, onEnter: (draft) => latestSendDraft.current(draft) });
  const showingDictation = !!dictation.preview;
  const projects = useProjects();
  const form = useLaunchForm({ section: "home" , project });
  const [pickedMode, setMode] = useState<"checkout" | "worktree" | "existing">();
  const mode = pickedMode ?? form.defaultWorkspace;
  const worktree = mode === "worktree";
  const [existing, setExisting] = useState("");
  const worktreeList = useQuery({
    queryKey: qk.projectWorktrees(form.value.node, project), queryFn: () => api.projectWorktrees(form.value.node, project!),
    enabled: !!project && mode === "existing", retry: false,
  });
  const chosen = worktreeList.data?.find((w) => w.path === existing) ?? worktreeList.data?.[0];
  const [base, setBase] = useState("");
  const branchList = useQuery({
    queryKey: qk.projectBranches(form.value.node, project), queryFn: () => api.projectBranches(form.value.node, project!),
    enabled: !!project, retry: false,
  });
  const checkoutBranch = branchList.data?.current ?? "";
  const branch = base && branchList.data?.branches.includes(base) ? base : checkoutBranch;
  // workspace is where the first message starts the thread: the checkout,
  // switched to branch, or a new worktree with its own branch made from it.
  const workspace = !project ? {} : mode === "existing" ? (chosen ? { worktree_path: chosen.path } : {}) : !branch ? {} : worktree ? { worktree: true, base_branch: branch } : { branch };
  useLayoutEffect(() => { const el = box.current; if (el && !showingDictation) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [dictation.value, showingDictation]);
  const start = useMutation({
    mutationFn: async (draft: string) => {
      form.remember();
      const body = draft.trim();
      const title = body ? undefined : files.length > 1 ? `${files.length} adjuntos` : files[0]?.name;
      let thread: Parameters<typeof api.createThread>[0] = { ...form.value, ...workspace, project_id: project || null, title, text: body };
      if (plan) {
        const task = await api.tasks.create({ project_id: project || null, title: body.split("\n")[0].slice(0, 80) || title || "Nueva tarea", description: body });
        thread = { ...thread, task_id: task.id, title: `Definir: ${task.title}`, text: planMessage({ id: task.id, title: task.title }, body) };
      }
      if (!files.length) return api.createThread(thread);
      // With files the thread starts empty, takes the uploads and then the message.
      const r = await api.createThread({ ...thread, text: "", title: thread.title ?? body.split("\n")[0].slice(0, 80) });
      const uploaded = await Promise.all(files.map((f) => api.upload(r.thread_id, f.file)));
      await api.send(r.thread_id, thread.text ?? "", uploaded.map((u) => u.id));
      return r;
    },
    onSuccess: (r) => { setText(""); attach.clear(); onCreated(r.thread_id); },
  });
  const sendDraft = (draft: string) => {
    if ((draft.trim() || files.length) && form.ready && !start.isPending) start.mutate(draft);
  };
  const latestSendDraft = useRef(sendDraft);
  useLayoutEffect(() => { latestSendDraft.current = sendDraft; });
  const submit = () => {
    if (dictation.active) dictation.finishAndSubmit((draft) => latestSendDraft.current(draft));
    else sendDraft(text);
  };
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
  const provider = form.value.provider ?? "";
  const suggest = useSuggest({ key: ["new", form.value.node, provider, project], commands: () => api.nodeCommands(form.value.node, provider, project || undefined) }, text, box, setText);
  const projectName = (projects.data ?? []).find((p) => p.id === project)?.name;
  return (
    <div className={centered ? "relative" : "space-y-2"}>
      <div className={`mb-6 space-y-2 text-center ${centered ? "absolute inset-x-0 bottom-full" : ""}`}>
        <h1 className="text-3xl font-light tracking-tight">
          {projectName ? <>¿Qué construimos en <span className="font-semibold">{projectName}</span>?</> : "¿Qué hacemos hoy?"}
        </h1>
        {projectName && (
          <button type="button" onClick={() => setProject("")} className="text-sm font-medium text-slate-500 hover:text-slate-800 dark:text-muted dark:hover:text-slate-200">
            o empieza sin proyecto
          </button>
        )}
      </div>
      <form data-dictation-panel data-unsaved-draft={!!text.trim() || files.length > 0 || dictation.active} className={composerFrame} onSubmit={(e) => { e.preventDefault(); submit(); }}
        {...attach.formProps}>
        {!dictation.active && suggest.menu}
        {attach.tray}
        {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} className="min-h-16" />}
        <textarea ref={box} data-dictation-disabled={start.isPending || undefined} rows={2} autoFocus value={dictation.value} readOnly={dictation.active} onChange={(e) => setText(e.target.value)} onSelect={() => { if (!dictation.active) suggest.track(); }} aria-label="Mensaje"
          placeholder={plan ? "Describe la tarea que quieres definir…" : "¿En qué trabajamos?"}
          onKeyDown={(e) => {
            if (!dictation.active && suggest.onKeyDown(e)) return;
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
            {attach.attachButton}
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
            <button type="button" title={`Modo plan (${formatChord(getShortcut("plan"))})`} aria-pressed={plan} onClick={() => setPlan(!plan)}
              className="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium hover:bg-slate-100 aria-pressed:bg-slate-200 dark:hover:bg-raised dark:aria-pressed:bg-raised">
              <ClipboardText className="size-4" />Modo plan
            </button>
          </div>
          <DictationMic dictation={dictation} disabled={start.isPending} />
          <button type="submit" className={iconButton} title="Enviar" aria-label="Enviar" disabled={(!dictation.value.trim() && !files.length) || !form.ready || start.isPending}>
            {start.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp className="size-4" weight="regular" />}
          </button>
        </div>
      </form>
      <div className={centered ? "absolute inset-x-0 top-full space-y-2 pt-2" : "space-y-2"}>
      <div className="flex flex-wrap items-center gap-1 px-2 text-xs text-slate-500 dark:text-muted">
        <Chip below title="Entorno" icon={<Desktop className="size-4 shrink-0" />} value={form.value.node} onChange={form.setNode} options={form.nodes} />
        <span aria-hidden="true" className="h-4 w-px bg-slate-200 dark:bg-border" />
        <Chip below title="Proyecto" icon={<Folder className="size-4 shrink-0" />} value={project} onChange={setProject}
          options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
        {project && (
          <>
            <Chip below title="Espacio de trabajo" icon={mode !== "checkout" ? <GitFork className="size-4 shrink-0" /> : <FolderOpen className="size-4 shrink-0" />}
              value={mode} onChange={(v) => setMode(v as typeof mode)}
              options={[{ value: "checkout", label: "Checkout actual" }, { value: "worktree", label: "Nuevo worktree" }, { value: "existing", label: "Worktree existente" }]} />
            <span className="ml-auto" />
            {mode === "existing" && (worktreeList.data?.length ? (
              <Chip below title="Worktree" icon={<GitBranch className="size-4 shrink-0" />} value={chosen?.path ?? ""} onChange={setExisting}
                options={worktreeList.data.map((w) => ({ value: w.path, label: `${w.branch || "(sin rama)"} · ${w.path.split("/").pop()}${w.working ? " · en marcha" : w.threads ? ` · ${w.threads} hilo${w.threads > 1 ? "s" : ""}` : ""}` }))} />
            ) : worktreeList.isSuccess && <span>No hay worktrees</span>)}
            {mode !== "existing" && !!branchList.data?.branches.length && (
              <>
                <Chip below shortcutBranch prefix={worktree ? "Desde" : undefined} title={worktree ? "Rama de la que sale el worktree" : "Rama del checkout"} icon={<GitBranch className="size-4 shrink-0" />} value={branch} onChange={setBase}
                  options={branchList.data.branches.map((b) => ({ value: b, label: b }))} />
              </>
            )}
          </>
        )}
      </div>
      <ErrorText error={start.error ?? dictation.error ?? form.error ?? projects.error ?? branchList.error} />
      </div>
    </div>
  );
}

// Home is the start screen: only the chat box, centred on the page.
export function Home() {
  const navigate = useNavigate();
  return (
    <div className="mx-auto flex h-[calc(100dvh_-_4rem_-_1px_-_var(--tabbar))] max-w-3xl flex-col justify-center sm:h-[calc(100dvh-3rem-2px)]">
      <NewThread centered onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
    </div>
  );
}
