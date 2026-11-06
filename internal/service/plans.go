package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// definable are the item statuses whose plan can still change.
var definable = map[string]bool{"inbox": true, "triaged": true, "ready": true}

// Markdown renders the non-empty sections as the item body.
func (p PlanSections) Markdown() string {
	var b strings.Builder
	for _, s := range []struct{ title, text string }{
		{"Objetivo", p.Goal}, {"Alcance", p.Scope}, {"Dónde", p.Where}, {"Diseño", p.Design},
		{"Requisitos", p.Requirements}, {"Cómo probar", p.Validation}, {"Riesgos", p.Risks},
	} {
		if text := strings.TrimSpace(s.text); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "## %s\n%s", s.title, text)
		}
	}
	return b.String()
}

func (p PlanSections) empty() bool { return strings.TrimSpace(p.Markdown()) == "" }

// LatestPlan returns the newest version of the item plan, or nil if it has none.
func LatestPlan(ctx context.Context, svc Service, itemID string) (*ItemPlan, error) {
	plans, err := svc.ItemPlans().List(ctx, Filter{Fields: map[string]any{"item_id": itemID}, Limit: 1})
	if err != nil || len(plans) == 0 {
		return nil, err
	}
	return &plans[0], nil
}

// ProposePlan stores a new plan version. A proposal never makes an item ready:
// an inbox or ready item goes back to triaged until the user accepts a version.
func ProposePlan(ctx context.Context, svc Service, actor, itemID, author string, sections PlanSections, questions []string) (ItemPlan, error) {
	var plan ItemPlan
	if sections.empty() {
		return plan, fmt.Errorf("%w: the plan needs at least one section", ErrInvalid)
	}
	clean := []string{}
	for _, q := range questions {
		if q = strings.TrimSpace(q); q != "" {
			clean = append(clean, q)
		}
	}
	err := svc.InTx(ctx, func(tx Service) error {
		item, err := tx.Items().Get(ctx, itemID)
		if err != nil {
			return err
		}
		if !definable[item.Status] {
			return fmt.Errorf("%w: an item in %s cannot change its plan", ErrInvalid, item.Status)
		}
		if plan, err = tx.ItemPlans().Put(ctx, actor, ItemPlan{ItemID: itemID, Author: author, Sections: sections, OpenQuestions: clean}); err != nil {
			return err
		}
		if item.Status == "triaged" {
			return nil
		}
		item.Status = "triaged"
		_, err = tx.Items().Put(ctx, actor, item)
		return err
	})
	return plan, err
}

// AcceptPlan accepts the newest plan version: it copies the plan to the item
// body and moves the item to ready. Only the user calls it.
func AcceptPlan(ctx context.Context, svc Service, actor, itemID string) (Item, ItemPlan, error) {
	var item Item
	var plan ItemPlan
	err := svc.InTx(ctx, func(tx Service) error {
		var err error
		if item, err = tx.Items().Get(ctx, itemID); err != nil {
			return err
		}
		latest, err := LatestPlan(ctx, tx, itemID)
		if err != nil {
			return err
		}
		switch {
		case latest == nil:
			return fmt.Errorf("%w: the item has no plan", ErrInvalid)
		case !definable[item.Status]:
			return fmt.Errorf("%w: an item in %s cannot accept a plan", ErrInvalid, item.Status)
		case len(latest.OpenQuestions) > 0:
			return fmt.Errorf("%w: the plan still has open questions", ErrInvalid)
		}
		plan = *latest
		if plan.AcceptedAt == nil {
			now := time.Now().UTC()
			plan.AcceptedAt = &now
			if plan, err = tx.ItemPlans().Put(ctx, actor, plan); err != nil {
				return err
			}
		}
		item.BodyMd, item.Status = plan.Sections.Markdown(), "ready"
		item, err = tx.Items().Put(ctx, actor, item)
		return err
	})
	return item, plan, err
}
