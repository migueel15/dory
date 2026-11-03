package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalid reports a kind or action that does not exist.
var ErrInvalid = errors.New("github: invalid request")

// Event is one entry of the conversation and activity of an issue or pull request.
type Event struct {
	// ID is set on comments and reviews, the events that can be edited.
	ID     string `json:"id"`
	Type   string `json:"type"`
	Actor  string `json:"actor"`
	At     string `json:"at"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// Detail is the working view of one issue or pull request.
type Detail struct {
	Kind     string   `json:"kind"`
	Body     string   `json:"body"`
	Issue    *Issue   `json:"issue"`
	Pull     *Pull    `json:"pull"`
	Timeline []Event  `json:"timeline"`
	Threads  []Thread `json:"threads"`
	// Viewer is the GitHub account Dory acts as. Assignees and Candidates
	// (the users that can be assigned or asked for a review) are only set
	// for pull requests.
	Viewer     string `json:"viewer"`
	Assignees  []User `json:"assignees"`
	Candidates []User `json:"candidates"`
	// Links are the repository's latest issues and pull requests, to
	// reference them with "#" while writing.
	Links []Link `json:"links"`
	// Parent and SubIssues relate an issue to its parent and child issues.
	Parent    *SubIssue  `json:"parent"`
	SubIssues []SubIssue `json:"sub_issues"`
}

// SubIssue is an issue linked as parent or child, possibly in another repository.
type SubIssue struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

const subIssueFields = `number title state repository { nameWithOwner }`

type wireSubIssue struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	Repository struct {
		Name string `json:"nameWithOwner"`
	} `json:"repository"`
}

func (w wireSubIssue) sub() SubIssue {
	return SubIssue{Repo: w.Repository.Name, Number: w.Number, Title: w.Title, State: w.State}
}

// Link is an issue or pull request that a message can reference.
type Link struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Pull   bool   `json:"pull"`
}

const linksFields = `issues(last: 50) { nodes { number title state } } pullRequests(last: 50) { nodes { number title state } }`

type wireLinks struct {
	Issues struct{ Nodes []Link } `json:"issues"`
	Pulls  struct{ Nodes []Link } `json:"pullRequests"`
}

// links merges issues and pull requests, newest number first.
func (w wireLinks) links() []Link {
	out := make([]Link, 0, len(w.Issues.Nodes)+len(w.Pulls.Nodes))
	out = append(out, w.Issues.Nodes...)
	for _, p := range w.Pulls.Nodes {
		p.Pull = true
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Link) int { return b.Number - a.Number })
	return out
}

// User is a GitHub account; ID is the node id that mutations need.
type User struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// ThreadComment is one message inside a review thread.
type ThreadComment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	At     string `json:"at"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// Thread is a review conversation on a pull request that can be resolved.
type Thread struct {
	ID         string          `json:"id"`
	Path       string          `json:"path"`
	Line       int             `json:"line"`
	Resolved   bool            `json:"resolved"`
	Outdated   bool            `json:"outdated"`
	ResolvedBy string          `json:"resolved_by"`
	Comments   []ThreadComment `json:"comments"`
}

// Action is a change requested by the user. Body is the comment or review text
// and Method the merge method (MERGE, SQUASH or REBASE).
type Action struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Method   string `json:"method"`
	ThreadID string `json:"thread_id"`
	// Path, Line and Side (LEFT or RIGHT) place a comment on the diff;
	// StartLine and StartSide are set for a range of lines.
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	StartSide string `json:"start_side"`
	// UserIDs are the node ids for request_review, assign and unassign.
	UserIDs []string `json:"user_ids"`
	// CommentID is the comment or review edited by edit_comment,
	// edit_review and edit_thread_comment.
	CommentID string `json:"comment_id"`
}

// Issue timelines reject pull request fragments, so only shared events go here.
const issueTimelineFields = `__typename
  ... on IssueComment { id commentAuthor: author { login } createdAt body }
  ... on ClosedEvent { actor { login } createdAt }
  ... on ReopenedEvent { actor { login } createdAt }
  ... on LabeledEvent { actor { login } createdAt label { name } }
  ... on AssignedEvent { actor { login } createdAt }
  ... on CrossReferencedEvent { actor { login } createdAt source { ... on Issue { number title } ... on PullRequest { number title } } }`

const pullTimelineFields = issueTimelineFields + `
  ... on PullRequestReview { id reviewAuthor: author { login } createdAt body state }
  ... on PullRequestCommit { commit { committedDate messageHeadline oid author { user { login } name } } }
  ... on MergedEvent { actor { login } createdAt }
  ... on ReadyForReviewEvent { actor { login } createdAt }
  ... on ConvertToDraftEvent { actor { login } createdAt }
  ... on ReviewRequestedEvent { actor { login } createdAt }`

const pullDetailQuery = `query($owner: String!, $name: String!, $number: Int!) { viewer { login } repository(owner: $owner, name: $name) {
  assignableUsers(first: 100) { nodes { id login } } ` + linksFields + `
  pullRequest(number: $number) {
  body ` + pullFields + ` assignees(first: 20) { nodes { id login } } timelineItems(last: 100, itemTypes: [ISSUE_COMMENT, PULL_REQUEST_REVIEW, PULL_REQUEST_COMMIT, CLOSED_EVENT, REOPENED_EVENT, MERGED_EVENT, LABELED_EVENT, ASSIGNED_EVENT, READY_FOR_REVIEW_EVENT, CONVERT_TO_DRAFT_EVENT, REVIEW_REQUESTED_EVENT, CROSS_REFERENCED_EVENT]) { nodes { ` + pullTimelineFields + ` } }
  reviewThreads(first: 100) { nodes { id path line originalLine isResolved isOutdated resolvedBy { login }
    comments(first: 100) { nodes { id author { login } createdAt body url } } } }
} } }`

type wireThread struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Line         *int   `json:"line"`
	OriginalLine *int   `json:"originalLine"`
	IsResolved   bool   `json:"isResolved"`
	IsOutdated   bool   `json:"isOutdated"`
	ResolvedBy   *login `json:"resolvedBy"`
	Comments     struct {
		Nodes []struct {
			ID        string `json:"id"`
			Author    *login `json:"author"`
			CreatedAt string `json:"createdAt"`
			Body      string `json:"body"`
			URL       string `json:"url"`
		} `json:"nodes"`
	} `json:"comments"`
}

func threads(ws []wireThread) []Thread {
	out := make([]Thread, 0, len(ws))
	for _, w := range ws {
		t := Thread{ID: w.ID, Path: w.Path, Resolved: w.IsResolved, Outdated: w.IsOutdated, Comments: []ThreadComment{}}
		if w.Line != nil {
			t.Line = *w.Line
		} else if w.OriginalLine != nil {
			t.Line = *w.OriginalLine
		}
		if w.ResolvedBy != nil {
			t.ResolvedBy = w.ResolvedBy.Login
		}
		for _, c := range w.Comments.Nodes {
			tc := ThreadComment{ID: c.ID, At: c.CreatedAt, Body: c.Body, URL: c.URL}
			if c.Author != nil {
				tc.Author = c.Author.Login
			}
			t.Comments = append(t.Comments, tc)
		}
		out = append(out, t)
	}
	return out
}

const issueDetailQuery = `query($owner: String!, $name: String!, $number: Int!) { viewer { login } repository(owner: $owner, name: $name) {
  assignableUsers(first: 100) { nodes { id login } } ` + linksFields + ` issue(number: $number) {
  id number title url state stateReason updatedAt body author { login } labels(first: 10) { nodes { name } } milestone { title } ` + metaFields + `
  assignees(first: 20) { nodes { id login } }
  parent { ` + subIssueFields + ` } subIssues(first: 50) { nodes { ` + subIssueFields + ` } }
  closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { number title } }
  timelineItems(last: 100, itemTypes: [ISSUE_COMMENT, CLOSED_EVENT, REOPENED_EVENT, LABELED_EVENT, ASSIGNED_EVENT, CROSS_REFERENCED_EVENT]) { nodes { ` + issueTimelineFields + ` } }
} } }`

type wireEvent struct {
	ID            string `json:"id"`
	Type          string `json:"__typename"`
	CommentAuthor *login `json:"commentAuthor"`
	ReviewAuthor  *login `json:"reviewAuthor"`
	Actor         *login `json:"actor"`
	CreatedAt     string `json:"createdAt"`
	Body          string `json:"body"`
	State         string `json:"state"`
	Label         *struct {
		Name string `json:"name"`
	} `json:"label"`
	Source *Ref `json:"source"`
	Commit *struct {
		Date     string `json:"committedDate"`
		Headline string `json:"messageHeadline"`
		Oid      string `json:"oid"`
		Author   struct {
			User *login `json:"user"`
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
}

func (w wireEvent) event() Event {
	e := Event{ID: w.ID, Type: w.Type, At: w.CreatedAt, Body: w.Body, State: w.State}
	switch {
	case w.CommentAuthor != nil:
		e.Actor = w.CommentAuthor.Login
	case w.ReviewAuthor != nil:
		e.Actor = w.ReviewAuthor.Login
	case w.Actor != nil:
		e.Actor = w.Actor.Login
	}
	if w.Label != nil {
		e.Detail = w.Label.Name
	}
	if w.Source != nil && w.Source.Number != 0 {
		e.Detail = fmt.Sprintf("#%d %s", w.Source.Number, w.Source.Title)
	}
	if c := w.Commit; c != nil {
		e.At, e.Actor, e.Detail = c.Date, c.Author.Name, c.Oid[:7]+" "+c.Headline
		if c.Author.User != nil {
			e.Actor = c.Author.User.Login
		}
	}
	return e
}

func (c *Client) checkRepo(ctx context.Context, repo string) (string, error) {
	if c.Token == "" {
		return "", ErrUnconfigured
	}
	repos, err := c.repos(ctx)
	if err != nil {
		return "", err
	}
	project, ok := repos[repo]
	if !ok {
		return "", ErrUnknownRepo
	}
	return project, nil
}

type wireNodes struct {
	Nodes []wireEvent `json:"nodes"`
}

func timeline(n wireNodes) []Event {
	out := make([]Event, 0, len(n.Nodes))
	for _, w := range n.Nodes {
		out = append(out, w.event())
	}
	return out
}

// Detail reads one issue ("issues") or pull request ("pulls") with its timeline.
func (c *Client) Detail(ctx context.Context, repo, kind string, number int) (*Detail, error) {
	project, err := c.checkRepo(ctx, repo)
	if err != nil {
		return nil, err
	}
	owner, short, _ := strings.Cut(repo, "/")
	vars := map[string]any{"owner": owner, "name": short, "number": number}
	switch kind {
	case "pulls":
		var data struct {
			Viewer     login `json:"viewer"`
			Repository struct {
				wireLinks
				Assignable  struct{ Nodes []User } `json:"assignableUsers"`
				PullRequest *struct {
					wirePull
					Assignees struct{ Nodes []User } `json:"assignees"`
					Body      string                 `json:"body"`
					Timeline  wireNodes              `json:"timelineItems"`
					Threads   struct {
						Nodes []wireThread `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := c.graphql(ctx, pullDetailQuery, vars, &data); err != nil {
			return nil, err
		}
		w := data.Repository.PullRequest
		if w == nil {
			return nil, ErrUnknownRepo
		}
		p := w.pull(repo, project)
		c.fillRequired(ctx, &p)
		return &Detail{Kind: kind, Body: w.Body, Pull: &p, Timeline: timeline(w.Timeline), Threads: threads(w.Threads.Nodes),
			Viewer: data.Viewer.Login, Assignees: nonNil(w.Assignees.Nodes), Candidates: nonNil(data.Repository.Assignable.Nodes), Links: data.Repository.links(), SubIssues: []SubIssue{}}, nil
	case "issues":
		var data struct {
			Viewer     login `json:"viewer"`
			Repository struct {
				wireLinks
				Assignable struct{ Nodes []User } `json:"assignableUsers"`
				Issue      *struct {
					wireIssue
					Assignees struct{ Nodes []User } `json:"assignees"`
					Body      string                 `json:"body"`
					Timeline  wireNodes              `json:"timelineItems"`
					Parent    *wireSubIssue          `json:"parent"`
					SubIssues struct {
						Nodes []wireSubIssue `json:"nodes"`
					} `json:"subIssues"`
				} `json:"issue"`
			} `json:"repository"`
		}
		if err := c.graphql(ctx, issueDetailQuery, vars, &data); err != nil {
			return nil, err
		}
		w := data.Repository.Issue
		if w == nil {
			return nil, ErrUnknownRepo
		}
		i := w.issue(repo, project)
		d := &Detail{Kind: kind, Body: w.Body, Issue: &i, Timeline: timeline(w.Timeline), Threads: []Thread{},
			Viewer: data.Viewer.Login, Assignees: nonNil(w.Assignees.Nodes), Candidates: nonNil(data.Repository.Assignable.Nodes), Links: data.Repository.links(),
			SubIssues: make([]SubIssue, 0, len(w.SubIssues.Nodes))}
		if w.Parent != nil {
			p := w.Parent.sub()
			d.Parent = &p
		}
		for _, n := range w.SubIssues.Nodes {
			d.SubIssues = append(d.SubIssues, n.sub())
		}
		return d, nil
	}
	return nil, ErrInvalid
}

// mutations maps each action to its GraphQL mutation; id is always the subject.
var mutations = map[string]struct {
	kinds, query string
	input        func(a Action, id string) map[string]any
}{
	"comment": {"issues pulls", `mutation($input: AddCommentInput!) { addComment(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"subjectId": id, "body": a.Body}
	}},
	"approve":         {"pulls", reviewMutation, review("APPROVE")},
	"request_changes": {"pulls", reviewMutation, review("REQUEST_CHANGES")},
	"merge": {"pulls", `mutation($input: MergePullRequestInput!) { mergePullRequest(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "mergeMethod": a.Method}
	}},
	"ready":       {"pulls", `mutation($input: MarkPullRequestReadyForReviewInput!) { markPullRequestReadyForReview(input: $input) { clientMutationId } }`, pullInput},
	"draft":       {"pulls", `mutation($input: ConvertPullRequestToDraftInput!) { convertPullRequestToDraft(input: $input) { clientMutationId } }`, pullInput},
	"close_pull":  {"pulls", `mutation($input: ClosePullRequestInput!) { closePullRequest(input: $input) { clientMutationId } }`, pullInput},
	"reopen_pull": {"pulls", `mutation($input: ReopenPullRequestInput!) { reopenPullRequest(input: $input) { clientMutationId } }`, pullInput},
	"reply_thread": {"pulls", `mutation($input: AddPullRequestReviewThreadReplyInput!) { addPullRequestReviewThreadReply(input: $input) { clientMutationId } }`, func(a Action, _ string) map[string]any {
		return map[string]any{"pullRequestReviewThreadId": a.ThreadID, "body": a.Body}
	}},
	"comment_lines": {"pulls", `mutation($input: AddPullRequestReviewThreadInput!) { addPullRequestReviewThread(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		in := map[string]any{"pullRequestId": id, "body": a.Body, "path": a.Path, "line": a.Line, "side": a.Side, "subjectType": "LINE"}
		if a.StartLine != 0 && (a.StartLine != a.Line || a.StartSide != a.Side) {
			in["startLine"], in["startSide"] = a.StartLine, a.StartSide
		}
		return in
	}},
	"comment_file": {"pulls", `mutation($input: AddPullRequestReviewThreadInput!) { addPullRequestReviewThread(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "body": a.Body, "path": a.Path, "subjectType": "FILE"}
	}},
	"edit_pull": {"pulls", `mutation($input: UpdatePullRequestInput!) { updatePullRequest(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "title": a.Title, "body": a.Body}
	}},
	"edit_comment": {"issues pulls", `mutation($input: UpdateIssueCommentInput!) { updateIssueComment(input: $input) { clientMutationId } }`, func(a Action, _ string) map[string]any {
		return map[string]any{"id": a.CommentID, "body": a.Body}
	}},
	"edit_review": {"pulls", `mutation($input: UpdatePullRequestReviewInput!) { updatePullRequestReview(input: $input) { clientMutationId } }`, func(a Action, _ string) map[string]any {
		return map[string]any{"pullRequestReviewId": a.CommentID, "body": a.Body}
	}},
	"edit_thread_comment": {"pulls", `mutation($input: UpdatePullRequestReviewCommentInput!) { updatePullRequestReviewComment(input: $input) { clientMutationId } }`, func(a Action, _ string) map[string]any {
		return map[string]any{"pullRequestReviewCommentId": a.CommentID, "body": a.Body}
	}},
	"request_review": {"pulls", `mutation($input: RequestReviewsInput!) { requestReviews(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "userIds": a.UserIDs, "union": true}
	}},
	"assign": {"issues pulls", `mutation($input: AddAssigneesToAssignableInput!) { addAssigneesToAssignable(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"assignableId": id, "assigneeIds": a.UserIDs}
	}},
	"unassign": {"issues pulls", `mutation($input: RemoveAssigneesFromAssignableInput!) { removeAssigneesFromAssignable(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"assignableId": id, "assigneeIds": a.UserIDs}
	}},
	"resolve_thread":   {"pulls", `mutation($input: ResolveReviewThreadInput!) { resolveReviewThread(input: $input) { clientMutationId } }`, threadInput},
	"unresolve_thread": {"pulls", `mutation($input: UnresolveReviewThreadInput!) { unresolveReviewThread(input: $input) { clientMutationId } }`, threadInput},
	// close_issue takes the reason in Method: COMPLETED (default) or NOT_PLANNED.
	"close_issue": {"issues", `mutation($input: CloseIssueInput!) { closeIssue(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		in := map[string]any{"issueId": id}
		if a.Method != "" {
			in["stateReason"] = a.Method
		}
		return in
	}},
	"edit_issue": {"issues", `mutation($input: UpdateIssueInput!) { updateIssue(input: $input) { clientMutationId } }`, func(a Action, id string) map[string]any {
		return map[string]any{"id": id, "title": a.Title, "body": a.Body}
	}},
	"reopen_issue": {"issues", `mutation($input: ReopenIssueInput!) { reopenIssue(input: $input) { clientMutationId } }`, issueInput},
}

const reviewMutation = `mutation($input: AddPullRequestReviewInput!) { addPullRequestReview(input: $input) { clientMutationId } }`

func review(event string) func(Action, string) map[string]any {
	return func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "event": event, "body": a.Body}
	}
}
func pullInput(_ Action, id string) map[string]any  { return map[string]any{"pullRequestId": id} }
func threadInput(a Action, _ string) map[string]any { return map[string]any{"threadId": a.ThreadID} }
func issueInput(_ Action, id string) map[string]any { return map[string]any{"issueId": id} }

func validate(kind string, a Action) error {
	m, ok := mutations[a.Type]
	if !ok || !slices.Contains(strings.Fields(m.kinds), kind) {
		return ErrInvalid
	}
	if strings.HasSuffix(a.Type, "_thread") && a.ThreadID == "" {
		return fmt.Errorf("%w: thread_id is required", ErrInvalid)
	}
	sides := []string{"LEFT", "RIGHT"}
	if a.Type == "comment_lines" && (a.Path == "" || a.Line < 1 || !slices.Contains(sides, a.Side) || (a.StartLine != 0 && !slices.Contains(sides, a.StartSide))) {
		return fmt.Errorf("%w: path, line and side are required", ErrInvalid)
	}
	if (a.Type == "request_review" || a.Type == "assign" || a.Type == "unassign") && len(a.UserIDs) == 0 {
		return fmt.Errorf("%w: user_ids is required", ErrInvalid)
	}
	if strings.HasPrefix(a.Type, "edit_") && a.Type != "edit_pull" && a.Type != "edit_issue" && (a.CommentID == "" || strings.TrimSpace(a.Body) == "") {
		return fmt.Errorf("%w: comment_id and text are required", ErrInvalid)
	}
	if (a.Type == "edit_pull" || a.Type == "edit_issue") && strings.TrimSpace(a.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if a.Type == "comment_file" && a.Path == "" {
		return fmt.Errorf("%w: path is required", ErrInvalid)
	}
	if (a.Type == "comment" || a.Type == "request_changes" || a.Type == "reply_thread" || a.Type == "comment_lines" || a.Type == "comment_file") && strings.TrimSpace(a.Body) == "" {
		return fmt.Errorf("%w: text is required", ErrInvalid)
	}
	if a.Type == "close_issue" && a.Method != "" && a.Method != "COMPLETED" && a.Method != "NOT_PLANNED" {
		return fmt.Errorf("%w: unknown close reason", ErrInvalid)
	}
	if a.Type == "merge" && !slices.Contains([]string{"MERGE", "SQUASH", "REBASE"}, a.Method) {
		return fmt.Errorf("%w: unknown merge method", ErrInvalid)
	}
	return nil
}

// Act applies an action on GitHub with the configured account.
func (c *Client) Act(ctx context.Context, repo, kind string, number int, a Action) error {
	if err := validate(kind, a); err != nil {
		return err
	}
	if _, err := c.checkRepo(ctx, repo); err != nil {
		return err
	}
	owner, short, _ := strings.Cut(repo, "/")
	field := map[string]string{"pulls": "pullRequest", "issues": "issue"}[kind]
	var data struct {
		Repository map[string]json.RawMessage `json:"repository"`
	}
	q := fmt.Sprintf(`query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { %s(number: $number) { id } } }`, field)
	if err := c.graphql(ctx, q, map[string]any{"owner": owner, "name": short, "number": number}, &data); err != nil {
		return err
	}
	var node struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data.Repository[field], &node); err != nil || node.ID == "" {
		return ErrUnknownRepo
	}
	m := mutations[a.Type]
	var ignored struct{}
	if err := c.graphql(ctx, m.query, map[string]any{"input": m.input(a, node.ID)}, &ignored); err != nil {
		return err
	}
	c.forget()
	return nil
}

func nonNil(u []User) []User {
	if u == nil {
		return []User{}
	}
	return u
}
