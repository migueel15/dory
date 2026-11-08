import { useEffect, useMemo, useState, type KeyboardEvent, type RefObject } from "react";
import { useQuery } from "@tanstack/react-query";
import { File, Folder, GitPullRequest, Lightning, Terminal } from "@phosphor-icons/react";
import { api, type T3ComposerEntry } from "../api/client";
import type { Token as TokenRef } from "./composer-tokens";

// Composer triggers, as in T3 Code: /command (only at the start), $skill,
// @path of the thread's workspace and #pull request.
type Kind = "command" | "skill" | "path" | "ref";
type Trigger = { kind: Kind; query: string; start: number; end: number };
type Suggestion = { key: string; label: string; hint?: string; insert: string; icon: React.ReactNode; token: TokenRef };

const sigils: Record<string, Kind> = { "/": "command", $: "skill", "@": "path", "#": "ref" };
const titles: Record<Kind, string> = { command: "Comandos", skill: "Skills", path: "Ficheros y carpetas", ref: "Pull requests" };
const prStates: Record<string, string> = { open: "Ready", draft: "Draft", merged: "Merged", closed: "Closed" };
const prTone: Record<string, string> = { open: "text-green-600", draft: "text-slate-400", merged: "text-violet-600", closed: "text-red-600" };

function triggerAt(text: string, caret: number): Trigger | null {
  const m = /(^|\s)([/$@#])([^\s]*)$/.exec(text.slice(0, caret));
  if (!m) return null;
  const start = caret - m[2].length - m[3].length;
  const kind = sigils[m[2]];
  if (kind === "command" && start !== 0) return null;
  return { kind, query: m[3], start, end: caret };
}

const match = (q: string, ...fields: (string | undefined)[]) => !q || fields.some((f) => f?.toLowerCase().includes(q.toLowerCase()));
const baseName = (path: string) => path.replace(/\/+$/, "").split("/").pop() || path;

// useComposerSuggest adds T3 Code's / $ @ # autocomplete to a textarea. Each
// pick becomes a token the composer keeps; onToken returns the text to write.
export function useComposerSuggest({ threadId, text, setText, area, commands, skills, repo, onToken }: {
  threadId: string; text: string; setText: (t: string) => void; area: RefObject<HTMLTextAreaElement | null>;
  commands: T3ComposerEntry[]; skills: T3ComposerEntry[];
  // repo (owner/name) is the thread's project repository: # lists only its pull requests.
  repo?: string;
  onToken: (insert: string, token: TokenRef) => string;
}) {
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);
  const [closed, setClosed] = useState<number | null>(null);
  const trigger = triggerAt(text, caret);
  const open = !!trigger && closed !== trigger.start;

  // @ asks T3 for the workspace entries; the query waits a moment for typing to settle.
  const [pathQuery, setPathQuery] = useState("");
  useEffect(() => {
    if (trigger?.kind !== "path") return;
    const t = setTimeout(() => setPathQuery(trigger.query), 150);
    return () => clearTimeout(t);
  }, [trigger?.kind, trigger?.query]);
  const entries = useQuery({ queryKey: ["t3-entries", threadId, pathQuery], queryFn: () => api.t3Entries(threadId, pathQuery), enabled: open && trigger?.kind === "path", staleTime: 30_000 });
  // # lists every pull request of the project's repository, whatever its state.
  const [refQuery, setRefQuery] = useState("");
  useEffect(() => {
    if (trigger?.kind !== "ref") return;
    const t = setTimeout(() => setRefQuery(trigger.query), 200);
    return () => clearTimeout(t);
  }, [trigger?.kind, trigger?.query]);
  const pulls = useQuery({ queryKey: ["github-pull-refs", repo, refQuery], queryFn: () => api.githubPullRefs(repo!, refQuery), enabled: open && trigger?.kind === "ref" && !!repo, staleTime: 60_000, retry: false });

  const items = useMemo((): Suggestion[] => {
    if (!trigger || !open) return [];
    const q = trigger.query;
    switch (trigger.kind) {
      case "command":
        return commands.filter((c) => match(q, c.name, c.description)).slice(0, 30).map((c) => ({
          key: c.name, label: `/${c.name}`, hint: c.description, insert: `/${c.name}`, icon: <Terminal className="size-4" />,
          token: { kind: "command", value: c.name, title: c.description },
        }));
      case "skill":
        return skills.filter((s) => match(q, s.name, s.description)).slice(0, 30).map((s) => ({
          key: s.name, label: `$${s.name}`, hint: s.description, insert: `$${s.name}`, icon: <Lightning className="size-4" />,
          token: { kind: "skill", value: s.name, title: s.description },
        }));
      case "path":
        return (entries.data?.entries ?? []).map((e) => ({
          key: e.path, label: e.path, insert: `@${baseName(e.path)}`, icon: e.kind === "directory" ? <Folder className="size-4" /> : <File className="size-4" />,
          token: { kind: "path", value: e.path, title: e.path },
        }));
      case "ref":
        return (pulls.data?.pulls ?? []).filter((p) => match(q, String(p.number), p.title)).map((p) => ({
          key: String(p.number), label: `#${p.number} ${p.title}`, hint: prStates[p.state], insert: `#${p.number}`,
          icon: <GitPullRequest weight={p.state === "merged" ? "fill" : "regular"} className={`size-4 ${prTone[p.state]}`} />,
          token: { kind: "ref", value: String(p.number), title: `#${p.number} ${p.title}` },
        }));
    }
  }, [trigger?.kind, trigger?.query, open, commands, skills, entries.data, pulls.data, repo]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => setActive(0), [trigger?.kind, trigger?.query]);

  const pick = (s: Suggestion) => {
    if (!trigger) return;
    const insert = `${onToken(s.insert, s.token)} `;
    setText(text.slice(0, trigger.start) + insert + text.slice(trigger.end));
    const pos = trigger.start + insert.length;
    requestAnimationFrame(() => { area.current?.focus(); area.current?.setSelectionRange(pos, pos); setCaret(pos); });
  };

  // onKeyDown handles the menu keys; it returns true when it used the key.
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!open || !items.length) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => (a + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length);
      return true;
    }
    if ((e.key === "Enter" && !e.shiftKey) || e.key === "Tab") { e.preventDefault(); pick(items[active]); return true; }
    if (e.key === "Escape") { e.preventDefault(); setClosed(trigger!.start); return true; }
    return false;
  };
  const track = () => { const el = area.current; if (el) setCaret(el.selectionStart ?? 0); };
  const reset = () => setClosed(null);

  const loading = trigger?.kind === "path" ? entries.isFetching : trigger?.kind === "ref" ? pulls.isFetching : false;
  const menu = open && trigger ? (
    <div role="listbox" aria-label={titles[trigger.kind]} className="notif-in absolute inset-x-3 bottom-full z-30 mb-2 max-h-72 overflow-y-auto rounded-2xl border border-slate-200 bg-white p-1 shadow-xl dark:border-border dark:bg-surface">
      <p className="px-2.5 pb-1 pt-1.5 text-[11px] font-medium uppercase tracking-wide text-slate-400">{titles[trigger.kind]}</p>
      {items.length === 0 ? <p className="px-2.5 py-2 text-sm text-slate-400">{loading ? "Buscando…" : "Sin resultados"}</p> : items.map((s, i) => (
        <button key={s.key} type="button" role="option" aria-selected={i === active} onMouseDown={(e) => e.preventDefault()} onMouseEnter={() => setActive(i)} onClick={() => pick(s)}
          className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-sm ${i === active ? "bg-slate-100 dark:bg-raised" : ""}`}>
          <span className="shrink-0 text-slate-400">{s.icon}</span>
          <span className="min-w-0 truncate font-medium">{s.label}</span>
          {s.hint && <span className="ml-auto min-w-0 max-w-[50%] shrink truncate text-xs text-slate-400">{s.hint}</span>}
        </button>
      ))}
    </div>
  ) : null;

  return { menu, onKeyDown, track, reset };
}
