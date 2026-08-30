package agent

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/exactint"
)

// Plan is one committed whole-list replacement of the Session plan. Revision
// and Items move as one immutable value so consumers cannot accidentally pair
// content from one replacement with the ordering identity of another.
//
// Absence is represented by a nil *Plan at projection boundaries. Therefore a
// Plan always has a positive revision, including an explicitly cleared Plan
// whose Items list is empty.
type Plan struct {
	revision uint64
	content  PlanContent
}

func NewPlan(revision uint64, items []PlanItem) (Plan, error) {
	content, err := NewPlanContent(items)
	if err != nil {
		return Plan{}, err
	}
	return CommitPlan(revision, content)
}

// PlanContent is one validated whole-list replacement before the Runtime gives
// it a durable revision. Keeping this value explicit lets commands and scripted
// fixtures describe content without inventing ordering metadata.
type PlanContent struct{ items []PlanItem }

func NewPlanContent(items []PlanItem) (PlanContent, error) {
	content := PlanContent{items: append([]PlanItem{}, items...)}
	if err := content.Validate(); err != nil {
		return PlanContent{}, err
	}
	return content, nil
}

func (c PlanContent) Validate() error {
	if err := validatePlan(c.items); err != nil {
		return fmt.Errorf("plan content: %w", err)
	}
	return nil
}

func (c PlanContent) Items() []PlanItem { return append([]PlanItem{}, c.items...) }

func (c PlanContent) Clone() PlanContent {
	return PlanContent{items: append([]PlanItem{}, c.items...)}
}

func (c PlanContent) Equal(other PlanContent) bool { return slices.Equal(c.items, other.items) }

func CommitPlan(revision uint64, content PlanContent) (Plan, error) {
	plan := Plan{revision: revision, content: content.Clone()}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// CommitNextPlan assigns the first durable revision to unwritten content or
// advances an existing Plan exactly once. Overflow is rejected instead of
// wrapping a long-lived Session back to an older identity.
func CommitNextPlan(previous *Plan, content PlanContent) (Plan, error) {
	revision := exactint.First()
	if previous != nil {
		if err := previous.Validate(); err != nil {
			return Plan{}, fmt.Errorf("previous plan: %w", err)
		}
		current, err := exactint.Restore(previous.revision)
		if err != nil {
			return Plan{}, fmt.Errorf("previous plan revision: %w", err)
		}
		revision, err = current.Next()
		if err != nil {
			return Plan{}, fmt.Errorf("plan revision: %w", err)
		}
	}
	return CommitPlan(revision.Value(), content)
}

func (p Plan) Validate() error {
	revision, err := exactint.Restore(p.revision)
	if err != nil {
		return fmt.Errorf("plan revision: %w", err)
	}
	if revision.IsZero() {
		return errors.New("plan revision must be positive")
	}
	if err := p.content.Validate(); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	return nil
}

func (p Plan) Revision() uint64 { return p.revision }

func (p Plan) Items() []PlanItem { return p.content.Items() }

func (p Plan) Content() PlanContent { return p.content.Clone() }

func (p Plan) Clone() Plan {
	return Plan{revision: p.revision, content: p.content.Clone()}
}

func (p Plan) Equal(other Plan) bool {
	return p.revision == other.revision && p.content.Equal(other.content)
}

func clonePlan(plan *Plan) *Plan {
	if plan == nil {
		return nil
	}
	cloned := plan.Clone()
	return &cloned
}

func equalPlans(left, right *Plan) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}
