package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// ForkPlan is the atomic durable command for creating one child Session with
// the complete visible history and Plan held by its resolved parent boundary.
type ForkPlan struct {
	parentID        resourceid.SessionID
	snapshot        Snapshot
	planReplacement *plan.Replacement
}

// NewForkPlan binds one already-remapped child snapshot and its initial Plan
// transition to the exact parent while every projection is still in Application.
func NewForkPlan(
	parentID string,
	snapshot Snapshot,
	planReplacement *plan.Replacement,
) (ForkPlan, error) {
	parent, err := resourceid.ParseSession(parentID)
	if err != nil {
		return ForkPlan{}, fmt.Errorf("sessions: fork plan parent: %w", err)
	}
	owned, err := ownWriteSnapshot(snapshot)
	if err != nil {
		return ForkPlan{}, fmt.Errorf("sessions: fork plan snapshot: %w", err)
	}
	if owned.Session.ParentID() != parent.String() {
		return ForkPlan{}, errors.New("sessions: fork plan child belongs to a different parent")
	}
	if owned.Session.Revision() != 1 {
		return ForkPlan{}, errors.New("sessions: fork plan child is not at its initial revision")
	}
	replacement, err := ownForkPlanReplacement(owned.Plan, planReplacement)
	if err != nil {
		return ForkPlan{}, err
	}
	// The replacement is the sole stored representation of inherited Plan steps.
	// Snapshot reconstructs the read projection from that owner when requested.
	owned.Plan = nil
	fork := ForkPlan{parentID: parent, snapshot: owned, planReplacement: replacement}
	return fork, nil
}

func ownForkPlanReplacement(steps []plan.Step, replacement *plan.Replacement) (*plan.Replacement, error) {
	if err := validateForkPlanReplacement(steps, replacement); err != nil {
		return nil, err
	}
	if replacement == nil {
		return nil, nil
	}
	owned := *replacement
	return &owned, nil
}

func validateForkPlanReplacement(steps []plan.Step, replacement *plan.Replacement) error {
	if len(steps) == 0 {
		if replacement != nil {
			return errors.New("sessions: fork plan has a Plan replacement without inherited steps")
		}
		return nil
	}
	if replacement == nil {
		return errors.New("sessions: fork plan inherited steps have no initial replacement")
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sessions: fork plan replacement: %w", err)
	}
	if !replacement.ExpectedVersion().IsUnwritten() {
		return errors.New("sessions: fork plan replacement does not start from an unwritten Plan")
	}
	if !slices.Equal(replacement.State().Steps(), steps) {
		return errors.New("sessions: fork plan replacement differs from the inherited Plan")
	}
	return nil
}

// IsZero reports whether no fork plan was constructed.
func (f ForkPlan) IsZero() bool { return f.parentID.String() == "" }

// ParentID returns the canonical parent Session identity.
func (f ForkPlan) ParentID() string { return f.parentID.String() }

// Child returns the complete Domain-derived child Session.
func (f ForkPlan) Child() session.Session { return f.snapshot.Session }

// Snapshot returns an ownership-isolated complete child projection.
func (f ForkPlan) Snapshot() Snapshot {
	var steps []plan.Step
	if f.planReplacement != nil {
		steps = f.planReplacement.State().Steps()
	}
	return Snapshot{
		Session: f.snapshot.Session, Messages: cloneSnapshotMessages(f.snapshot.Messages),
		Runs: slices.Clone(f.snapshot.Runs), Items: slices.Clone(f.snapshot.Items),
		ToolResults: slices.Clone(f.snapshot.ToolResults), Plan: steps,
	}
}

// PlanReplacement returns an isolated initial Plan transition when the fork
// boundary held a non-empty Plan.
func (f ForkPlan) PlanReplacement() *plan.Replacement {
	if f.planReplacement == nil {
		return nil
	}
	replacement := *f.planReplacement
	return &replacement
}
