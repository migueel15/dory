import { Link, useNavigate } from "@tanstack/react-router";
import { type GithubPull } from "../../api/client";
import { Card, pill } from "../../components/ui";
import { GithubMark, Icon, Tip, icons } from "./github-ui";
import { type Icon as PhosphorIcon } from "@phosphor-icons/react";

export const ok = "text-green-700 dark:text-green-400";
export const bad = "text-red-600 dark:text-red-400";
export const wait = "text-amber-700 dark:text-amber-400";
export const ciText: Record<string, [string, string]> = { SUCCESS: ["CI correcta", ok], FAILURE: ["CI fallida", bad], ERROR: ["CI con error", bad], PENDING: ["CI en curso", wait], EXPECTED: ["CI pendiente", wait], "": ["Sin CI", "text-slate-500 dark:text-muted"] };

// Text and color of the merge status; "BLOCKED" is what branch protection reports.
export function mergeStatus(p: GithubPull): [string, string] {
  if (p.draft) return ["Draft", wait];
  if (p.mergeable === "CONFLICTING") return ["Con conflictos", bad];
  if (p.merge_state === "BLOCKED") return ["Bloqueada", bad];
  if (p.merge_state === "BEHIND") return ["Rama desactualizada", wait];
  if (p.mergeable === "MERGEABLE") return ["Se puede mergear", ok];
  return ["Calculando", wait];
}

export function approvalsText(p: GithubPull) {
  return p.required_approvals ? `${p.approvals}/${p.required_approvals} aprobaciones` : `${p.approvals} aprobaciones`;
}

export const tone = { ok: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300", bad: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300", wait: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300", none: "bg-slate-100 text-slate-600 dark:bg-raised dark:text-text-secondary" };
export const toneOf = (color: string) => (color === ok ? tone.ok : color === bad ? tone.bad : color === wait ? tone.wait : tone.none);
export function Chip({ icon, label, color, title }: { icon: PhosphorIcon; label: string; color: string; title: string }) {
  return (
    <Tip text={title}><span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium ${toneOf(color)}`}>
      <Icon i={icon} className="size-3.5" /> {label}
    </span></Tip>
  );
}

// Avatar shows a GitHub user's picture; the name appears as a tooltip.
export function Avatar({ login, mark, ring, title }: { login: string; mark?: string; ring?: string; title?: string }) {
  return (
    <Tip text={title ?? login}><span className="relative inline-block">
      <img src={`https://github.com/${login}.png?size=48`} alt={login} loading="lazy" className={`size-6 rounded-full bg-slate-200 ring-2 dark:bg-hover ${ring ?? "ring-white dark:ring-surface"}`} />
      {mark && <span className={`absolute -bottom-1 -right-1 rounded-full p-px ring-2 ring-white dark:ring-surface ${mark}`}><Icon i={mark.includes("green") ? icons.check : mark.includes("red") ? icons.x : icons.eye} className="size-2.5 text-white" /></span>}
    </span></Tip>
  );
}

export function PullStatus({ pull }: { pull: GithubPull }) {
  const [ci, ciColor] = ciText[pull.ci] ?? [pull.ci, wait];
  const [merge, mergeColor] = mergeStatus(pull);
  const enough = pull.required_approvals ? pull.approvals >= pull.required_approvals : pull.review_decision !== "REVIEW_REQUIRED";
  // One avatar per person. GitHub keeps someone's last review and also lists
  // them as requested when a new review is asked of them; the mark shows the
  // review and an amber ring the pending request.
  const people = new Map<string, { mark?: string; title: string[]; pending: boolean }>();
  const add = (l: string, f: (p: { mark?: string; title: string[]; pending: boolean }) => void) => {
    const p = people.get(l) ?? { title: [], pending: false };
    f(p); people.set(l, p);
  };
  pull.approved_by.forEach((l) => add(l, (p) => { p.mark = "bg-green-600"; p.title.push("aprobó"); }));
  pull.changes_by.forEach((l) => add(l, (p) => { p.mark = "bg-red-600"; p.title.push("pidió cambios"); }));
  pull.review_requests.forEach((l) => add(l, (p) => { p.pending = true; p.title.push(p.mark ? "revisión pedida de nuevo" : "revisión pendiente"); }));
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <Chip icon={icons.merge} label={merge} color={mergeColor} title="Estado de merge" />
      <Chip icon={icons.check} label={pull.required_approvals ? `${pull.approvals}/${pull.required_approvals}` : `${pull.approvals}`} color={enough ? ok : wait} title={approvalsText(pull)} />
      <Chip icon={pull.ci === "SUCCESS" ? icons.check : pull.ci === "FAILURE" || pull.ci === "ERROR" ? icons.x : icons.ci} label="CI" color={ciColor} title={ci} />
      {people.size > 0 && (
        <span className="ml-1 flex items-center gap-1.5">
          {[...people].map(([l, p]) => (
            <Avatar key={l} login={l} mark={p.mark ?? (p.pending ? "bg-amber-500" : undefined)} ring={p.mark && p.pending ? "ring-amber-500" : undefined} title={`${l}: ${p.title.join(" · ")}`} />
          ))}
        </span>
      )}
    </div>
  );
}

// ago formats an ISO date as a short relative time in Spanish.
export function ago(iso: string) {
  if (!iso) return "";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  const [n, u] = s < 3600 ? [Math.floor(s / 60), "min"] : s < 86400 ? [Math.floor(s / 3600), "h"] : s < 2592000 ? [Math.floor(s / 86400), "d"] : s < 31536000 ? [Math.floor(s / 2592000), "mes"] : [Math.floor(s / 31536000), "a"];
  return n < 1 ? "ahora" : `hace ${n} ${u}`;
}

export const Meta = ({ icon, children, title }: { icon: PhosphorIcon; children: React.ReactNode; title: string }) => (
  <Tip text={title}><span className="inline-flex items-center gap-1"><Icon i={icon} className="size-3.5" />{children}</span></Tip>
);

// Label uses the color GitHub gives it, softened so it reads in both themes.
export function Label({ name, color }: { name: string; color?: string }) {
  const c = color ? `#${color}` : undefined;
  return (
    <span className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[0.6875rem] font-medium text-slate-700 dark:text-text" style={c ? { borderColor: `${c}88`, backgroundColor: `${c}22` } : undefined}>
      <span className="size-2 rounded-full" style={{ backgroundColor: c ?? "#94a3b8" }} />{name}
    </span>
  );
}

export type CardItem = { repo: string; number: number; title: string; url: string; author: string; created_at: string; updated_at: string; comments: number; assignees: string[]; labels: string[]; label_colors: Record<string, string> };

// Row is the shared card of issues and pull requests: identity, people, activity and labels.
export function Row({ item, kind, meta, children }: { item: CardItem; kind: "pulls" | "issues"; meta?: React.ReactNode; children?: React.ReactNode }) {
  const [owner, name] = item.repo.split("/");
  const pull = kind === "pulls";
  const navigate = useNavigate();
  // A click anywhere on the card opens it, except on its own links and buttons.
  const open = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest("a, button") || window.getSelection()?.toString()) return;
    navigate({ to: "/github/$owner/$repo/$kind/$number", params: { owner, repo: name, kind, number: String(item.number) } });
  };
  return (
    <div onClick={open} className="cursor-pointer">
    <Card className="group space-y-2 transition-colors hover:border-slate-300 hover:bg-slate-50/60 dark:hover:border-border-strong dark:hover:bg-raised/40">
      <div className="flex items-start gap-2">
        <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo: name, kind, number: String(item.number) }} className="min-w-0 flex-1">
          <div className="font-medium break-words group-hover:text-blue-700 dark:group-hover:text-accent-text">{item.title}</div>
          <div className="mt-0.5 flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted">
            <span title={pull ? "Pull request" : "Issue"} className={`grid size-4.5 shrink-0 place-items-center rounded-full ${pull ? "bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300" : "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300"}`}>
              <Icon i={pull ? icons.pull : icons.issue} className="size-3" />
            </span>
            <span className="truncate font-mono">{item.repo}</span><span className="font-semibold">#{item.number}</span>
          </div>
        </Link>
        <Tip text="Abrir en GitHub"><a href={item.url} target="_blank" rel="noreferrer" aria-label="Abrir en GitHub" className={pill}><GithubMark className="size-3.5" /></a></Tip>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-muted">
        <span className="inline-flex items-center gap-1.5"><Avatar login={item.author} title={`Autor: ${item.author}`} /><span className="font-medium text-slate-700 dark:text-text">{item.author}</span></span>
        <Meta icon={icons.clock} title={`Creada ${new Date(item.created_at).toLocaleString()} · actualizada ${new Date(item.updated_at).toLocaleString()}`}>{ago(item.updated_at)}</Meta>
        <Meta icon={icons.comment} title={`${item.comments} comentarios`}>{item.comments}</Meta>
        {meta}
        {item.assignees.length > 0 && (
          <span className="ml-auto flex items-center -space-x-1.5">{item.assignees.map((a) => <Avatar key={a} login={a} title={`Asignada a ${a}`} />)}</span>
        )}
      </div>
      {(children || item.labels.length > 0) && (
        <div className="flex flex-wrap items-center gap-1.5">
          {children}
          {item.labels.map((l) => <Label key={l} name={l} color={item.label_colors[l]} />)}
        </div>
      )}
    </Card>
    </div>
  );
}

// linked turns the issues or pull requests referenced by a card into links to their own view.
export const linked = (repo: string, prefix: "PR" | "Issue", refs: { number: number; title: string }[]) => {
  const [owner, name] = repo.split("/");
  return refs.map((r) => (
    <Tip key={r.number} text={r.title}>
      <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo: name, kind: prefix === "PR" ? "pulls" : "issues", number: String(r.number) }}
        className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-white px-2 py-0.5 text-[0.6875rem] font-medium text-slate-700 shadow-sm transition-colors hover:border-blue-400 hover:bg-blue-50 hover:text-blue-700 dark:border-border dark:bg-surface dark:text-text dark:hover:border-accent dark:hover:bg-accent-subtle dark:hover:text-accent-text">
        <Icon i={prefix === "PR" ? icons.pull : icons.issue} className="size-3" />{prefix} #{r.number}
      </Link>
    </Tip>
  ));
};

