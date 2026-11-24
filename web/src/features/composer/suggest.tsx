import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowUp, File as FileIcon, Lightning, TerminalWindow } from "@phosphor-icons/react";
import { api } from "../../api/client";
import { type Token } from "./composer-tokens";
import { qk } from "../../api/queries/keys";

export type Suggestion = { key: string; label: string; hint?: string; insert: string; icon: React.ReactNode; token: Token };
export const titles = { "/": "Comandos", $: "Skills", "@": "Ficheros" } as const;

// useSuggest autocompletes the composer as in v1: /command (only at the
// start), $skill and @file of the thread's folder. A pick replaces the typed
// word with how the harness invokes it.
// Source is where the menu reads from: a thread, or the harness and project of
// a thread not created yet (no files, and no /clear: there is no session).
export type SuggestSource = { key: unknown[]; commands: () => Promise<{ commands: CatalogEntry[]; skills: CatalogEntry[] }>; files?: (q: string) => Promise<string[]> };
export type CatalogEntry = { name: string; description: string; insert: string };
export const threadSource = (threadId: string, provider: string): SuggestSource => ({ key: [threadId, provider], commands: () => api.threadCommands(threadId), files: (q) => api.threadFiles(threadId, q) });

export function useSuggest(source: SuggestSource, text: string, area: React.RefObject<HTMLTextAreaElement | null>, setText: (t: string) => void, onToken: (insert: string, t: Token) => string = (insert) => insert) {
  const [word, setWord] = useState<{ start: number; sigil: "/" | "$" | "@"; q: string } | null>(null);
  const [index, setIndex] = useState(0);
  const catalog = useQuery({ queryKey: qk.commands(...source.key), queryFn: source.commands, enabled: word?.sigil === "/" || word?.sigil === "$", staleTime: 60_000 });
  const files = useQuery({ queryKey: qk.files(...source.key, word?.q), queryFn: () => source.files!(word!.q), enabled: word?.sigil === "@" && !!source.files, staleTime: 30_000 });
  const q = word?.q.toLowerCase() ?? "";
  const hit = (e: { name: string; description: string }) => !q || e.name.toLowerCase().includes(q) || e.description.toLowerCase().includes(q);
  const entry = (e: { name: string; description: string; insert: string }, icon: React.ReactNode, kind: "command" | "skill"): Suggestion => ({ key: e.insert, label: e.insert, hint: e.description, insert: e.insert, icon, token: { kind, value: e.name, title: e.description } });
  const list: Suggestion[] = !word ? []
    : word.sigil === "@" ? (files.data ?? []).map((f) => ({ key: f, label: f.split("/").pop()!, hint: f, insert: `@${f.split("/").pop()}`, icon: <FileIcon className="size-4 shrink-0 text-slate-400" />, token: { kind: "path", value: f } }))
    : word.sigil === "/" ? [...(catalog.data?.commands ?? []).filter((e) => hit(e) && (source.files || e.insert !== "/clear")).map((e) => entry(e, <TerminalWindow className="size-4 shrink-0 text-slate-400" />, "command")),
      ...(catalog.data?.skills ?? []).filter(hit).map((e) => entry(e, <Lightning className="size-4 shrink-0 text-slate-400" />, "skill"))]
    : (catalog.data?.skills ?? []).filter(hit).map((e) => entry(e, <Lightning className="size-4 shrink-0 text-slate-400" />, "skill"));
  const shown = list.slice(0, 50);
  const track = () => {
    const el = area.current;
    if (!el) return;
    const before = text.slice(0, el.selectionStart);
    const m = /(^|\s)([/$@])([^\s/$@]*)$/.exec(before);
    const start = m ? before.length - m[3].length - 1 : -1;
    // A command only makes sense at the start of the message.
    if (!m || (m[2] === "/" && start !== 0) || (m[2] === "@" && !source.files)) { setWord(null); return; }
    setWord((w) => (w && w.start === start && w.q === m[3] ? w : { start, sigil: m[2] as "/" | "$" | "@", q: m[3] }));
    setIndex(0);
  };
  useEffect(track, [text]); // eslint-disable-line react-hooks/exhaustive-deps
  const pick = (s: Suggestion) => {
    if (!word) return;
    const end = word.start + 1 + word.q.length;
    const written = onToken(s.insert, s.token);
    const next = `${text.slice(0, word.start)}${written} ${text.slice(end).replace(/^ /, "")}`;
    setText(next);
    setWord(null);
    requestAnimationFrame(() => { const el = area.current; if (el) { const at = word.start + written.length + 1; el.focus(); el.setSelectionRange(at, at); } });
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!shown.length) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setIndex((i) => (i + (e.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length); return true; }
    if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pick(shown[index]); return true; }
    if (e.key === "Escape") { e.preventDefault(); setWord(null); return true; }
    return false;
  };
  const loading = word && (word.sigil === "@" ? files.isPending : catalog.isPending);
  const menu = word && (shown.length > 0 || loading || word.sigil !== "@") && (
    <div className="absolute bottom-full left-3 z-30 mb-2 w-[min(32rem,calc(100%-1.5rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
      <p className="px-3 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-slate-400">{titles[word.sigil]}</p>
      <ul role="listbox" aria-label={titles[word.sigil]} className="max-h-64 overflow-y-auto p-1">
        {shown.map((s, i) => (
          <li key={s.key}>
            <button type="button" role="option" aria-selected={i === index} onMouseDown={(e) => { e.preventDefault(); pick(s); }} onMouseEnter={() => setIndex(i)}
              className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${i === index ? "bg-slate-100 dark:bg-raised" : ""}`}>
              {s.icon}
              <span className="shrink-0 font-medium">{s.label}</span>
              {s.hint && <span className="min-w-0 truncate text-xs text-slate-500 dark:text-muted">{s.hint}</span>}
            </button>
          </li>
        ))}
        {!shown.length && <li className="px-2 py-1.5 text-xs text-slate-400">{loading ? "Cargando…" : word.sigil === "/" ? "Este harness no tiene comandos." : "No hay skills que coincidan."}</li>}
      </ul>
    </div>
  );
  return { menu, track, onKeyDown };
}

