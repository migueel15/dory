package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/apperr"
)

type IssueCreateInput struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
}

type IssueUpdateInput struct {
	Title       *string   `json:"title,omitempty"`
	Body        *string   `json:"body,omitempty"`
	State       *string   `json:"state,omitempty"`
	StateReason *string   `json:"state_reason,omitempty"`
	Labels      *[]string `json:"labels,omitempty"`
	Assignees   *[]string `json:"assignees,omitempty"`
}

type IssueComment struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PullCreateInput struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft,omitempty"`
}

type PullUpdateInput struct {
	Title     *string   `json:"title,omitempty"`
	Body      *string   `json:"body,omitempty"`
	State     *string   `json:"state,omitempty"`
	Draft     *bool     `json:"draft,omitempty"`
	Assignees *[]string `json:"assignees,omitempty"`
	Reviewers *[]string `json:"reviewers,omitempty"`
}

type ReviewCommentInput struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side,omitempty"`
	Body string `json:"body"`
}

type PullReviewInput struct {
	Event    string               `json:"event"` // APPROVE | REQUEST_CHANGES | COMMENT
	Body     string               `json:"body,omitempty"`
	Comments []ReviewCommentInput `json:"comments,omitempty"`
}

type PullMergeInput struct {
	Method       string `json:"merge_method,omitempty"` // merge | squash | rebase
	CommitTitle  string `json:"commit_title,omitempty"`
	DeleteBranch bool   `json:"-"`
}

type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
}

// Viewer returns the authenticated GitHub login, caching it on the client.
func (c *Client) Viewer(ctx context.Context) (string, error) {
	c.mu.Lock()
	v := c.viewer
	c.mu.Unlock()
	if v != "" {
		return v, nil
	}
	_, login := c.Status(ctx)
	if login != "" {
		c.mu.Lock()
		c.viewer = login
		c.mu.Unlock()
	}
	return login, nil
}

func (c *Client) requestJSON(ctx context.Context, method, path string, payload any) ([]byte, error) {
	token := c.token(ctx)
	if token == "" {
		return nil, ErrUnconfigured
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		c.unauthorized(resp.StatusCode)
		return nil, apperr.New(apperr.Unavailable, "github: unauthorized")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

type restIssue struct {
	Number      int    `json:"number"`
	NodeID      string `json:"node_id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
	HTMLURL     string `json:"html_url"`
	Comments    int    `json:"comments"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ClosedAt    string `json:"closed_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Labels []struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

func (ri restIssue) toIssue(repo string) Issue {
	labels := make([]string, 0, len(ri.Labels))
	colors := map[string]string{}
	for _, l := range ri.Labels {
		labels = append(labels, l.Name)
		if l.Color != "" {
			colors[l.Name] = l.Color
		}
	}
	assignees := make([]string, 0, len(ri.Assignees))
	for _, a := range ri.Assignees {
		assignees = append(assignees, a.Login)
	}
	milestone := ""
	if ri.Milestone != nil {
		milestone = ri.Milestone.Title
	}
	return Issue{
		NodeID:      ri.NodeID,
		Repo:        repo,
		Number:      ri.Number,
		Title:       ri.Title,
		Author:      ri.User.Login,
		URL:         ri.HTMLURL,
		State:       strings.ToLower(ri.State),
		StateReason: strings.ToUpper(ri.StateReason),
		UpdatedAt:   ri.UpdatedAt,
		Labels:      labels,
		Meta: Meta{
			CreatedAt:   ri.CreatedAt,
			ClosedAt:    ri.ClosedAt,
			Comments:    ri.Comments,
			Assignees:   assignees,
			Milestone:   milestone,
			LabelColors: colors,
		},
	}
}

func (c *Client) CreateIssue(ctx context.Context, repo string, in IssueCreateInput) (Issue, string, error) {
	raw, err := c.requestJSON(ctx, http.MethodPost, "/repos/"+repo+"/issues", in)
	if err != nil {
		return Issue{}, "", err
	}
	var ri restIssue
	if err := json.Unmarshal(raw, &ri); err != nil {
		return Issue{}, "", err
	}
	return ri.toIssue(repo), ri.Body, nil
}

// GetIssue reads one issue with its body.
func (c *Client) GetIssue(ctx context.Context, repo string, number int) (Issue, string, error) {
	raw, err := c.requestJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", repo, number), nil)
	if err != nil {
		return Issue{}, "", err
	}
	var ri restIssue
	if err := json.Unmarshal(raw, &ri); err != nil {
		return Issue{}, "", err
	}
	return ri.toIssue(repo), ri.Body, nil
}

func (c *Client) UpdateIssue(ctx context.Context, repo string, number int, in IssueUpdateInput) (Issue, string, error) {
	raw, err := c.requestJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", repo, number), in)
	if err != nil {
		return Issue{}, "", err
	}
	var ri restIssue
	if err := json.Unmarshal(raw, &ri); err != nil {
		return Issue{}, "", err
	}
	return ri.toIssue(repo), ri.Body, nil
}

func (c *Client) CommentIssue(ctx context.Context, repo string, number int, body string) (IssueComment, error) {
	raw, err := c.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number), map[string]string{"body": body})
	if err != nil {
		return IssueComment{}, err
	}
	var rc struct {
		ID        int64  `json:"id"`
		Body      string `json:"body"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &rc); err != nil {
		return IssueComment{}, err
	}
	return IssueComment{
		ID:        rc.ID,
		Author:    rc.User.Login,
		Body:      rc.Body,
		URL:       rc.HTMLURL,
		CreatedAt: rc.CreatedAt,
		UpdatedAt: rc.UpdatedAt,
	}, nil
}

func (c *Client) ListIssueComments(ctx context.Context, repo string, number int) ([]IssueComment, error) {
	raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", repo, number), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var list []struct {
		ID        int64  `json:"id"`
		Body      string `json:"body"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]IssueComment, 0, len(list))
	for _, rc := range list {
		out = append(out, IssueComment{
			ID:        rc.ID,
			Author:    rc.User.Login,
			Body:      rc.Body,
			URL:       rc.HTMLURL,
			CreatedAt: rc.CreatedAt,
			UpdatedAt: rc.UpdatedAt,
		})
	}
	return out, nil
}

func (c *Client) CreatePull(ctx context.Context, repo string, in PullCreateInput) (Pull, error) {
	raw, err := c.requestJSON(ctx, http.MethodPost, "/repos/"+repo+"/pulls", in)
	if err != nil {
		return Pull{}, err
	}
	return decodeRestPull(repo, raw)
}

func (c *Client) UpdatePull(ctx context.Context, repo string, number int, in PullUpdateInput) (Pull, error) {
	patch := map[string]any{}
	if in.Title != nil {
		patch["title"] = *in.Title
	}
	if in.Body != nil {
		patch["body"] = *in.Body
	}
	if in.State != nil {
		patch["state"] = *in.State
	}
	var pr Pull
	var nodeID string
	if len(patch) > 0 {
		raw, err := c.requestJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), patch)
		if err != nil {
			return Pull{}, err
		}
		var meta struct {
			NodeID string `json:"node_id"`
		}
		_ = json.Unmarshal(raw, &meta)
		nodeID = meta.NodeID
		if pr, err = decodeRestPull(repo, raw); err != nil {
			return Pull{}, err
		}
	}
	if in.Assignees != nil {
		if _, err := c.requestJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", repo, number), map[string]any{"assignees": *in.Assignees}); err != nil {
			return Pull{}, err
		}
	}
	if in.Reviewers != nil && len(*in.Reviewers) > 0 {
		if _, err := c.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls/%d/requested_reviewers", repo, number), map[string]any{"reviewers": *in.Reviewers}); err != nil {
			return Pull{}, err
		}
	}
	if in.Draft != nil {
		if nodeID == "" {
			raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), "application/vnd.github+json")
			if err != nil {
				return Pull{}, err
			}
			var meta struct {
				NodeID string `json:"node_id"`
			}
			_ = json.Unmarshal(raw, &meta)
			nodeID = meta.NodeID
			pr, _ = decodeRestPull(repo, raw)
		}
		mutation := `mutation($id: ID!) { markPullRequestReadyForReview(input: {pullRequestId: $id}) { pullRequest { id isDraft } } }`
		if *in.Draft {
			mutation = `mutation($id: ID!) { convertPullRequestToDraft(input: {pullRequestId: $id}) { pullRequest { id isDraft } } }`
		}
		if _, err := c.requestJSON(ctx, http.MethodPost, "/graphql", map[string]any{
			"query":     mutation,
			"variables": map[string]any{"id": nodeID},
		}); err != nil {
			return Pull{}, err
		}
		pr.Draft = *in.Draft
	}
	return pr, nil
}

// ReviewPull submits a review on a pull request. APPROVE and REQUEST_CHANGES
// are rejected when the authenticated viewer is the PR author.
func (c *Client) ReviewPull(ctx context.Context, repo string, number int, in PullReviewInput) error {
	event := strings.ToUpper(strings.TrimSpace(in.Event))
	if event == "" {
		event = "COMMENT"
	}
	if event == "APPROVE" || event == "REQUEST_CHANGES" {
		viewer, _ := c.Viewer(ctx)
		if viewer != "" {
			raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), "application/vnd.github+json")
			if err == nil {
				var meta struct {
					User struct {
						Login string `json:"login"`
					} `json:"user"`
				}
				_ = json.Unmarshal(raw, &meta)
				if strings.EqualFold(meta.User.Login, viewer) {
					return apperr.New(apperr.Invalid, fmt.Sprintf("cannot %s your own pull request #%d (COMMENT is allowed)", event, number))
				}
			}
		}
	}
	payload := map[string]any{
		"event": event,
		"body":  in.Body,
	}
	if len(in.Comments) > 0 {
		payload["comments"] = in.Comments
	}
	_, err := c.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number), payload)
	return err
}

func (c *Client) MergePull(ctx context.Context, repo string, number int, in PullMergeInput) error {
	payload := map[string]any{}
	if in.Method != "" {
		payload["merge_method"] = in.Method
	}
	if in.CommitTitle != "" {
		payload["commit_title"] = in.CommitTitle
	}
	if _, err := c.requestJSON(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/pulls/%d/merge", repo, number), payload); err != nil {
		return err
	}
	if in.DeleteBranch {
		raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), "application/vnd.github+json")
		if err == nil {
			var meta struct {
				Head struct {
					Ref string `json:"ref"`
				} `json:"head"`
			}
			_ = json.Unmarshal(raw, &meta)
			if meta.Head.Ref != "" {
				_, _ = c.requestJSON(ctx, http.MethodDelete, fmt.Sprintf("/repos/%s/git/refs/heads/%s", repo, meta.Head.Ref), nil)
			}
		}
	}
	return nil
}

func (c *Client) PullChecks(ctx context.Context, repo string, number int) ([]CheckRun, error) {
	raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var pr struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.Unmarshal(raw, &pr); err != nil || pr.Head.SHA == "" {
		return nil, err
	}
	checksRaw, err := c.get(ctx, fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=100", repo, pr.Head.SHA), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal(checksRaw, &resp); err != nil {
		return nil, err
	}
	out := make([]CheckRun, 0, len(resp.CheckRuns))
	for _, cr := range resp.CheckRuns {
		out = append(out, CheckRun{
			Name:       cr.Name,
			Status:     cr.Status,
			Conclusion: cr.Conclusion,
			URL:        cr.HTMLURL,
		})
	}
	return out, nil
}

func decodeRestPull(repo string, raw []byte) (Pull, error) {
	var rp struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Draft     bool   `json:"draft"`
		HTMLURL   string `json:"html_url"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.Unmarshal(raw, &rp); err != nil {
		return Pull{}, err
	}
	return Pull{
		Repo:      repo,
		Number:    rp.Number,
		Title:     rp.Title,
		Author:    rp.User.Login,
		URL:       rp.HTMLURL,
		State:     strings.ToLower(rp.State),
		Draft:     rp.Draft,
		HeadRef:   rp.Head.Ref,
		BaseRef:   rp.Base.Ref,
		UpdatedAt: rp.UpdatedAt,
	}, nil
}
