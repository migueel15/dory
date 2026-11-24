import { type GithubPull } from "../../api/client";

export const labels: Record<string, string> = {
  IssueComment: "comentó", PullRequestReview: "revisó", PullRequestCommit: "hizo un commit", ClosedEvent: "cerró", ReopenedEvent: "reabrió", MergedEvent: "mergeó", LabeledEvent: "añadió la etiqueta", AssignedEvent: "asignó", ReadyForReviewEvent: "marcó como lista", ConvertToDraftEvent: "pasó a draft", ReviewRequestedEvent: "pidió revisión", CrossReferencedEvent: "referenció desde",
};
export const reviewStates: Record<string, string> = { APPROVED: "aprobó", CHANGES_REQUESTED: "pidió cambios", COMMENTED: "comentó en la revisión", DISMISSED: "revisión descartada" };

// canApprove is false for the author (GitHub rejects it) and for whoever
// already approved; requesting changes again brings the button back.
export const canApprove = (pull: GithubPull, me: string) => pull.author !== me && !pull.approved_by.includes(me);

export type Act = (type: string, extra?: { method?: string; user_ids?: string[]; title?: string; body?: string; milestone_id?: string }) => void;

