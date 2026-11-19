import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, PencilSimple } from "@phosphor-icons/react";
import { api, ApiError, type Plan } from "../api/client";
import { Button, ErrorText, Md } from "./ui";

// PlanDoc shows the plan of a task: one Markdown document that the user and
// the agents edit. It follows the agent's edits live (the plan query refreshes
// on every replicated change) and briefly marks the document when it changes.
export function PlanDoc({ taskId }: { taskId: string }) {
  const qc = useQueryClient();
  const key = ["plans", taskId, "doc"];
  const plan = useQuery({ queryKey: key, queryFn: () => api.plan(taskId) });
  const p = plan.data;
  // edit holds the text being written and the revision it started from.
  const [edit, setEdit] = useState<{ text: string; base: number }>();
  const changed = useFlash(p?.revision);
  const done = (next: Plan) => {
    qc.setQueryData(key, next);
    qc.invalidateQueries({ queryKey: ["card", taskId] });
    qc.invalidateQueries({ queryKey: ["board"] });
  };
  const save = useMutation({
    mutationFn: (v: { text: string; base: number }) => api.savePlan(taskId, { body_md: v.text, revision: v.base }),
    onSuccess: (next) => { setEdit(undefined); done(next); },
  });
  const accept = useMutation({ mutationFn: () => api.acceptPlan(taskId), onSuccess: (r) => done({ ...r.plan, body_md: p?.body_md ?? r.plan.body_md }) });
  // Ticking a checkbox saves at once, against the revision on screen.
  const toggle = useMutation({
    mutationFn: (line: number) => {
      const lines = (p?.body_md ?? "").split("\n");
      lines[line - 1] = lines[line - 1].replace(/\[( |x|X)\]/, (m) => (m === "[ ]" ? "[x]" : "[ ]"));
      return api.savePlan(taskId, { body_md: lines.join("\n"), revision: p?.revision ?? 0 });
    },
    onSuccess: done,
  });
  const conflict = [save.error, toggle.error].some((e) => e instanceof ApiError && e.status === 409);
  // While editing, the agent may still change the plan underneath.
  const moved = !!edit && !!p && p.revision !== edit.base;

  if (plan.error) return <div className="p-4"><ErrorText error={plan.error} /></div>;
  if (!p) return null;
  const empty = !p.body_md.trim();
  return (
    <div className="flex min-h-full flex-col">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-slate-200 bg-slate-50/90 px-4 py-2 text-xs text-slate-500 backdrop-blur dark:border-border dark:bg-canvas/90 dark:text-muted">
        <span>{p.revision > 0 ? `Revisión ${p.revision} · ${p.author === "agent" ? "agente" : "tú"}` : "Sin plan"}</span>
        {p.accepted_at && <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400"><Check className="size-3.5" />Aceptado</span>}
        <span className="flex-1" />
        {!edit && <Button variant="ghost" className="!px-2 text-xs" onClick={() => setEdit({ text: p.body_md, base: p.revision })}><PencilSimple className="size-3.5" />{empty ? "Escribir" : "Editar"}</Button>}
        {!edit && !empty && !p.accepted_at && <Button className="text-xs" onClick={() => accept.mutate()} disabled={accept.isPending}>Aceptar plan</Button>}
      </div>
      {edit ? (
        <form className="flex flex-1 flex-col gap-2 p-4" onSubmit={(e) => { e.preventDefault(); save.mutate(edit); }}>
          {(moved || conflict) && (
            <p className="flex flex-wrap items-center gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
              El plan ha cambiado mientras editabas.
              <button type="button" className="underline" onClick={() => { save.reset(); setEdit({ text: p.body_md, base: p.revision }); }}>Descartar lo mío y cargar el actual</button>
            </p>
          )}
          <PlanTextarea value={edit.text} onChange={(text) => setEdit({ ...edit, text })} onSubmit={() => save.mutate(edit)} />
          <div className="flex gap-2">
            <Button type="submit" disabled={save.isPending}>Guardar</Button>
            <Button type="button" variant="ghost" onClick={() => { save.reset(); setEdit(undefined); }}>Descartar</Button>
          </div>
          {!conflict && <ErrorText error={save.error} />}
        </form>
      ) : empty ? (
        <p className="p-6 text-center text-sm text-slate-500 dark:text-muted">Todavía no hay plan. Pide al agente que lo escriba o escríbelo tú.</p>
      ) : (
        <div className={`m-2 rounded-lg px-3 py-2 transition-colors duration-700 ${changed ? "bg-accent/10" : "bg-transparent"}`}>
          <Md onToggle={(line) => toggle.mutate(line)}>{p.body_md}</Md>
          {conflict ? <p className="mt-2 text-xs text-amber-700 dark:text-amber-300">El plan cambió antes de guardar la casilla; vuelve a marcarla.</p> : <ErrorText error={toggle.error ?? accept.error} />}
        </div>
      )}
    </div>
  );
}

// useFlash is true for a moment each time the revision moves past the first one seen.
function useFlash(revision?: number) {
  const seen = useRef(revision);
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (revision === undefined) return;
    if (seen.current === undefined || seen.current === revision) { seen.current = revision; return; }
    seen.current = revision;
    setOn(true);
    const t = setTimeout(() => setOn(false), 1200);
    return () => clearTimeout(t);
  }, [revision]);
  return on;
}

function PlanTextarea({ value, onChange, onSubmit }: { value: string; onChange: (v: string) => void; onSubmit: () => void }) {
  const area = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [value]);
  return (
    <textarea ref={area} autoFocus value={value} onChange={(e) => onChange(e.target.value)} aria-label="Plan en Markdown"
      onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); onSubmit(); } }}
      placeholder={"## Objetivo\n\n…"}
      className="min-h-64 w-full resize-none rounded-lg border border-slate-200 bg-white p-3 font-mono text-sm leading-6 dark:border-border dark:bg-surface" />
  );
}
