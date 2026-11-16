// GitHub shapes served by internal/github, unchanged from v1.

export type GithubRef = {
    number: number;
    title: string;
};

export type GithubIssue = {
    repo: string;
    project_id: string;
    number: number;
    title: string;
    author: string;
    url: string;
    state: string;
    updated_at: string;
    labels: string[];
    pulls: GithubRef[];
    milestone: string;
    /** @description COMPLETED, NOT_PLANNED or REOPENED; empty when unknown. */
    state_reason: string;
    created_at: string;
    /** @description Empty while open. */
    closed_at: string;
    comments: number;
    assignees: string[];
    /** @description Hex color of each label, without #. */
    label_colors: {
        [key: string]: string;
    };
};

export type GithubPull = {
    repo: string;
    project_id: string;
    number: number;
    title: string;
    author: string;
    url: string;
    state: string;
    updated_at: string;
    labels: string[];
    issues: GithubRef[];
    draft: boolean;
    head_ref: string;
    base_ref: string;
    review_decision: string;
    approvals: number;
    approved_by: string[];
    changes_by: string[];
    required_approvals: number | null;
    review_requests: string[];
    ci: string;
    mergeable: string;
    merge_state: string;
    additions: number;
    deletions: number;
    changed_files: number;
    created_at: string;
    /** @description Empty while open. */
    closed_at: string;
    comments: number;
    assignees: string[];
    milestone: string;
    /** @description Hex color of each label, without #. */
    label_colors: {
        [key: string]: string;
    };
};

export type GithubOverview = {
    issues: GithubIssue[];
    pulls: GithubPull[];
    /** @description Empty until the first refresh finishes. */
    fetched_at: string;
    /** @description A background refresh is running; poll again to get each repository as it finishes. */
    refreshing: boolean;
    /** @description GitHub login of the token owner; empty until the first repository answers. */
    viewer: string;
};

export type GithubClosed = {
    issues: GithubIssue[];
    pulls: GithubPull[];
    /** @description Pass it back to read the next page; empty when there is nothing more. */
    cursor: string;
};

export type GithubEvent = {
    /** @description Set on comments and reviews, which can be edited. */
    id: string;
    type: string;
    actor: string;
    at: string;
    body: string;
    state: string;
    detail: string;
};

export type GithubDetail = {
    kind: string;
    body: string;
    issue: GithubIssue | null;
    pull: GithubPull | null;
    timeline: GithubEvent[];
    threads: GithubThread[];
    /** @description GitHub login Dory acts as. */
    viewer: string;
    assignees: GithubUser[];
    /** @description Users that can be assigned or asked for a review. */
    candidates: GithubUser[];
    /** @description Latest issues and pull requests of the repository, to reference with #. */
    links: GithubLink[];
    /** @description Open milestones of the repository; empty for pull requests. */
    milestones: GithubMilestone[];
    /** @description Parent issue of an issue, if any. */
    parent: GithubSubIssue | null;
    /** @description Sub-issues of an issue; empty for pull requests. */
    sub_issues: GithubSubIssue[];
};

export type GithubMilestone = {
    id: string;
    title: string;
};

export type GithubSubIssue = {
    repo: string;
    number: number;
    title: string;
    state: string;
};

export type GithubLink = {
    number: number;
    title: string;
    state: string;
    pull: boolean;
};

export type GithubUser = {
    id: string;
    login: string;
};

export type GithubThreadComment = {
    id: string;
    author: string;
    at: string;
    body: string;
    url: string;
};

export type GithubThread = {
    id: string;
    path: string;
    line: number;
    resolved: boolean;
    outdated: boolean;
    resolved_by: string;
    comments: GithubThreadComment[];
};

export type GithubAction = {
    type: string;
    /** @description New title for edit_pull and edit_issue. */
    title?: string;
    body?: string;
    /** @description Merge method for merge (MERGE, SQUASH, REBASE) or close reason for close_issue (COMPLETED, NOT_PLANNED). */
    method?: string;
    thread_id?: string;
    path?: string;
    line?: number;
    /** @enum {string} */
    side?: "LEFT" | "RIGHT";
    start_line?: number;
    /** @enum {string} */
    start_side?: "LEFT" | "RIGHT";
    /** @description Comment or review edited by edit_comment, edit_review and edit_thread_comment. */
    comment_id?: string;
    /** @description User node ids for request_review, assign and unassign. */
    user_ids?: string[];
    /** @description Milestone node id for set_milestone; empty removes the milestone. */
    milestone_id?: string;
};
