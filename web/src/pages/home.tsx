import { useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, ClipboardText, Desktop, FileText, Folder, FolderOpen, GitBranch, GitFork, Paperclip, X } from "@phosphor-icons/react";
import { api } from "../api/client";
import { DictationMic, DictationPreview, useLocalDictation } from "../components/local-dictation";
import { ErrorText, iconButton } from "../components/ui";
import { useLaunchForm, ProviderIcon } from "../components/launch-picker";
import { modelEffort, modelId } from "../effort";
import { useProject } from "../project";
import { Chip, ModelPicker, composerArea, composerFrame, useSuggest } from "./composer";
import { useCommand, useShortcuts } from "../components/shortcuts";
import { formatChord } from "../components/dictation-shortcut";
import { planMessage } from "../lib/auto-message";


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
  const [files, setFiles] = useState<{ key: string; file: File; preview: string }[]>([]);
  const picker = useRef<HTMLInputElement>(null);
  const addFiles = (list: FileList | File[]) => setFiles((fs) => [...fs, ...Array.from(list).map((file) => {
    const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
    return { key: crypto.randomUUID(), file: named, preview: URL.createObjectURL(named) };
  })]);
  const removeFile = (k: string) => setFiles((fs) => fs.filter((f) => { if (f.key === k) URL.revokeObjectURL(f.preview); return f.key !== k; }));
  const box = useRef<HTMLTextAreaElement>(null);
  useCommand("plan", () => { if (!box.current?.getClientRects().length) return false; setPlan((value) => !value); });
  const dictation = useLocalDictation(text, setText, box, { panelShortcut: true, onEnter: (draft) => latestSendDraft.current(draft) });
  const showingDictation = !!dictation.preview;
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const form = useLaunchForm({ section: "home" , project });
  const [mode, setMode] = useState<"checkout" | "worktree" | "existing">("checkout");
  const worktree = mode === "worktree";
  const [existing, setExisting] = useState("");
  const worktreeList = useQuery({
    queryKey: ["project-worktrees", form.value.node, project], queryFn: () => api.projectWorktrees(form.value.node, project!),
    enabled: !!project && mode === "existing", retry: false,
  });
  const chosen = worktreeList.data?.find((w) => w.path === existing) ?? worktreeList.data?.[0];
  const [base, setBase] = useState("");
  const branchList = useQuery({
    queryKey: ["project-branches", form.value.node, project], queryFn: () => api.projectBranches(form.value.node, project!),
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
      const title = body ? undefined : files.length > 1 ? `${files.length} adjuntos` : files[0]?.file.name;
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
    onSuccess: (r) => { setText(""); files.forEach((f) => URL.revokeObjectURL(f.preview)); setFiles([]); onCreated(r.thread_id); },
  });
  const sendDraft = (draft: string) => {
    if ((draft.trim() || files.length) && form.ready && !start.isPending) start.mutate(draft);
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
        onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
        onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
        onPaste={(e) => {
          // Screenshots often come only as clipboard items, not in .files.
          const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
          if (fs.length) { e.preventDefault(); addFiles(fs); }
        }}>
        {!dictation.active && suggest.menu}
        {files.length > 0 && (
          <div className="flex flex-wrap gap-2 px-3 pt-3">
            {files.map((f) => (
              <div key={f.key} title={f.file.name} className="group relative size-14 shrink-0 overflow-hidden rounded-xl border border-slate-200 bg-slate-50 dark:border-border dark:bg-raised">
                <a href={f.preview} target="_blank" rel="noreferrer" aria-label={`Ver ${f.file.name}`} className="block size-14 cursor-zoom-in">
                  {f.file.type.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
                </a>
                <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.file.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
              </div>
            ))}
          </div>
        )}
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
            <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
            <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar" className={iconButton}>
              <Paperclip weight="regular" className="size-4" />
            </button>
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
          <button type="submit" className={iconButton} title="Enviar" aria-label="Enviar" disabled={(!text.trim() && !files.length) || !form.ready || start.isPending || dictation.active}>
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
    <div className="mx-auto flex h-[calc(100dvh-4rem-1px)] max-w-3xl flex-col justify-center sm:h-[calc(100dvh-3rem-2px)]">
      <NewThread centered onCreated={(id) => navigate({ to: "/threads/$threadId", params: { threadId: id } })} />
    </div>
  );
}
