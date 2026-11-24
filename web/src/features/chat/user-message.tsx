import { useEffect, useRef, useState } from "react";
import { GitMerge, GitPullRequest, Lightning, Rocket, CaretDown, CaretRight, ClipboardText, FileText, ListChecks, X } from "@phosphor-icons/react";
import { api, type Attachment } from "../../api/client";
import { Md } from "../../components/ui";
import { splitMessageQuotes } from "./message-quotes";
import { Link } from "@tanstack/react-router";
import { parseAutoMessage, type AutoMessage } from "../../lib/auto-message";
import { RefLink } from "../../components/refs";
import { t } from "../../i18n/es";
import { MessageParts } from "./message-parts";

// AUTO names each kind of message Dory writes for the user, with its icon.
export const AUTO: Record<string, { label: string; hint: string; icon: typeof ClipboardText }> = {
  plan: { label: "Modo plan", hint: "definir la tarea antes de implementarla", icon: ClipboardText },
  implement: { label: "Implementar", hint: "trabajar la tarea en su rama", icon: Rocket },
  "issue-tasks": { label: "Tareas de la issue", hint: "partir la issue en tareas", icon: ListChecks },
  "pr-conflicts": { label: "Conflictos", hint: "resolver los conflictos de la pull request", icon: GitMerge },
  "pr-review": { label: "Revisión", hint: "revisar la pull request", icon: GitPullRequest },
  "pr-question": { label: "Pregunta sobre el código", hint: "", icon: GitPullRequest },
};

// AutoStart shows a message Dory wrote for the user: what it is for, a link
// to what it is about and the user's own words. The context the agent got
// folds below.
export function AutoStart({ msg }: { msg: AutoMessage }) {
  const [open, setOpen] = useState(false);
  const meta = AUTO[msg.kind] ?? { label: "Mensaje de Dory", hint: "", icon: Lightning };
  const Icon = meta.icon;
  const { task, title, repo, pr, issue, path, start, end } = msg.attrs;
  const ref = pr ? `#${pr}` : issue ? `#${issue}` : "";
  const where = path ? `${path}${start && start !== "0" ? `:${start}${end && end !== start ? `-${end}` : ""}` : ""}` : "";
  return (
    <div className="w-full max-w-[85%] overflow-hidden rounded-2xl rounded-br-md border border-slate-200 bg-white dark:border-border dark:bg-surface">
      <div className="flex min-w-0 items-center gap-2 border-b border-slate-200 px-4 py-2 text-sm dark:border-border">
        <Icon className="size-4 shrink-0 text-accent dark:text-accent-text" aria-hidden="true" />
        <span className="shrink-0 font-semibold">{meta.label}</span>
        <span className="min-w-0 truncate text-slate-500 dark:text-muted">{title ? `· ${title}` : where ? `· ${where}` : meta.hint ? `· ${meta.hint}` : ""}</span>
        <span className="ml-auto shrink-0 text-xs text-slate-500 dark:text-muted">
          {task && <Link to="/tasks" search={{ task } as never} className="hover:underline">Ver tarea</Link>}
          {repo && ref && <RefLink href={`https://github.com/${repo}/${pr ? "pull" : "issues"}/${pr ?? issue}`} className="hover:underline">{repo}{ref}</RefLink>}
        </span>
      </div>
      {msg.body.trim() && <div className="px-4 py-2.5"><MessageParts text={msg.body} /></div>}
      <div className="border-t border-slate-200 px-4 py-1.5 dark:border-border">
        <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
          {open ? <CaretDown className="size-3" /> : <CaretRight className="size-3" />}Contexto enviado al agente
        </button>
        {open && <div className="mt-1 pb-1 text-[0.8125rem] text-slate-600 dark:text-muted [&_*]:!text-inherit"><Md>{msg.context}</Md></div>}
      </div>
    </div>
  );
}

export function UserMessageBody({ text }: { text: string }) {
  const auto = parseAutoMessage(text);
  if (auto) return <AutoStart msg={auto} />;
  const parts = splitMessageQuotes(text);
  const hasQuotes = parts.some((part) => part.kind !== "text");
  if (!hasQuotes) {
    return (
      <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent">
        <MessageParts text={text} />
      </div>
    );
  }
  return (
    <div className="flex max-w-[85%] flex-col gap-1.5 rounded-2xl rounded-br-md bg-slate-100 px-4 py-2.5 dark:bg-raised">
      <MessageParts text={text} />
    </div>
  );
}

// SentFiles shows the files sent with a message, above its bubble.
export function SentFiles({ threadId, detail }: { threadId: string; detail?: string }) {
  const [image, setImage] = useState<Attachment>();
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (image && dialog.current && !dialog.current.open) dialog.current.showModal();
  }, [image]);
  let files: Attachment[] = [];
  try { files = detail ? JSON.parse(detail) : []; } catch { return null; }
  if (!Array.isArray(files) || !files.length) return null;
  return (
    <>
      <div className="mb-1 flex max-w-[85%] flex-wrap justify-end gap-2">
        {files.map((f) => {
          const url = api.attachmentUrl(threadId, f.id);
          return f.mime?.startsWith("image/")
            ? <button key={f.id} type="button" onClick={() => setImage(f)} title={f.name} aria-label={`Ampliar ${f.name}`} className="max-w-full cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 dark:border-border">
              <img src={url} alt={f.name} className="max-h-48 w-auto max-w-full sm:max-w-64" />
            </button>
            : <a key={f.id} href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 px-3 py-2 text-xs text-slate-600 hover:bg-slate-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised"><FileText className="size-4" />{f.name}</a>;
        })}
      </div>
      {image && (
        <dialog ref={dialog} aria-label={image.name} onClose={() => setImage(undefined)}
          onClick={(event) => { if (event.target === event.currentTarget) event.currentTarget.close(); }}
          className="m-auto max-h-[92dvh] max-w-[94vw] overflow-hidden rounded-xl bg-white text-slate-900 backdrop:bg-black/75 dark:bg-surface dark:text-text">
          <div className="flex items-center justify-between gap-4 px-3 py-2">
            <span className="min-w-0 truncate text-sm">{image.name}</span>
            <button type="button" aria-label="Cerrar imagen" onClick={() => dialog.current?.close()} className="grid size-7 shrink-0 place-items-center text-slate-400 hover:text-slate-900 dark:hover:text-white"><X weight="regular" className="size-4" /></button>
          </div>
          <img src={api.attachmentUrl(threadId, image.id)} alt={image.name} className="max-h-[82dvh] max-w-full object-contain" />
        </dialog>
      )}
    </>
  );
}

