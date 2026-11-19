import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Crosshair } from "@phosphor-icons/react";
import { EditorContent, Extension, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import { Markdown } from "@tiptap/markdown";
import type { Node as PMNode } from "@tiptap/pm/model";
import { api, ApiError, type Plan } from "../api/client";
import { merge3 } from "../lib/merge3";
import { Button, ErrorText, prose } from "./ui";

// PlanDoc shows the plan of a task: one Markdown document that the user and
// the agents edit. It is edited in place already rendered (headings, lists and
// checkboxes as you type "# ", "- " or "[] "), saved by itself, and follows the
// agent's edits live: each block that changes is marked and, with "Seguir" on,
// the view goes there before the change is shown.
export function PlanDoc({ taskId, showStatus = true }: { taskId: string; showStatus?: boolean }) {
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
        {showStatus && <span>{p.revision > 0 ? `Revisión ${p.revision} · ${p.author === "agent" ? "agente" : "tú"}` : "Sin plan"}</span>}
        <button type="button" onClick={() => setFollow(!following)} aria-pressed={following}
          title={following ? "Siguiendo al agente: la vista va a cada cambio. Pulsa para dejar de seguirlo" : "Seguir al agente: llevar la vista a cada cambio"}
          className={`flex items-center gap-1 rounded-md px-1.5 py-0.5 ${following ? "bg-accent/15 text-accent-text" : "hover:bg-slate-200/70 dark:hover:bg-raised"}`}>
          <Crosshair weight={following ? "bold" : "regular"} className="size-3.5" />Seguir
        </button>
        {showStatus && p.accepted_at && <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400"><Check className="size-3.5" />Aceptado</span>}
        <span className="flex-1" />
        <span aria-live="polite">{doc.state === "saving" ? "Guardando…" : doc.state === "dirty" ? "Sin guardar" : ""}</span>
        {!doc.empty && !p.accepted_at && <Button className="text-xs" onClick={() => accept.mutate()} disabled={accept.isPending || doc.state !== "saved"}>Aceptar plan</Button>}
      </div>
      {doc.notice && (
        <p className="m-3 mb-0 flex flex-wrap items-center gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
          {doc.notice}
          <button type="button" className="underline" onClick={doc.dismiss}>Entendido</button>
        </p>
      )}
      <div className="relative flex-1">
        {doc.empty && <p className="pointer-events-none absolute left-5 top-4 text-sm text-slate-400 dark:text-muted">Escribe el plan o pide al agente que lo escriba. Usa «# » para títulos, «- » para listas y «[] » para casillas.</p>}
        <EditorContent editor={doc.editor} className={`${prose} plan-editor px-5 py-4`} />
      </div>
      <div className="px-5 pb-3"><ErrorText error={doc.error ?? accept.error} /></div>
    </div>
  );
}

type SyncState = "saved" | "dirty" | "saving";

// usePlanEditor keeps the editor and the stored plan in step. Typing saves on
// a short pause against the revision the text came from. A newer revision
// from the agent is shown as it arrives; when there is unsaved text, the two
// are merged by lines (lib/merge3) and the result is saved, so both writers
// keep their changes. Where both changed the same line, both versions stay.
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
  const [notice, setNotice] = useState<string>();
  const [empty, setEmpty] = useState(true);
  const followRef = useRef(following);
  followRef.current = following;

  const save = async (ed: Editor) => {
    clearTimeout(timer.current);
    if (base.current === undefined) return;
    const md = ed.getMarkdown().trim();
    if (md === stored.current) { setState("saved"); stateRef.current = "saved"; apply(ed); return; }
    const at = edits.current;
    setState("saving");
    stateRef.current = "saving";
    try {
      const next = await api.savePlan(taskId, { body_md: md, revision: base.current });
      base.current = next.revision;
      stored.current = next.body_md;
      qc.setQueryData(["plans", taskId, "doc"], next);
      qc.invalidateQueries({ queryKey: ["card", taskId] });
      setError(undefined);
      // Typing during the save leaves more to save.
      if (edits.current !== at) { setState("dirty"); stateRef.current = "dirty"; timer.current = setTimeout(() => save(ed), 700); }
      else { setState("saved"); stateRef.current = "saved"; apply(ed); }
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        // Someone saved first: merge with their version and save again.
        setState("dirty");
        stateRef.current = "dirty";
        try {
          const fresh = await qc.fetchQuery({ queryKey: ["plans", taskId, "doc"], queryFn: () => api.plan(taskId), staleTime: 0 });
          latest.current = fresh;
          rebase(ed, fresh);
        } catch (err) { setError(err); }
      } else { setError(e); setState("dirty"); }
    }
  };

  const editor = useEditor({
    extensions: [StarterKit.configure({ link: { openOnClick: false } }), TaskList, TaskItem.configure({ nested: true }), Markdown, Flash],
    content: "",
    contentType: "markdown",
    onUpdate: ({ editor: ed }) => {
      setEmpty(ed.isEmpty);
      if (applying.current) return;
      edits.current++;
      setState("dirty");
      stateRef.current = "dirty";
      clearTimeout(timer.current);
      timer.current = setTimeout(() => save(ed), 700);
    },
    onBlur: ({ editor: ed }) => { if (stateRef.current === "dirty") save(ed); },
  });

  // set replaces the text without counting it as an edit. Only the stretch
  // that differs is replaced, in one transaction, so the cursor and what the
  // user is writing elsewhere stay in place.
  const set = (ed: Editor, md: string) => {
    const next = parse(ed, md);
    const cur = ed.state.doc;
    const start = cur.content.findDiffStart(next.content);
    if (start === null) return;
    let { a: endA, b: endB } = cur.content.findDiffEnd(next.content)!;
    const overlap = start - Math.min(endA, endB);
    if (overlap > 0) { endA += overlap; endB += overlap; }
    applying.current = true;
    ed.view.dispatch(ed.state.tr.replace(start, endA, next.slice(start, endB)).setMeta("addToHistory", false));
    applying.current = false;
    setEmpty(ed.isEmpty);
  };

  // mark highlights the blocks that differ from old, and with scroll brings
  // the first one into view.
  const flashes = useRef(0);
  const mark = (ed: Editor, old: PMNode, scroll: boolean) => {
    const doc = ed.state.doc;
    const ranges = changedBlocks(old, doc).after.flatMap((pos) => { const n = doc.nodeAt(pos); return n ? [{ from: pos, to: pos + n.nodeSize }] : []; });
    if (!ranges.length) return;
    // A decoration, not a class on the DOM: the editor redraws its nodes and
    // would drop a class it does not know. Clearing first restarts the blink.
    applying.current = true;
    ed.view.dispatch(ed.state.tr.setMeta(flashKey, "clear").setMeta("addToHistory", false));
    ed.view.dispatch(ed.state.tr.setMeta(flashKey, ranges).setMeta("addToHistory", false));
    applying.current = false;
    const n = ++flashes.current;
    setTimeout(() => { if (n === flashes.current && !ed.isDestroyed) ed.view.dispatch(ed.state.tr.setMeta(flashKey, "clear").setMeta("addToHistory", false)); }, 2600);
    const first = ed.view.nodeDOM(ranges[0].from);
    if (scroll && first instanceof HTMLElement) first.scrollIntoView({ behavior: "smooth", block: "nearest" });
  };

  // rebase merges a newer stored version into unsaved text and saves the
  // result. The view does not move: the user is writing.
  const rebase = (ed: Editor, next: Plan) => {
    const norm = (md: string) => ed.markdown!.serialize(ed.markdown!.parse(md)).trim();
    const label = next.author === "agent" ? "Versión del agente" : "Otra versión";
    const merged = merge3(norm(stored.current), ed.getMarkdown().trim(), norm(next.body_md), label);
    const old = ed.state.doc;
    set(ed, merged.text);
    base.current = next.revision;
    stored.current = next.body_md;
    mark(ed, old, false);
    if (merged.conflicts) setNotice(`El agente cambió lo mismo que tú en ${merged.conflicts === 1 ? "un punto" : `${merged.conflicts} puntos`}: he dejado tu versión y, debajo, la suya citada. Quédate con la que quieras.`);
    // The merged text is not saved yet: mark it so before saving it.
    stateRef.current = "dirty";
    edits.current++;
    save(ed);
  };

  // Remote revisions: the first one loads the editor; later ones are shown
  // one at a time and every changed block blinks. The user is writing while
  // the editor has the focus: then a change waits for the user's next save,
  // merges with it and the view does not move. Otherwise it shows at once
  // and, when following, the view first goes to where it changes.
  const latest = useRef(p);
  latest.current = p;
  const busy = useRef(false);
  const apply = async (ed: Editor) => {
    const next = latest.current;
    if (busy.current || !next || next.revision === base.current) return;
    if (base.current === undefined) { applying.current = true; ed.commands.setContent(next.body_md, { contentType: "markdown", emitUpdate: false }); applying.current = false; setEmpty(ed.isEmpty); base.current = next.revision; stored.current = next.body_md; return; }
    if (next.revision < base.current) return;
    // A save in flight runs this again when it ends; its conflict merges.
    if (stateRef.current === "saving") return;
    // Unsaved text is saved first (now, or at the writer's pause), and the
    // save merges with the newer version.
    if (stateRef.current === "dirty") { if (!ed.isFocused) save(ed); return; }
    const follow = followRef.current && !ed.isFocused;
    busy.current = true;
    try {
      const target = follow ? changedBlocks(ed.state.doc, parse(ed, next.body_md)).before[0] : undefined;
      const el = target === undefined ? null : (ed.view.nodeDOM(target) as HTMLElement | null);
      if (el) await reveal(el);
      const shown = latest.current!;
      if (stateRef.current !== "saved") return;
      const old = ed.state.doc;
      set(ed, shown.body_md);
      base.current = shown.revision;
      stored.current = shown.body_md;
      mark(ed, old, follow);
    } finally {
      busy.current = false;
    }
    // A revision that arrived meanwhile comes next.
    if (latest.current && latest.current.revision > (base.current ?? 0) && stateRef.current === "saved") apply(ed);
  };
  useEffect(() => { if (editor) apply(editor); }, [editor, p?.revision]); // eslint-disable-line react-hooks/exhaustive-deps

  // Leaving with unsaved text saves it.
  useEffect(() => () => { if (editor && stateRef.current === "dirty") save(editor); }, [editor]); // eslint-disable-line react-hooks/exhaustive-deps

  return {
    editor,
    state,
    error,
    empty,
    notice,
    dismiss: () => setNotice(undefined),
  };
}

// Flash draws the blocks just changed by someone else with the plan-changed
// class, as node decorations that follow the document as it changes.
const flashKey = new PluginKey<DecorationSet>("plan-flash");
const Flash = Extension.create({
  name: "planFlash",
  addProseMirrorPlugins() {
    return [new Plugin<DecorationSet>({
      key: flashKey,
      state: {
        init: () => DecorationSet.empty,
        apply(tr, set) {
          const meta = tr.getMeta(flashKey) as "clear" | { from: number; to: number }[] | undefined;
          if (meta === "clear") return DecorationSet.empty;
          if (meta) return DecorationSet.create(tr.doc, meta.map(({ from, to }) => Decoration.node(from, to, { class: "plan-changed" })));
          return set.map(tr.mapping, tr.doc);
        },
      },
      props: { decorations: (state) => flashKey.getState(state) },
    })];
  },
});

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
