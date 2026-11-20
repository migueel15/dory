import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api } from "../api/client";
import { Button, ErrorText, Modal, Select, Textarea, uuid } from "./ui";
import { useProject } from "../project";
import { DictationMic, DictationPreview, useDictationInProgress, useLocalDictation, useEnterFinishesDictation } from "./local-dictation";

// createIdea saves raw text as an inbox task. The server gives it a
// provisional title and may replace it with one written by the title model.
function createIdea(id: string, raw: string, projectId: string) {
  return api.createIdea({ id, text: raw, project_id: projectId || null });
}

// IdeaCapture is the quick way to drop ideas in the inbox (requirement RF-29): a large text box, the project and two save actions.
export function IdeaCapture({ onDone, focusOnSlash = false, autoFocus = false, projectId }: { onDone?: () => void; focusOnSlash?: boolean; autoFocus?: boolean; projectId?: string }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const current = useProject();
  const [text, setText] = useState("");
  const [project, setProject] = useState(projectId || current);
  // Follow the global filter when it changes; a manual pick only affects this entry.
  useEffect(() => { setProject(projectId || current); }, [projectId, current]);
  const box = useRef<HTMLTextAreaElement>(null);
  const dictation = useLocalDictation(text, setText, box);
  useEnterFinishesDictation(dictation, box);
  const dictating = useDictationInProgress();
  const showingDictation = !!dictation.preview;
  const picker = useRef<HTMLButtonElement>(null);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const save = useMutation({
    mutationFn: ({ id, raw }: { id: string; raw: string; define: boolean }) => createIdea(id, raw, project),
    onSuccess: (item, { define }) => {
      setText("");
      qc.invalidateQueries({ queryKey: ["board"] });
      onDone?.();
      if (define) navigate({ to: "/tasks", search: { task: item.id } as never });
    },
  });
  const submit = (define: boolean) => { if (text.trim() && !save.isPending && !dictating) save.mutate({ id: uuid(), raw: text.trim(), define }); };
  // Grow with the content.
  useLayoutEffect(() => {
    const el = box.current;
    if (el && !showingDictation) { el.style.height = "auto"; el.style.height = `${el.scrollHeight}px`; }
  }, [dictation.value, showingDictation]);
  useEffect(() => {
    if (!focusOnSlash) return;
    const onKey = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && e.target.closest("input, textarea, select, [contenteditable]");
      if (e.key === "/" && !typing) { e.preventDefault(); box.current?.focus(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [focusOnSlash]);
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); submit(e.shiftKey); }
    // Alt+P opens the project list; picking one returns to the text.
    else if (e.altKey && e.code === "KeyP") { e.preventDefault(); picker.current?.focus(); picker.current?.click(); }
  };
  return (
    <div data-dictation-panel className="space-y-3">
      {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} className="min-h-40 !px-0 !pt-0 !text-base" />}
      <Textarea bare ref={box} autoFocus={autoFocus} rows={4} value={dictation.value} readOnly={dictation.active} onChange={(e) => setText(e.target.value)} onKeyDown={onKeyDown} disabled={save.isPending} aria-label="Nueva idea"
        placeholder="Apunta una idea; queda en el inbox de tareas."
        className={showingDictation ? "sr-only" : "max-h-[60vh] min-h-40 resize-none rounded-sm !text-base focus-visible:!outline-2 focus-visible:!outline-offset-2 focus-visible:!outline-slate-400 dark:focus-visible:!outline-border-strong"} />
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-t border-slate-200 pt-3 dark:border-border">
        <div className="flex min-w-0 items-center gap-1">
          <Select plain buttonRef={picker} className="min-w-0 max-w-48 [&>button]:max-w-full" value={project} onChange={(v) => { setProject(v); box.current?.focus(); }} options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
          <Kbd>Alt P</Kbd>
          <DictationMic dictation={dictation} disabled={save.isPending} />
        </div>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
          <Button type="button" disabled={!text.trim() || save.isPending || dictating} onClick={() => submit(false)} title="Guardar en inbox">Guardar en inbox<Kbd inverse>Ctrl ↵</Kbd></Button>
          <Button type="button" variant="ghost" disabled={!text.trim() || save.isPending || dictating} onClick={() => submit(true)} title="Guardar y definir">Guardar y definir<Kbd>Ctrl ⇧ ↵</Kbd></Button>
        </div>
      </div>
      <ErrorText error={save.error ?? dictation.error} />
    </div>
  );
}

// Kbd shows a shortcut next to the control it triggers; phones have no keyboard, so it hides there.
function Kbd({ children, inverse = false }: { children: string; inverse?: boolean }) {
  const mac = /Mac|iPhone|iPad/.test(navigator.platform);
  return (
    <kbd className={`ml-1 hidden font-sans text-[0.625rem] font-normal sm:inline ${inverse ? "text-white/70" : "text-slate-400 dark:text-muted"}`}>
      {mac ? children.replace("Ctrl", "⌘").replace("Alt", "⌥") : children}
    </kbd>
  );
}

// IdeaModal shows IdeaCapture in a modal.
export function IdeaModal({ onClose, projectId }: { onClose: () => void; projectId?: string }) {
  return (
    <Modal title="Nueva idea" onClose={onClose} wide>
      <IdeaCapture autoFocus onDone={onClose} projectId={projectId} />
    </Modal>
  );
}
