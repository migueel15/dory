import { useState } from "react";
import { type GithubDetail, type GithubPull } from "../../api/client";
import { Select } from "../../components/ui";
import { Icon, IconButton, Menu, MenuItem, Tip, UserPicker, icons, useArm } from "./github-ui";
import { review } from "./detail-optimistic";
import { type Act, canApprove } from "./detail-shared";

// ReviewPicker asks someone for a review; it sits next to the reviewers' avatars.
export function ReviewPicker({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const pull = d.pull!;
  const reviewed = new Set([...pull.approved_by, ...pull.changes_by]);
  const users = d.candidates.filter((u) => u.login !== pull.author && !pull.review_requests.includes(u.login));
  return <UserPicker tip="Pedir revisión" title="Pedir revisión a" users={users} disabled={busy} note={(u) => (reviewed.has(u.login) ? "de nuevo" : undefined)} onPick={(u) => act("request_review", { user_ids: [u.id] })} />;
}

// Assignees shows who owns the issue or pull request and lets Dory's account change it.
export function Assignees({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const assignable = d.candidates.filter((u) => !d.assignees.some((a) => a.id === u.id));
  const me = assignable.find((u) => u.login === d.viewer);
  return (
    <div className="flex items-center gap-2">
      <Tip text="Asignados"><span className="text-slate-400"><Icon i={icons.user} /></span></Tip>
      <span className="flex -space-x-1.5">
        {d.assignees.map((a) => (
          <Tip key={a.id} text={`${a.login} · clic para quitar`}>
            <button type="button" aria-label={`Quitar a ${a.login}`} disabled={busy} onClick={() => act("unassign", { user_ids: [a.id] })} className="group relative rounded-full">
              <img src={`https://github.com/${a.login}.png?size=48`} alt={a.login} className="size-6 rounded-full bg-slate-200 ring-2 ring-white dark:bg-hover dark:ring-surface" />
              <span className="absolute inset-0 grid place-items-center rounded-full bg-red-600/80 text-white opacity-0 transition-opacity group-hover:opacity-100"><Icon i={icons.x} className="size-3" /></span>
            </button>
          </Tip>
        ))}
      </span>
      <UserPicker tip="Asignar" title="Asignar a" users={assignable.filter((u) => u.id !== me?.id)} disabled={busy} onPick={(u) => act("assign", { user_ids: [u.id] })}
        extra={me && ((close) => (
          <MenuItem onClick={() => { close(); act("assign", { user_ids: [me.id] }); }}>
            <span className="grid size-6 place-items-center rounded-full bg-blue-100 text-blue-700 dark:bg-accent-subtle dark:text-accent-text"><Icon i={icons.user} className="size-3.5" /></span>
            <span className="flex-1 font-medium">Asignármela</span>
          </MenuItem>
        ))} />
    </div>
  );
}

// MilestonePicker sets or removes the issue's milestone; a closed one stays listed while it is the current one.
export function MilestonePicker({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const current = d.issue!.milestone;
  const known = d.milestones.find((m) => m.title === current);
  const options = [{ value: "", label: "Sin milestone" }, ...d.milestones.map((m) => ({ value: m.id, label: m.title }))];
  if (current && !known) options.push({ value: "current", label: current });
  const value = known?.id ?? (current ? "current" : "");
  return (
    <span className="inline-flex items-center gap-1.5">
      <Tip text="Milestone"><span className="text-slate-400"><Icon i={icons.flag} /></span></Tip>
      <Select value={value} disabled={busy} options={options} className="min-w-40" onChange={(v) => v !== value && v !== "current" && act("set_milestone", { milestone_id: v })} />
    </span>
  );
}

export const methods = [
  { value: "MERGE", name: "Merge commit", hint: "Conserva todos los commits" },
  { value: "SQUASH", name: "Squash", hint: "Un único commit" },
  { value: "REBASE", name: "Rebase", hint: "Commits sobre la base, sin merge" },
];

// MergeButton is a split button: the main part merges with the chosen method.
export function MergeButton({ pull, act, busy }: { pull: GithubPull; act: Act; busy: boolean }) {
  const [method, setMethod] = useState(methods[0]);
  const [armed, merge] = useArm(() => act("merge", { method: method.value }));
  return (<>
    <Menu align="right" trigger={(open, toggle) => (
      <div className="inline-flex overflow-hidden rounded-lg bg-green-600 text-white shadow-sm">
        <button type="button" disabled={busy} onClick={merge} className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium hover:bg-green-700 disabled:opacity-50">
          <Icon i={icons.merge} /> {method.name}<span aria-hidden={!armed} className={`overflow-hidden whitespace-nowrap transition-all duration-200 ease-out motion-reduce:transition-none ${armed ? "max-w-28 opacity-100" : "max-w-0 opacity-0"}`}>· ¿Confirmar?</span>
        </button>
        <button type="button" aria-label="Elegir método de merge" disabled={busy} onClick={toggle} className={`border-l border-green-500 px-2 hover:bg-green-700 ${open ? "bg-green-700" : ""}`}>
          <Icon i={icons.chevron} />
        </button>
      </div>
    )}>
      {(close) => methods.map((m) => (
        <MenuItem key={m.value} onClick={() => { setMethod(m); close(); }}>
          <span className="grid size-5 place-items-center text-green-600">{m.value === method.value && <Icon i={icons.check} />}</span>
          <span><span className="block font-medium">{m.name}</span><span className="block text-xs text-slate-500">{m.hint}</span></span>
        </MenuItem>
      ))}
    </Menu></>
  );
}

export function PullActions({ pull, me, act, busy, diff, toggleDiff }: { pull: GithubPull; me: string; act: Act; busy: boolean; diff: boolean; toggleDiff: () => void }) {
  const view = <IconButton icon={icons.code} tip={diff ? "Ocultar diff" : "Ver diff"} tone={diff ? "active" : "plain"} onClick={toggleDiff} />;
  if (pull.state !== "OPEN") return <>{view}{pull.state === "CLOSED" && <IconButton icon={icons.reopen} tip="Reabrir" disabled={busy} confirm onClick={() => act("reopen_pull")} />}</>;
  return (
    <>
      {view}
      {pull.draft
        ? <IconButton icon={icons.ready} tip="Marcar como lista para revisión" disabled={busy} confirm onClick={() => act("ready")} />
        : <IconButton icon={icons.draft} tip="Pasar a draft" disabled={busy} confirm onClick={() => act("draft")} />}
      {canApprove(pull, me) && <IconButton icon={icons.check} tip="Aprobar" tone="green" disabled={busy} onClick={() => act("approve")} />}
      <IconButton icon={icons.x} tip="Cerrar sin mergear" tone="red" disabled={busy} confirm onClick={() => act("close_pull")} />
      <span className="ml-auto"><MergeButton pull={pull} act={act} busy={busy} /></span>
    </>
  );
}

// IssueActions closes an issue as completed or not planned, or reopens it.
export function IssueActions({ open, act, busy }: { open: boolean; act: Act; busy: boolean }) {
  if (!open) return <><IconButton icon={icons.reopen} tip="Reabrir issue" disabled={busy} confirm onClick={() => act("reopen_issue")} /></>;
  return (
    <>
      <IconButton icon={icons.ready} tip="Cerrar como completada" tone="green" disabled={busy} confirm onClick={() => act("close_issue", { method: "COMPLETED" })} />
      <IconButton icon={icons.x} tip="Cerrar como no planificada" tone="red" disabled={busy} confirm onClick={() => act("close_issue", { method: "NOT_PLANNED" })} />
    </>
  );
}

