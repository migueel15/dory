package runtime

import (
	"context"
	"regexp"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

// MaxActiveChildren caps the subagents of one thread working at once.
const MaxActiveChildren = 5

// resultChars caps the result a subagent hands to its parent.
const resultChars = 4000

// harnessMarkers are inline markers a harness leaves in its answers, such as
// codex web citations between U+E200 and U+E201; they are not text.
var harnessMarkers = regexp.MustCompile("\uE200[^\uE201]*\uE201")

// CanSpawn says whether parent may launch one more subagent: a subagent
// cannot launch its own, and at most MaxActiveChildren work at once.
func (m *Manager) CanSpawn(ctx context.Context, parent data.Thread) error {
	if parent.ParentID != nil {
		return data.ErrInvalid.Msgf("a subagent cannot launch subagents")
	}
	working, err := m.store.Threads().List(ctx, data.Filter{Fields: map[string]any{"parent_id": parent.ID, "status": "working"}, Limit: MaxActiveChildren + 1})
	if err != nil {
		return err
	}
	if len(working) >= MaxActiveChildren {
		return data.ErrInvalid.Msgf("%d subagents are already working; wait for one or stop it", MaxActiveChildren)
	}
	return nil
}

// notifyParent hands the outcome of a finished subagent turn to the node of
// its parent, which records it and wakes the parent.
func (m *Manager) notifyParent(ctx context.Context, child data.Thread, status string, stopped bool) {
	parent, err := m.store.Threads().Get(ctx, *child.ParentID)
	if err != nil || parent.ArchivedAt != nil {
		return
	}
	outcome := map[string]string{"idle": "completed", "error": "failed", "waiting": "waiting"}[status]
	if stopped {
		outcome = "interrupted"
	}
	if outcome == "" {
		return
	}
	var result string
	if msgs, err := m.Messages(ctx, child.ID); err == nil {
		for _, msg := range msgs {
			if msg.Role == "assistant" {
				result = msg.Text
			}
		}
	}
	result = strings.TrimSpace(harnessMarkers.ReplaceAllString(result, ""))
	if r := []rune(result); len(r) > resultChars {
		result = string(r[:resultChars]) + "…"
	}
	if fresh, err := m.store.Threads().Get(ctx, child.ID); err == nil {
		child = fresh
	}
	m.deliver(ctx, parent.NodeID, Outcome{ParentThreadID: parent.ID, ChildThreadID: child.ID, ChildNodeID: m.node, Title: child.Title, Status: outcome, Result: result})
}
