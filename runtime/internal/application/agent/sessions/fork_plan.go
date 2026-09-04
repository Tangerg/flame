package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
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
	owned, err := ownForkSnapshot(snapshot)
	if err != nil {
		return ForkPlan{}, err
	}
	replacement, err := ownForkPlanReplacement(owned.Plan, planReplacement)
	if err != nil {
		return ForkPlan{}, err
	}
	// The replacement is the sole stored representation of inherited Plan steps.
	// Snapshot reconstructs the read projection from that owner when requested.
	owned.Plan = nil
	fork := ForkPlan{parentID: parent, snapshot: owned, planReplacement: replacement}
	if err := fork.Validate(); err != nil {
		return ForkPlan{}, err
	}
	return fork, nil
}

func ownForkSnapshot(snapshot Snapshot) (Snapshot, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return Snapshot{}, fmt.Errorf("sessions: fork plan snapshot normalization: %w", err)
	}
	history, err := conversation.New(normalized.Messages)
	if err != nil {
		return Snapshot{}, fmt.Errorf("sessions: fork plan conversation: %w", err)
	}
	owned := Snapshot{
		Session: normalized.Session, Messages: history.Messages(),
		Runs: runsInParentFirstOrder(normalized.Runs), Items: slices.Clone(normalized.Items),
		ToolResults: slices.Clone(normalized.ToolResults), Plan: slices.Clone(normalized.Plan),
	}
	if err := owned.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("sessions: fork plan snapshot: %w", err)
	}
	return owned, nil
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

// Validate proves that the child is an initial fork of the addressed parent and
// that every durable projection and optional Plan transition agrees with it.
func (f ForkPlan) Validate() error {
	if err := f.parentID.Validate(); err != nil {
		return fmt.Errorf("sessions: fork plan parent: %w", err)
	}
	snapshot := f.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("sessions: fork plan snapshot: %w", err)
	}
	child := snapshot.Session
	if child.ParentID() != f.parentID.String() {
		return errors.New("sessions: fork plan child belongs to a different parent")
	}
	if child.Revision() != 1 {
		return errors.New("sessions: fork plan child is not at its initial revision")
	}
	return validateForkPlanReplacement(snapshot.Plan, f.planReplacement)
}

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
		Session: f.snapshot.Session, Messages: cloneForkMessages(f.snapshot.Messages),
		Runs: slices.Clone(f.snapshot.Runs), Items: slices.Clone(f.snapshot.Items),
		ToolResults: slices.Clone(f.snapshot.ToolResults), Plan: steps,
	}
}

func cloneForkMessages(messages []chat.Message) []chat.Message {
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		owned[index] = message.Clone()
	}
	return owned
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
