import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Crosshair } from "@phosphor-icons/react";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { Markdown } from "@tiptap/markdown";
import type { Node as PMNode } from "@tiptap/pm/model";
import { TextSelection } from "@tiptap/pm/state";
import { api, ApiError, type Plan } from "../api/client";
import { Button, ErrorText, prose } from "./ui";

// PlanDoc shows the plan of a task: one Markdown document that the user and
// the agents edit. It is edited in place already rendered (headings, lists and
// checkboxes as you type "# ", "- " or "[] "), saved by itself, and follows the
// agent's edits live: each block that changes is marked and, with "Seguir" on,
// the view goes there before the change is shown.
export function PlanDoc({ taskId }: { taskId: string }) {
  const qc = useQueryClient();
  const plan = useQuery({ queryKey: ["plans", taskId, "doc"], queryFn: () => api.plan(taskId) });
  const p = plan.data;
  // Following the agent is a choice kept in this browser; on by default.
  const [following, setFollowing] = useState(() => { try { return localStorage.getItem("dory.plan-follow") !== "0"; } catch { return true; } });
  const setFollow = (v: boolean) => { setFollowing(v); try { localStorage.setItem("dory.plan-follow", v ? "1" : "0"); } catch { /* per-browser only */ } };
  const doc = usePlanEditor(taskId, p, following);
  const accept = useMutation({
    mutationFn: () => api.acceptPlan(taskId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["plans", taskId] }); qc.invalidateQueries({ queryKey: ["card", taskId] }); qc.invalidateQueries({ queryKey: ["board"] }); },
  });

  if (plan.error) return <div className="p-4"><ErrorText error={plan.error} /></div>;
  if (!p) return null;
  return (
    <div className="flex min-h-full flex-col">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-slate-200 bg-slate-50/90 px-4 py-2 text-xs text-slate-500 backdrop-blur dark:border-border dark:bg-canvas/90 dark:text-muted">
        <span>{p.revision > 0 ? `Revisión ${p.revision} · ${p.author === "agent" ? "agente" : "tú"}` : "Sin plan"}</span>
        <button type="button" onClick={() => setFollow(!following)} aria-pressed={following}
          title={following ? "Siguiendo al agente: la vista va a cada cambio. Pulsa para dejar de seguirlo" : "Seguir al agente: llevar la vista a cada cambio"}
          className={`flex items-center gap-1 rounded-md px-1.5 py-0.5 ${following ? "bg-accent/15 text-accent-text" : "hover:bg-slate-200/70 dark:hover:bg-raised"}`}>
          <Crosshair weight={following ? "bold" : "regular"} className="size-3.5" />Seguir
        </button>
        {p.accepted_at && <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400"><Check className="size-3.5" />Aceptado</span>}
        <span className="flex-1" />
        <span aria-live="polite">{doc.state === "saving" ? "Guardando…" : doc.state === "dirty" ? "Sin guardar" : ""}</span>
        {!doc.empty && !p.accepted_at && <Button className="text-xs" onClick={() => accept.mutate()} disabled={accept.isPending || doc.state !== "saved"}>Aceptar plan</Button>}
      </div>
      {doc.state === "conflict" && (
        <p className="m-3 mb-0 flex flex-wrap items-center gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
          El plan ha cambiado mientras escribías y tus últimos cambios no se han guardado.
          <button type="button" className="underline" onClick={doc.reload}>Cargar el plan actual</button>
        </p>
      )}
      <div className="relative flex-1">
        {doc.empty && <p className="pointer-events-none absolute left-5 top-4 text-sm text-slate-400 dark:text-muted">Escribe el plan o pide al agente que lo escriba. Usa «# » para títulos, «- » para listas y «[] » para casillas.</p>}
        <EditorContent editor={doc.editor} className={`${prose} plan-editor px-5 py-4`} />
      </div>
      <div className="px-5 pb-3"><ErrorText error={(doc.state === "conflict" ? undefined : doc.error) ?? accept.error} /></div>
    </div>
  );
}

type SyncState = "saved" | "dirty" | "saving" | "conflict";

// usePlanEditor keeps the editor and the stored plan in step. Typing saves on
// a short pause against the revision the text came from; a newer revision
// from the agent is applied when there is nothing unsaved, and a save that
// lost the race is reported instead of overwriting the other edit.
function usePlanEditor(taskId: string, p: Plan | undefined, following: boolean) {
  const qc = useQueryClient();
  // base is the revision the editor text comes from; stored its Markdown.
  const base = useRef<number | undefined>(undefined);
  const stored = useRef("");
  const applying = useRef(false);
  const edits = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const [state, setState] = useState<SyncState>("saved");
  const stateRef = useRef(state);
  stateRef.current = state;
  const [error, setError] = useState<unknown>();
  const [empty, setEmpty] = useState(true);
  const followRef = useRef(following);
  followRef.current = following;

  const save = async (ed: Editor) => {
    clearTimeout(timer.current);
    if (stateRef.current === "conflict" || base.current === undefined) return;
    const md = ed.getMarkdown().trim();
    if (md === stored.current) { setState("saved"); return; }
    const at = edits.current;
    setState("saving");
    try {
      const next = await api.savePlan(taskId, { body_md: md, revision: base.current });
      base.current = next.revision;
      stored.current = next.body_md;
      qc.setQueryData(["plans", taskId, "doc"], next);
      qc.invalidateQueries({ queryKey: ["card", taskId] });
      setError(undefined);
      // Typing during the save leaves more to save.
      if (edits.current !== at) { setState("dirty"); timer.current = setTimeout(() => save(ed), 700); } else setState("saved");
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) setState("conflict");
      else { setError(e); setState("dirty"); }
    }
  };

  const editor = useEditor({
    extensions: [StarterKit.configure({ link: { openOnClick: false } }), TaskList, TaskItem.configure({ nested: true }), Markdown],
    content: "",
    contentType: "markdown",
    onUpdate: ({ editor: ed }) => {
      setEmpty(ed.isEmpty);
      if (applying.current) return;
      edits.current++;
      if (stateRef.current !== "conflict") setState("dirty");
      clearTimeout(timer.current);
      timer.current = setTimeout(() => save(ed), 700);
    },
    onBlur: ({ editor: ed }) => { if (stateRef.current === "dirty") save(ed); },
  });

  // set replaces the text without counting it as an edit, keeping the cursor
  // where it was as far as the new text allows.
  const set = (ed: Editor, md: string) => {
    const { from, to } = ed.state.selection;
    applying.current = true;
    ed.commands.setContent(md, { contentType: "markdown", emitUpdate: false });
    applying.current = false;
    if (ed.isFocused) {
      const max = ed.state.doc.content.size;
      ed.view.dispatch(ed.state.tr.setSelection(TextSelection.create(ed.state.doc, Math.min(from, max), Math.min(to, max))));
    }
    setEmpty(ed.isEmpty);
  };

  // Remote revisions: the first one loads the editor; later ones are shown
  // one at a time, scrolling first to where they change when following.
  const latest = useRef(p);
  latest.current = p;
  const busy = useRef(false);
  const apply = async (ed: Editor) => {
    const next = latest.current;
    if (busy.current || !next || next.revision === base.current) return;
    if (base.current === undefined) { set(ed, next.body_md); base.current = next.revision; stored.current = next.body_md; return; }
    if (next.revision < base.current) return;
    if (stateRef.current !== "saved") { if (stateRef.current === "dirty") setState("conflict"); return; }
    busy.current = true;
    try {
      const target = followRef.current ? changedBlocks(ed.state.doc, parse(ed, next.body_md)).before[0] : undefined;
      const el = target === undefined ? null : (ed.view.nodeDOM(target) as HTMLElement | null);
      if (el) await reveal(el);
      const shown = latest.current!;
      if (stateRef.current !== "saved") return;
      const old = ed.state.doc;
      set(ed, shown.body_md);
      base.current = shown.revision;
      stored.current = shown.body_md;
      const marks = changedBlocks(old, ed.state.doc).after.map((pos) => ed.view.nodeDOM(pos)).filter((n): n is HTMLElement => n instanceof HTMLElement);
      marks.forEach((m) => { m.classList.remove("plan-changed"); void m.offsetWidth; m.classList.add("plan-changed"); });
      if (marks[0] && followRef.current) marks[0].scrollIntoView({ behavior: "smooth", block: "nearest" });
    } finally {
      busy.current = false;
    }
    // A revision that arrived meanwhile comes next.
    if (latest.current && latest.current.revision !== base.current) apply(ed);
  };
  useEffect(() => { if (editor) apply(editor); }, [editor, p?.revision]); // eslint-disable-line react-hooks/exhaustive-deps

  // Leaving with unsaved text saves it.
  useEffect(() => () => { if (editor && stateRef.current === "dirty") save(editor); }, [editor]); // eslint-disable-line react-hooks/exhaustive-deps

  return {
    editor,
    state,
    error,
    empty,
    // reload drops the unsaved text and loads the stored plan.
    reload: () => {
      if (!editor || !latest.current) return;
      clearTimeout(timer.current);
      set(editor, latest.current.body_md);
      base.current = latest.current.revision;
      stored.current = latest.current.body_md;
      setError(undefined);
      setState("saved");
    },
  };
}

function parse(ed: Editor, md: string): PMNode {
  return ed.schema.nodeFromJSON(ed.markdown!.parse(md));
}

// changedBlocks compares two documents and gives the positions of the
// innermost blocks that differ, in each of them: when a single list, list item
// or quote changed, it looks inside it, so one item is found and not the whole
// list. A pure insertion or deletion points at the block that follows it.
function changedBlocks(a: PMNode, b: PMNode) {
  const walk = (x: PMNode, y: PMNode, xStart: number, yStart: number): { before: number[]; after: number[] } => {
    const xs: [PMNode, number][] = [], ys: [PMNode, number][] = [];
    x.forEach((n, off) => xs.push([n, xStart + off]));
    y.forEach((n, off) => ys.push([n, yStart + off]));
    let s = 0;
    while (s < xs.length && s < ys.length && xs[s][0].eq(ys[s][0])) s++;
    let e = 0;
    while (e < xs.length - s && e < ys.length - s && xs[xs.length - 1 - e][0].eq(ys[ys.length - 1 - e][0])) e++;
    const rx = xs.slice(s, xs.length - e), ry = ys.slice(s, ys.length - e);
    if (rx.length === 1 && ry.length === 1 && rx[0][0].type === ry[0][0].type && !rx[0][0].isTextblock && rx[0][0].childCount > 0 && ry[0][0].childCount > 0) {
      return walk(rx[0][0], ry[0][0], rx[0][1] + 1, ry[0][1] + 1);
    }
    const at = (list: [PMNode, number][], all: [PMNode, number][]) => (list.length ? list.map(([, pos]) => pos) : all[s] ? [all[s][1]] : []);
    return { before: at(rx, xs), after: at(ry, ys) };
  };
  return walk(a, b, 0, 0);
}

// reveal scrolls el into view and resolves once it is there.
function reveal(el: HTMLElement) {
  return new Promise<void>((done) => {
    const r = el.getBoundingClientRect();
    if (r.top >= 80 && r.bottom <= window.innerHeight - 40) { done(); return; }
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    let finished = false;
    const finish = () => { if (finished) return; finished = true; seen.disconnect(); clearTimeout(timer); setTimeout(done, 120); };
    const seen = new IntersectionObserver(([e]) => { if (e.intersectionRatio > 0.9 || (e.isIntersecting && e.intersectionRect.height >= 120)) finish(); }, { threshold: [0.5, 0.9, 1] });
    seen.observe(el);
    const timer = setTimeout(finish, 900);
  });
}
