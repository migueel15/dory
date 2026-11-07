package service

import (
	"context"
	"fmt"
)

// Title sources: who wrote the title of an item.
const (
	TitlePending = "pending"
	TitleAI      = "ai"
	TitleUser    = "user"
	// TitleFallback is the provisional title kept after the generation failed or timed out.
	TitleFallback = "fallback"
)

// titledItems enforces the title_source rules on every write, whether it
// arrives from the web or from an agent through MCP.
type titledItems struct{ resource[Item] }

// ResolveTitleSource returns the title source an item gets when it replaces
// current (nil when it is new).
func ResolveTitleSource(current *Item, in Item) (string, error) {
	switch in.TitleSource {
	case "", TitlePending, TitleAI, TitleUser, TitleFallback:
	default:
		return "", fmt.Errorf("%w: title_source must be pending, ai, user or fallback", ErrInvalid)
	}
	if current == nil {
		if in.TitleSource == TitleAI || in.TitleSource == TitleFallback {
			return "", fmt.Errorf("%w: a new item cannot have an AI or fallback title yet", ErrInvalid)
		}
		if in.TitleSource == "" {
			return TitleUser, nil
		}
		return in.TitleSource, nil
	}
	if in.TitleSource == TitleAI {
		if current.TitleSource == TitlePending {
			return TitleAI, nil
		}
		// A full replacement that echoes an AI title keeps it; a changed title is never taken from the AI
		// once the title is no longer pending.
		if in.Title == current.Title {
			return current.TitleSource, nil
		}
		return "", fmt.Errorf("%w: the title is not pending generation", ErrInvalid)
	}
	if in.Title != current.Title {
		return TitleUser, nil
	}
	// Same title: only these hand-offs change the source: pending -> fallback or user (failure or
	// timeout), fallback -> pending (regeneration) and fallback -> user.
	switch {
	case current.TitleSource == TitlePending && (in.TitleSource == TitleUser || in.TitleSource == TitleFallback),
		current.TitleSource == TitleFallback && (in.TitleSource == TitleUser || in.TitleSource == TitlePending):
		return in.TitleSource, nil
	}
	return current.TitleSource, nil
}

func (t titledItems) Put(ctx context.Context, actor string, in Item) (Item, error) {
	var current *Item
	if in.ID != "" {
		if c, err := t.Get(ctx, in.ID); err == nil {
			current = &c
		}
	}
	source, err := ResolveTitleSource(current, in)
	if err != nil {
		return Item{}, err
	}
	in.TitleSource = source
	return t.resource.Put(ctx, actor, in)
}
