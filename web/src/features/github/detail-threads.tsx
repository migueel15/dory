import { useState } from "react";
import { type GithubDetail, type GithubThread } from "../../api/client";
import { DictationField } from "../composer/dictation-field";
import { useDictationInProgress } from "../composer/local-dictation";
import { Badge, Button, Card, toggleTask } from "../../components/ui";
import { Avatar } from "./pull-row";
import { Icon, RefMd, Tip, icons, MentionArea } from "./github-ui";

export type ThreadAct = (a: { type: string; thread_id?: string; body?: string; comment_id?: string }) => Promise<unknown>;

// quote turns a message into a markdown quote to answer it.
export const quote = (who: string, body: string) => `> **@${who}**\n${body.trim().split("\n").map((l) => `> ${l}`).join("\n")}\n\n`;

// Message is one comment: who and when, the text, and buttons to answer it
// or, when Dory's account wrote it, to edit it in place.
export function Message({ author, verb, at, url, body, mine, editType, id, act, busy, onReply }: { author: string; verb?: string; at: string; url?: string; body: string; mine: boolean; editType: string; id: string; act: ThreadAct; busy: boolean; onReply: (q: string) => void }) {
  const [draft, setDraft] = useState<string>();
  const dictating = useDictationInProgress();
  const when = new Date(at).toLocaleString();
  const small = "grid size-7 place-items-center rounded-md text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-raised dark:hover:text-text";
  return (
    <div className="group/msg">
      <div className="mb-1 flex items-center gap-2 text-xs text-slate-500 dark:text-muted">
        <Avatar login={author} />
        <span><span className="font-medium text-slate-700 dark:text-text">{author}</span>{verb && ` ${verb}`} · {url ? <a href={url} target="_blank" rel="noreferrer" className="hover:underline">{when}</a> : when}</span>
        <span className="ml-auto flex gap-0.5 opacity-60 transition-opacity group-hover/msg:opacity-100">
          {body && <Tip text="Responder citando"><button type="button" aria-label="Responder citando" onClick={() => onReply(quote(author, body))} className={small}><Icon i={icons.reply} /></button></Tip>}
          {mine && id && !id.startsWith("pending-") && draft === undefined && <Tip text="Editar"><button type="button" aria-label="Editar" onClick={() => setDraft(body)} className={small}><Icon i={icons.edit} /></button></Tip>}
        </span>
      </div>
      {draft === undefined ? <RefMd onToggle={mine && id && !id.startsWith("pending-") && !busy ? (line) => act({ type: editType, comment_id: id, body: toggleTask(body, line) }) : undefined}>{body}</RefMd> : (
        <div className="space-y-2">
          <DictationField aria-label="Editar mensaje" autoFocus value={draft} onValue={setDraft} onKeyDown={(e) => e.key === "Escape" && setDraft(undefined)} rows={Math.min(14, draft.split("\n").length + 2)} className="p-2 font-mono" />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDraft(undefined)}>Cancelar</Button>
            <Button disabled={dictating || busy || !draft.trim()} onClick={() => { act({ type: editType, comment_id: id, body: draft }).catch(() => {}); setDraft(undefined); }}>Guardar</Button>
          </div>
        </div>
      )}
    </div>
  );
}

export function ThreadCard({ t, me, people, links, act, busy }: { t: GithubThread; me: string; people: string[]; links: GithubDetail["links"]; act: ThreadAct; busy: boolean }) {
  const [open, setOpen] = useState(!t.resolved);
  const [reply, setReply] = useState("");
  const dictating = useDictationInProgress();
  const send = () => { act({ type: "reply_thread", thread_id: t.id, body: reply }).catch(() => {}); setReply(""); };
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <button type="button" onClick={() => setOpen(!open)} className="min-w-0 break-all text-left font-mono text-xs">
          {open ? "▾" : "▸"} {t.path}{t.line ? `:${t.line}` : ""}
        </button>
        <div className="flex flex-wrap items-center gap-1">
          {t.outdated && <Badge>desactualizado</Badge>}
          {t.resolved && <Badge>resuelto{t.resolved_by && ` por ${t.resolved_by}`}</Badge>}
          <Button variant="ghost" disabled={busy} onClick={() => act({ type: t.resolved ? "unresolve_thread" : "resolve_thread", thread_id: t.id })}>
            {t.resolved ? "Desresolver" : "Resolver"}
          </Button>
        </div>
      </div>
      {!open && t.comments[0] && <div className="mt-1 truncate text-xs text-slate-500 dark:text-muted">{t.comments[0].author}: {t.comments[0].body}</div>}
      {open && <>
        {t.comments.map((c, i) => (
          <div key={i} className="mt-2 border-l-2 border-slate-200 pl-2 dark:border-border">
            <Message author={c.author} at={c.at} url={c.url} body={c.body} mine={c.author === me} editType="edit_thread_comment" id={c.id} act={act} busy={busy} onReply={(q) => { if (!dictating) setReply(q + reply); }} />
          </div>
        ))}
        <MentionArea people={people} links={links} aria-label="Responder en el hilo" value={reply} onValue={setReply} rows={2} placeholder="Responder…" className="mt-2 p-2" />
        <div className="flex gap-2">
          <Button disabled={dictating || busy || !reply.trim()} onClick={send}>Responder</Button>
          {reply && <Button variant="ghost" disabled={dictating} onClick={() => setReply("")}>Descartar</Button>}
        </div>
      </>}
    </Card>
  );
}

export function Threads({ threads, me, people, links, act, busy }: { threads: GithubThread[]; me: string; people: string[]; links: GithubDetail["links"]; act: ThreadAct; busy: boolean }) {
  if (threads.length === 0) return null;
  const pending = threads.filter((t) => !t.resolved).length;
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">Conversaciones de revisión · {pending} sin resolver de {threads.length}</h2>
      {threads.map((t) => <ThreadCard key={t.id} t={t} me={me} people={people} links={links} act={act} busy={busy} />)}
    </section>
  );
}

