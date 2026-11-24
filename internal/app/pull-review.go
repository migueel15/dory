package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/mesh/peerapi"
)

// PullReviews holds the review aids of a pull request: files marked as
// reviewed on this node and the side chat with an agent (requirement RF-25).
type PullReviews struct {
	Store   *data.Store
	Threads *Threads
}

// ReviewMark is a file of a pull request marked as reviewed at one patch.
type ReviewMark struct {
	Path       string `json:"path"`
	PatchHash  string `json:"patch_hash"`
	ReviewedAt string `json:"reviewed_at"`
}

// Marks lists the files of a pull request marked as reviewed.
func (p *PullReviews) Marks(ctx context.Context, repo string, n int) ([]ReviewMark, error) {
	rows, err := p.Store.LocalDB().QueryContext(ctx, `SELECT path, patch_hash, reviewed_at FROM pr_file_reviews WHERE repo = ? AND number = ?`, repo, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReviewMark{}
	for rows.Next() {
		var m ReviewMark
		if err = rows.Scan(&m.Path, &m.PatchHash, &m.ReviewedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Mark records a file as reviewed at the given patch.
func (p *PullReviews) Mark(ctx context.Context, repo string, n int, path, patchHash string) error {
	_, err := p.Store.LocalDB().ExecContext(ctx, `INSERT INTO pr_file_reviews(repo, number, path, patch_hash, reviewed_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo, number, path) DO UPDATE SET patch_hash = excluded.patch_hash, reviewed_at = excluded.reviewed_at`,
		repo, n, path, patchHash, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Unmark forgets that a file was reviewed.
func (p *PullReviews) Unmark(ctx context.Context, repo string, n int, path string) error {
	_, err := p.Store.LocalDB().ExecContext(ctx, `DELETE FROM pr_file_reviews WHERE repo = ? AND number = ? AND path = ?`, repo, n, path)
	return err
}

// PullQuestion is a question about a pull request, optionally about some of
// its code. Launch configures the review thread when there is none yet.
type PullQuestion struct {
	Path     string `json:"path"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Snippet  string `json:"snippet"`
	Question string `json:"question"`
	Launch   *struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		RuntimeMode string `json:"runtime_mode"`
	} `json:"launch"`
}

// Ask sends a question about a pull request to its review thread, creating
// the thread on the first question.
func (p *PullReviews) Ask(ctx context.Context, repo string, n int, q PullQuestion) (data.Thread, error) {
	t, err := p.Thread(ctx, repo, n)
	if err != nil {
		return data.Thread{}, err
	}
	var first string
	if t == nil {
		if q.Launch == nil {
			return data.Thread{}, apperr.New(apperr.NoThread, "the pull request has no review thread yet: send launch")
		}
		first = fmt.Sprintf("Revisas la pull request #%d de %s. Léela con `gh pr view %d --repo %s` y `gh pr diff %d --repo %s`. No modifiques nada salvo que te lo pida. Responde a mis preguntas sobre ella.\n\n", n, repo, n, repo, n, repo)
	}
	var b strings.Builder
	b.WriteString(first)
	attrs := map[string]string{"repo": repo, "pr": fmt.Sprint(n)}
	if q.Path != "" {
		attrs["path"] = q.Path
		if q.Start > 0 {
			attrs["start"], attrs["end"] = fmt.Sprint(q.Start), fmt.Sprint(q.End)
			fmt.Fprintf(&b, "Sobre `%s` líneas %d-%d:\n", q.Path, q.Start, q.End)
		} else {
			fmt.Fprintf(&b, "Sobre `%s`:\n", q.Path)
		}
		if q.Snippet != "" {
			fmt.Fprintf(&b, "```\n%s\n```\n", q.Snippet)
		}
	}
	// The first message and any question about some code carry their
	// context in a block; a plain follow-up question goes as typed.
	text := q.Question
	if context := b.String(); strings.TrimSpace(context) != "" {
		kind := "pr-review"
		if q.Path != "" {
			kind = "pr-question"
		}
		text = AutoMessage(kind, attrs, context, q.Question)
	}
	if t == nil {
		project, repository := p.repository(ctx, repo)
		worktree := false
		return p.Threads.Create(ctx, CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{
			ProjectID: project, RepositoryID: repository, Provider: q.Launch.Provider, Model: q.Launch.Model,
			Effort: q.Launch.Effort, RuntimeMode: q.Launch.RuntimeMode, Title: pullTitle(repo, n), Text: text, Worktree: &worktree,
		}})
	}
	if strings.TrimSpace(text) != "" {
		if err := p.Threads.Runtime.Send(ctx, t.ID, text); err != nil {
			return data.Thread{}, err
		}
	}
	return *t, nil
}

// pullTitle names the review thread of a pull request; it is also how the
// thread is found again.
func pullTitle(repo string, n int) string { return fmt.Sprintf("Revisión de %s#%d", repo, n) }

// Thread finds the open review thread of a pull request.
func (p *PullReviews) Thread(ctx context.Context, repo string, n int) (*data.Thread, error) {
	threads, err := p.Store.Threads().List(ctx, data.Filter{Fields: map[string]any{"title": pullTitle(repo, n)}, Limit: 1})
	if err != nil || len(threads) == 0 {
		return nil, err
	}
	return &threads[0], nil
}

// repository matches a GitHub repository with its project and id.
func (p *PullReviews) repository(ctx context.Context, repo string) (*string, string) {
	repos, _ := p.Store.Repositories().ListAll(ctx, data.Filter{})
	for _, r := range repos {
		if name, ok := github.ParseRemote(r.RemoteURL); ok && strings.EqualFold(name, repo) {
			project := r.ProjectID
			return &project, r.ID
		}
	}
	return nil, ""
}
