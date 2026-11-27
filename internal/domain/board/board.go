// Package board derives kanban columns from task, thread and pull request state.
package board

type Column string

const (
	Inbox        Column = "inbox"
	Ready        Column = "ready"
	Working      Column = "working"
	NeedsYou     Column = "needs_you"
	InReview     Column = "in_review"
	ReadyToMerge Column = "ready_to_merge"
	Done         Column = "done"
	Cancelled    Column = "cancelled"
)

func Columns() []Column {
	return []Column{Inbox, Ready, Working, NeedsYou, InReview, ReadyToMerge, Done, Cancelled}
}

type PullSummary struct {
	Repo           string
	Number         int
	State          string // draft | open | merged | closed
	CI             string
	ReviewDecision string
	ReviewRequests []string
}

type CardInput struct {
	Source         string // internal | github
	Stage          string // manual decision
	ExternalState  string // open | closed (external only)
	ExternalReason string // COMPLETED | NOT_PLANNED | ""
	PlanAccepted   bool
	Threads        []string
	Pulls          []PullSummary
}

type Result struct {
	Column Column
	Reason string
}

// Evaluate computes the deterministic kanban column and reason for a task.
func Evaluate(in CardInput) Result {
	isExternal := in.Source != "" && in.Source != "internal"

	// 1. Closed external issue
	if isExternal && in.ExternalState == "closed" {
		if in.ExternalReason == "NOT_PLANNED" {
			return Result{Column: Cancelled, Reason: "external_not_planned"}
		}
		return Result{Column: Done, Reason: "external_closed"}
	}

	// 2. Internal manually cancelled
	if !isExternal && in.Stage == "cancelled" {
		return Result{Column: Cancelled}
	}

	// 3. Any thread waiting or error
	working := false
	for _, status := range in.Threads {
		switch status {
		case "waiting":
			return Result{Column: NeedsYou, Reason: "waiting_thread"}
		case "error":
			return Result{Column: NeedsYou, Reason: "error_thread"}
		case "working":
			working = true
		}
	}

	// 4. Any open or draft PR with CI FAILURE/ERROR or CHANGES_REQUESTED
	hasDraft := false
	openPulls := 0
	allOpenReady := true
	hasMerged := false
	for _, p := range in.Pulls {
		switch p.State {
		case "open", "draft":
			if p.CI == "FAILURE" || p.CI == "ERROR" {
				return Result{Column: NeedsYou, Reason: "ci_failed"}
			}
			if p.ReviewDecision == "CHANGES_REQUESTED" {
				return Result{Column: NeedsYou, Reason: "changes_requested"}
			}
			if p.State == "draft" {
				hasDraft = true
			} else {
				openPulls++
				if !(p.ReviewDecision == "APPROVED" || (p.ReviewDecision == "" && len(p.ReviewRequests) == 0)) {
					allOpenReady = false
				}
			}
		case "merged":
			hasMerged = true
		}
	}

	// 5. Any working thread or draft PR
	if working || hasDraft {
		return Result{Column: Working}
	}

	// 6. Open PRs
	if openPulls > 0 {
		if allOpenReady {
			return Result{Column: ReadyToMerge}
		}
		return Result{Column: InReview}
	}

	// 7. Internal manually marked done (no open PRs)
	if !isExternal && in.Stage == "done" {
		return Result{Column: Done}
	}

	// 8 & 9. Pulls exist, none open/draft, at least one merged
	if len(in.Pulls) > 0 && hasMerged {
		if !isExternal {
			return Result{Column: Done}
		}
		return Result{Column: NeedsYou, Reason: "issue_open_pulls_merged"}
	}

	// 10. Manual ready or accepted plan
	if in.Stage == "ready" || in.PlanAccepted {
		return Result{Column: Ready}
	}

	// 11. Idle thread without open PR or accepted plan needs user action
	if len(in.Threads) > 0 {
		return Result{Column: NeedsYou, Reason: "idle_thread"}
	}

	// 12. Default: Inbox
	return Result{Column: Inbox}
}

// FinishedInput reports whether a task evaluated by CardInput is Done or Cancelled.
func FinishedInput(in CardInput) bool {
	col := Evaluate(in).Column
	return col == Done || col == Cancelled
}

type PullInfo struct {
	CI             string
	ReviewDecision string
	ReviewRequests []string
}

type Card struct {
	Stage       string
	PRState     string
	HasPRNumber bool
	Pull        *PullInfo
	Threads     []string
}

func DoneByHand(c Card) bool {
	return c.Stage == "done" && !c.HasPRNumber
}

// Finished reports whether the task can stop blocking its dependents.
func Finished(c Card) bool {
	return c.Stage == "cancelled" || c.PRState == "merged" || DoneByHand(c)
}

func (c Card) Column() Column {
	if c.Stage == "cancelled" || c.PRState == "closed" {
		return Cancelled
	}
	if c.PRState == "merged" || DoneByHand(c) {
		return Done
	}
	working := false
	for _, status := range c.Threads {
		switch status {
		case "waiting", "error":
			return NeedsYou
		case "working":
			working = true
		}
	}
	if p := c.Pull; p != nil && (p.CI == "FAILURE" || p.CI == "ERROR" || p.ReviewDecision == "CHANGES_REQUESTED") {
		return NeedsYou
	}
	if working || c.PRState == "draft" {
		return Working
	}
	if p := c.Pull; p != nil {
		if p.ReviewDecision == "APPROVED" || p.ReviewDecision == "" && len(p.ReviewRequests) == 0 {
			return ReadyToMerge
		}
		return InReview
	}
	if len(c.Threads) > 0 {
		return Working
	}
	if c.Stage == "ready" {
		return Ready
	}
	return Inbox
}
