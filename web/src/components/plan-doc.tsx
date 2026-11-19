import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, Check, PencilSimple } from "@phosphor-icons/react";
import { api, ApiError, type Plan } from "../api/client";
import { Button, ErrorText, Md } from "./ui";

// PlanDoc shows the plan of a task: one Markdown document that the user and
// the agents edit. It follows the agent's edits live (the plan query refreshes
// on every replicated change): each change scrolls to the block that changed
// and marks it, until the user scrolls away on their own.
export function PlanDoc({ taskId }: { taskId: string }) {
  const qc = useQueryClient();
  const key = ["plans", taskId, "doc"];
  const plan = useQuery({ queryKey: key, queryFn: () => api.plan(taskId) });
  const p = plan.data;
  // edit holds the text being written and the revision it started from.
  const [edit, setEdit] = useState<{ text: string; base: number }>();
  const doc = useRef<HTMLDivElement>(null);
  const follow = useFollowChanges(p?.body_md, doc, !!edit);
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
    <div className="flex min-h-full flex-col" onWheel={follow.pause} onTouchMove={follow.pause}>
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
        <div ref={doc} className="px-5 py-3">
          <Md onToggle={(line) => toggle.mutate(line)} rehypePlugins={[sourceLines]}>{follow.text}</Md>
          {conflict ? <p className="mt-2 text-xs text-amber-700 dark:text-amber-300">El plan cambió antes de guardar la casilla; vuelve a marcarla.</p> : <ErrorText error={toggle.error ?? accept.error} />}
        </div>
      )}
      {follow.paused && (
        <button type="button" onClick={follow.resume}
          className="sticky bottom-3 mx-auto mb-3 flex items-center gap-1.5 rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs shadow-md hover:bg-slate-50 dark:border-border-strong dark:bg-surface dark:hover:bg-raised">
          <ArrowDown className="size-3.5" />Seguir al agente
        </button>
      )}
    </div>
  );
}

type HastNode = { type: string; tagName?: string; properties?: Record<string, unknown>; position?: { start: { line: number }; end: { line: number } }; children?: HastNode[] };
const BLOCKS = new Set(["h1", "h2", "h3", "h4", "h5", "h6", "p", "li", "pre", "blockquote", "table", "tr", "hr"]);

// sourceLines marks each rendered block with the Markdown lines it comes
// from, so a change in the text can be found on screen.
function sourceLines() {
  const walk = (n: HastNode) => {
    if (n.type === "element" && n.tagName && BLOCKS.has(n.tagName) && n.position) {
      n.properties = { ...n.properties, "data-ls": n.position.start.line, "data-le": n.position.end.line };
    }
    n.children?.forEach(walk);
  };
  return (tree: HastNode) => walk(tree);
}

// changedLines gives the lines (1-based, inclusive) that differ between prev
// and next, on each side; undefined when nothing changed. A pure insertion or
// deletion points at the line after it on the side that has no lines.
function changedLines(prev: string, next: string): { before: [number, number]; after: [number, number] } | undefined {
  const a = prev.split("\n"), b = next.split("\n");
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  if (start === a.length && start === b.length) return;
  let end = 0;
  while (end < a.length - start && end < b.length - start && a[a.length - 1 - end] === b[b.length - 1 - end]) end++;
  return { before: [start + 1, Math.max(start + 1, a.length - end)], after: [start + 1, Math.max(start + 1, b.length - end)] };
}

// blocksIn are the innermost rendered blocks over the given source lines, so a
// list item is found and not its whole list. With none, the next block below.
function blocksIn(doc: HTMLElement, [from, to]: [number, number]) {
  const all = [...doc.querySelectorAll<HTMLElement>("[data-ls]")];
  const hit = all.filter((el) => Number(el.dataset.ls) <= to && Number(el.dataset.le) >= from);
  const inner = hit.filter((el) => !hit.some((o) => o !== el && el.contains(o)));
  if (inner.length) return inner;
  const below = all.find((el) => Number(el.dataset.ls) >= from);
  return below ? [below] : [];
}

// reveal scrolls el into view and resolves once it is there.
function reveal(el: HTMLElement) {
  return new Promise<void>((done) => {
    const r = el.getBoundingClientRect();
    if (r.top >= 80 && r.bottom <= window.innerHeight - 40) { done(); return; }
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    let finished = false;
    const finish = () => { if (finished) return; finished = true; seen.disconnect(); clearTimeout(timer); setTimeout(done, 120); };
    const seen = new IntersectionObserver(([e]) => { if (e.intersectionRatio > 0.9 || e.isIntersecting && e.intersectionRect.height >= 120) finish(); }, { threshold: [0.5, 0.9, 1] });
    seen.observe(el);
    const timer = setTimeout(finish, 900);
  });
}

// useFollowChanges follows the agent through the plan. A new version is not
// shown at once: the view first scrolls, with the old text, to where it
// changes, and only then shows the new text and marks the changed blocks.
// Versions that arrive meanwhile are folded into the latest. Scrolling by
// hand pauses it (changes then appear in place) and, once the agent has
// edited, offers to follow again.
function useFollowChanges(body: string | undefined, doc: RefObject<HTMLDivElement | null>, editing: boolean) {
  const [shown, setShown] = useState(body);
  const latest = useRef(body);
  latest.current = body;
  const busy = useRef(false);
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;
  // live: the agent has changed the plan while it is on screen, so there is something to follow.
  const [live, setLive] = useState(false);
  useEffect(() => {
    if (body === undefined || body === shown || busy.current) return;
    const range = shown === undefined ? undefined : changedLines(shown, body);
    if (!range || editing || pausedRef.current || !doc.current) { setShown(body); return; }
    const target = blocksIn(doc.current, range.before)[0];
    if (!target) { setShown(body); return; }
    busy.current = true;
    reveal(target).then(() => { busy.current = false; setShown(latest.current); });
  }, [body, shown, editing]); // eslint-disable-line react-hooks/exhaustive-deps
  const before = useRef(shown);
  useLayoutEffect(() => {
    const prev = before.current;
    before.current = shown;
    if (prev === undefined || shown === undefined || !doc.current) return;
    const range = changedLines(prev, shown);
    if (!range) return;
    const marks = blocksIn(doc.current, range.after);
    marks.forEach((el) => { el.classList.remove("plan-changed"); void el.offsetWidth; el.classList.add("plan-changed"); });
    if (!marks[0]) return;
    setLive(true);
    if (!pausedRef.current) marks[0].scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [shown]); // eslint-disable-line react-hooks/exhaustive-deps
  return {
    text: shown ?? body ?? "",
    paused: paused && live,
    pause: () => setPaused(true),
    resume: () => {
      setPaused(false);
      doc.current?.querySelectorAll<HTMLElement>(".plan-changed")[0]?.scrollIntoView({ behavior: "smooth", block: "center" });
    },
  };
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
