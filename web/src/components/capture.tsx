import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api } from "../api/client";
import { Button, ErrorText, Modal, Select, uuid } from "./ui";
import { useProject } from "../project";

// createIdea saves raw text as an inbox item; the definition agent fills in the rest.
function createIdea(raw: string, projectId: string) {
  const title = (raw.split("\n").find((l) => l.trim()) ?? raw).trim().slice(0, 80);
  // The title is provisional until the server replaces it with a generated one (title_source pending).
  return api.items.create({ id: uuid(), title, raw_text: raw, status: "inbox", source: "web", title_source: "pending", tags: [], project_id: projectId || null });
}

// IdeaCapture is the only way to create items: a large text box, the project and two save actions.
export function IdeaCapture({ onDone, focusOnSlash = false, autoFocus = false, projectId }: { onDone?: () => void; focusOnSlash?: boolean; autoFocus?: boolean; projectId?: string }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const current = useProject();
  const [text, setText] = useState("");
  const [project, setProject] = useState(projectId || current);
  const box = useRef<HTMLTextAreaElement>(null);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const save = useMutation({
    mutationFn: ({ raw }: { raw: string; define: boolean }) => createIdea(raw, project),
    onSuccess: (item, { define }) => {
      setText("");
      qc.invalidateQueries({ queryKey: ["items"] });
      onDone?.();
      if (define) navigate({ to: "/items/$id/define", params: { id: item.id } });
    },
  });
  const submit = (define: boolean) => { if (text.trim() && !save.isPending) save.mutate({ raw: text.trim(), define }); };
  // Grow with the content.
  useLayoutEffect(() => {
    const el = box.current;
    if (el) { el.style.height = "auto"; el.style.height = `${el.scrollHeight}px`; }
  }, [text]);
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
  };
  return (
    <div className="space-y-2 rounded-lg border border-slate-300 bg-white p-3 focus-within:border-slate-500 dark:border-border dark:bg-surface dark:focus-within:border-border-strong">
      <textarea ref={box} autoFocus={autoFocus} rows={4} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={onKeyDown} disabled={save.isPending} aria-label="Nueva idea"
        placeholder="¿Qué quieres hacer? Cuéntalo a grandes rasgos; el agente se encarga del resto."
        className="max-h-[60vh] min-h-24 w-full resize-none bg-transparent text-sm outline-none" />
      <div className="flex flex-wrap items-center gap-2">
        <Select plain className="min-w-36" value={project} onChange={setProject} options={[{ value: "", label: "Sin proyecto" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
        <span className="hidden flex-1 text-[11px] text-slate-400 sm:inline">Enter nueva línea · Ctrl+Enter inbox · Ctrl+Shift+Enter definir</span>
        <div className="ml-auto flex gap-2">
          <Button type="button" variant="ghost" disabled={!text.trim() || save.isPending} onClick={() => submit(false)}>Guardar en inbox</Button>
          <Button type="button" disabled={!text.trim() || save.isPending} onClick={() => submit(true)}>Guardar y definir</Button>
        </div>
      </div>
      <ErrorText error={save.error} />
    </div>
  );
}

// IdeaModal shows IdeaCapture in a modal.
export function IdeaModal({ onClose, projectId }: { onClose: () => void; projectId?: string }) {
  return (
    <Modal title="Nueva idea" onClose={onClose} wide>
      <h2 className="font-semibold">Nueva idea</h2>
      <IdeaCapture autoFocus onDone={onClose} projectId={projectId} />
    </Modal>
  );
}
