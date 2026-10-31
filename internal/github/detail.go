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
	Type   string `json:"type"`
	Actor  string `json:"actor"`
	At     string `json:"at"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// Detail is the working view of one issue or pull request.
type Detail struct {
	Kind     string  `json:"kind"`
	Body     string  `json:"body"`
	Issue    *Issue  `json:"issue"`
	Pull     *Pull   `json:"pull"`
	Timeline []Event `json:"timeline"`
}

// Action is a change requested by the user. Body is the comment or review text
// and Method the merge method (MERGE, SQUASH or REBASE).
type Action struct {
	Type   string `json:"type"`
	Body   string `json:"body"`
	Method string `json:"method"`
}

// Issue timelines reject pull request fragments, so only shared events go here.
const issueTimelineFields = `__typename
  ... on IssueComment { commentAuthor: author { login } createdAt body }
  ... on ClosedEvent { actor { login } createdAt }
  ... on ReopenedEvent { actor { login } createdAt }
  ... on LabeledEvent { actor { login } createdAt label { name } }
  ... on AssignedEvent { actor { login } createdAt }
  ... on CrossReferencedEvent { actor { login } createdAt source { ... on Issue { number title } ... on PullRequest { number title } } }`

const pullTimelineFields = issueTimelineFields + `
  ... on PullRequestReview { reviewAuthor: author { login } createdAt body state }
  ... on PullRequestCommit { commit { committedDate messageHeadline oid author { user { login } name } } }
  ... on MergedEvent { actor { login } createdAt }
  ... on ReadyForReviewEvent { actor { login } createdAt }
  ... on ConvertToDraftEvent { actor { login } createdAt }
  ... on ReviewRequestedEvent { actor { login } createdAt }`

const pullDetailQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { pullRequest(number: $number) {
  body ` + pullFields + ` timelineItems(last: 100, itemTypes: [ISSUE_COMMENT, PULL_REQUEST_REVIEW, PULL_REQUEST_COMMIT, CLOSED_EVENT, REOPENED_EVENT, MERGED_EVENT, LABELED_EVENT, ASSIGNED_EVENT, READY_FOR_REVIEW_EVENT, CONVERT_TO_DRAFT_EVENT, REVIEW_REQUESTED_EVENT, CROSS_REFERENCED_EVENT]) { nodes { ` + pullTimelineFields + ` } }
} } }`

const issueDetailQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { issue(number: $number) {
  id number title url state updatedAt body author { login } labels(first: 10) { nodes { name } }
  closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { number title } }
  timelineItems(last: 100, itemTypes: [ISSUE_COMMENT, CLOSED_EVENT, REOPENED_EVENT, LABELED_EVENT, ASSIGNED_EVENT, CROSS_REFERENCED_EVENT]) { nodes { ` + issueTimelineFields + ` } }
} } }`

type wireEvent struct {
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
	e := Event{Type: w.Type, At: w.CreatedAt, Body: w.Body, State: w.State}
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
			Repository struct {
				PullRequest *struct {
					wirePull
					Body     string    `json:"body"`
					Timeline wireNodes `json:"timelineItems"`
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
		return &Detail{Kind: kind, Body: w.Body, Pull: &p, Timeline: timeline(w.Timeline)}, nil
	case "issues":
		var data struct {
			Repository struct {
				Issue *struct {
					wireIssue
					Body     string    `json:"body"`
					Timeline wireNodes `json:"timelineItems"`
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
		i := Issue{Repo: repo, ProjectID: project, Number: w.Number, Title: w.Title, Author: name(w.Author), URL: w.URL, State: w.State, UpdatedAt: w.UpdatedAt, Labels: w.Labels.names(), Pulls: w.Closed.Nodes}
		return &Detail{Kind: kind, Body: w.Body, Issue: &i, Timeline: timeline(w.Timeline)}, nil
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
	"ready":        {"pulls", `mutation($input: MarkPullRequestReadyForReviewInput!) { markPullRequestReadyForReview(input: $input) { clientMutationId } }`, pullInput},
	"draft":        {"pulls", `mutation($input: ConvertPullRequestToDraftInput!) { convertPullRequestToDraft(input: $input) { clientMutationId } }`, pullInput},
	"close_pull":   {"pulls", `mutation($input: ClosePullRequestInput!) { closePullRequest(input: $input) { clientMutationId } }`, pullInput},
	"reopen_pull":  {"pulls", `mutation($input: ReopenPullRequestInput!) { reopenPullRequest(input: $input) { clientMutationId } }`, pullInput},
	"close_issue":  {"issues", `mutation($input: CloseIssueInput!) { closeIssue(input: $input) { clientMutationId } }`, issueInput},
	"reopen_issue": {"issues", `mutation($input: ReopenIssueInput!) { reopenIssue(input: $input) { clientMutationId } }`, issueInput},
}

const reviewMutation = `mutation($input: AddPullRequestReviewInput!) { addPullRequestReview(input: $input) { clientMutationId } }`

func review(event string) func(Action, string) map[string]any {
	return func(a Action, id string) map[string]any {
		return map[string]any{"pullRequestId": id, "event": event, "body": a.Body}
	}
}
func pullInput(_ Action, id string) map[string]any  { return map[string]any{"pullRequestId": id} }
func issueInput(_ Action, id string) map[string]any { return map[string]any{"issueId": id} }

func validate(kind string, a Action) error {
	m, ok := mutations[a.Type]
	if !ok || !slices.Contains(strings.Fields(m.kinds), kind) {
		return ErrInvalid
	}
	if (a.Type == "comment" || a.Type == "request_changes") && strings.TrimSpace(a.Body) == "" {
		return fmt.Errorf("%w: text is required", ErrInvalid)
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
