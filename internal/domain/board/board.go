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
