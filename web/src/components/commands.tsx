import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api } from "../api/client";
import { useProject } from "../project";
import { Button, ErrorText, Input, Label, Modal, Select } from "./ui";

type Command = { id: string; label: string; hint?: string; run: () => void };

// CommandPalette opens with Ctrl+K (Cmd+K on macOS) and runs app commands.
export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [idea, setIdea] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") { e.preventDefault(); setOpen((o) => !o); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const go = (to: string) => () => navigate({ to });
  const commands: Command[] = [
    { id: "idea", label: "Nueva idea", hint: "Al inbox", run: () => setIdea(true) },
    { id: "home", label: "Ir a Inicio", run: go("/") },
    { id: "inbox", label: "Ir a Inbox", run: go("/inbox") },
    { id: "attention", label: "Ir a Atención", run: go("/attention") },
    { id: "github", label: "Ir a GitHub", run: go("/github") },
    { id: "projects", label: "Ir a Proyectos", run: go("/projects") },
    { id: "memories", label: "Ir a Memorias", run: go("/memories") },
    { id: "environments", label: "Ir a Entornos", run: go("/environments") },
    { id: "usage", label: "Ir a Uso", run: go("/usage") },
  ];
  return (
    <>
      {open && <Palette commands={commands} onClose={() => setOpen(false)} />}
      {idea && <IdeaForm onClose={() => setIdea(false)} />}
    </>
  );
}

function Palette({ commands, onClose }: { commands: Command[]; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const [hi, setHi] = useState(0);
  const list = useRef<HTMLDivElement>(null);
  const q = query.trim().toLowerCase();
  const shown = commands.filter((c) => !q || c.label.toLowerCase().includes(q));
  useEffect(() => { setHi(0); }, [q]);
  useEffect(() => { list.current?.querySelector<HTMLElement>("[aria-selected=true]")?.scrollIntoView({ block: "nearest" }); }, [hi]);
  const run = (c?: Command) => { if (!c) return; onClose(); c.run(); };
  const key = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setHi((i) => Math.min(shown.length - 1, i + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setHi((i) => Math.max(0, i - 1)); }
    else if (e.key === "Enter") { e.preventDefault(); run(shown[hi]); }
    else if (e.key === "Escape") onClose();
  };
  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/40 p-2 pt-[15vh] sm:p-4 sm:pt-[15vh]" onClick={onClose}>
      <div className="w-full max-w-lg overflow-hidden rounded-xl border border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900" onClick={(e) => e.stopPropagation()}>
        <input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={key} placeholder="Escribe un comando…" aria-label="Buscar comando"
          className="w-full border-b border-slate-200 bg-transparent px-4 py-3 text-sm outline-none dark:border-slate-700" />
        <div ref={list} role="listbox" className="max-h-80 overflow-auto p-1">
          {shown.length === 0 && <p className="px-3 py-2 text-sm text-slate-400">Sin comandos.</p>}
          {shown.map((c, i) => (
            <button key={c.id} type="button" role="option" aria-selected={i === hi} onMouseEnter={() => setHi(i)} onClick={() => run(c)}
              className={`flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm ${i === hi ? "bg-slate-100 dark:bg-slate-800" : ""}`}>
              <span>{c.label}</span>
              {c.hint && <span className="text-xs text-slate-400">{c.hint}</span>}
            </button>
          ))}
        </div>
        <div className="flex gap-3 border-t border-slate-200 px-3 py-1.5 text-[11px] text-slate-400 dark:border-slate-700">
          <span>↑↓ moverse</span><span>Enter ejecutar</span><span>Esc cerrar</span>
        </div>
      </div>
    </div>
  );
}

// IdeaForm saves a new idea to the inbox, linked to the selected project by default.
function IdeaForm({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const current = useProject();
  const [text, setText] = useState("");
  const [project, setProject] = useState(current);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const save = useMutation({
    mutationFn: (raw: string) => api.items.create({ title: raw.split("\n")[0].slice(0, 80), raw_text: raw, status: "inbox", tags: [], project_id: project || null }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["items"] }); onClose(); },
  });
  const submit = () => { if (text.trim() && !save.isPending) save.mutate(text.trim()); };
  return (
    <Modal title="Nueva idea" onClose={onClose}>
      <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); submit(); }} onKeyDown={(e) => e.key === "Escape" && onClose()}>
        <Label text="Idea"><Input autoFocus value={text} onChange={(e) => setText(e.target.value)} placeholder="¿Qué se te ha ocurrido?" /></Label>
        <Label text="Proyecto">
          <Select value={project} onChange={setProject} options={[{ value: "", label: "— ninguno —" }, ...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))]} />
        </Label>
        <ErrorText error={save.error} />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button type="submit" disabled={!text.trim() || save.isPending}>Guardar en inbox</Button>
        </div>
      </form>
    </Modal>
  );
}
