package t3

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Thread organization commands, as T3 Code's sidebar sends them
// (orchestration.dispatchCommand, T3 0.1.8+).

// RenameThread changes a thread's title.
func (c Client) RenameThread(ctx context.Context, threadID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("t3: empty title")
	}
	return c.Dispatch(ctx, map[string]any{"type": "thread.metadata.update", "threadId": threadID, "title": title})
}

func (c Client) simple(ctx context.Context, kind, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": kind, "threadId": threadID})
}

// UnarchiveThread brings an archived thread back.
func (c Client) UnarchiveThread(ctx context.Context, threadID string) error {
	return c.simple(ctx, "thread.unarchive", threadID)
}

// UnsettleThread moves a settled thread back to the active list.
func (c Client) UnsettleThread(ctx context.Context, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "thread.unsettle", "threadId": threadID, "reason": "user"})
}

// PinThread keeps a thread at the top of the list; T3 picks its place.
func (c Client) PinThread(ctx context.Context, threadID string) error {
	return c.simple(ctx, "thread.pin", threadID)
}

// UnpinThread drops a thread from the pinned ones.
func (c Client) UnpinThread(ctx context.Context, threadID string) error {
	return c.simple(ctx, "thread.unpin", threadID)
}

// SnoozeThread hides a thread from the active list until the given time.
func (c Client) SnoozeThread(ctx context.Context, threadID string, until time.Time) error {
	if !until.After(time.Now()) {
		return errors.New("t3: snooze time must be in the future")
	}
	return c.Dispatch(ctx, map[string]any{"type": "thread.snooze", "threadId": threadID, "snoozedUntil": until.UTC().Format(time.RFC3339Nano)})
}

// UnsnoozeThread wakes a snoozed thread now.
func (c Client) UnsnoozeThread(ctx context.Context, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "thread.unsnooze", "threadId": threadID, "reason": "user"})
}

// VisitThread marks a thread as read, as opening it in T3 Code does.
func (c Client) VisitThread(ctx context.Context, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "thread.visit", "threadId": threadID, "visitedAt": time.Now().UTC().Format(time.RFC3339Nano), "opened": true})
}

// MarkThreadUnread marks a thread as unread.
func (c Client) MarkThreadUnread(ctx context.Context, threadID string) error {
	return c.simple(ctx, "thread.mark-unread", threadID)
}

// StopThread stops everything the thread is running.
func (c Client) StopThread(ctx context.Context, threadID string) error {
	return c.simple(ctx, "thread.stop", threadID)
}

// ForkThread copies a thread up to the end of one of its runs into a new
// thread and returns the new thread's id.
func (c Client) ForkThread(ctx context.Context, threadID, runID, title string) (string, error) {
	if runID == "" {
		return "", errors.New("t3: fork needs a run")
	}
	id := newID()
	cmd := map[string]any{
		"type": "thread.fork", "createdBy": "user", "creationSource": "web",
		"sourceThreadId": threadID, "targetThreadId": id,
		"sourcePoint": map[string]any{"type": "run", "runId": runID},
	}
	if title = strings.TrimSpace(title); title != "" {
		cmd["title"] = title
	}
	return id, c.Dispatch(ctx, cmd)
}
