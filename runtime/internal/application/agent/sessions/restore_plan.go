package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// RestorePlan is the atomic durable command for replacing one Session and all
// of its Session-owned durable projections.
type RestorePlan struct {
	sessionReplacement session.Replacement
	snapshot           Snapshot
	planReplacement    *plan.Replacement
}

// NewRestorePlan binds an already-decided Session replacement to one normalized
// restored projection and the exact Plan transition that publishes its steps.
func NewRestorePlan(
	snapshot Snapshot,
	sessionReplacement session.Replacement,
	planReplacement *plan.Replacement,
) (RestorePlan, error) {
	if err := sessionReplacement.Validate(); err != nil {
		return RestorePlan{}, fmt.Errorf("sessions: restore plan Session replacement: %w", err)
	}
	owned, err := ownWriteSnapshot(snapshot)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("sessions: restore plan snapshot: %w", err)
	}
	restoredSession := sessionReplacement.State()
	if owned.Session.ID() != restoredSession.ID() {
		return RestorePlan{}, errors.New("sessions: restore plan Session replacement has a different identity")
	}
	replacement, err := ownRestorePlanReplacement(owned.Plan, planReplacement)
	if err != nil {
		return RestorePlan{}, err
	}
	owned.Session = restoredSession
	// The replacement is the sole stored representation of restored Plan steps.
	owned.Plan = nil
	restore := RestorePlan{
		sessionReplacement: sessionReplacement,
		snapshot:           owned,
		planReplacement:    replacement,
	}
	if err := restore.Validate(); err != nil {
		return RestorePlan{}, err
	}
	return restore, nil
}

func ownRestorePlanReplacement(steps []plan.Step, replacement *plan.Replacement) (*plan.Replacement, error) {
	if err := validateRestorePlanReplacement(steps, replacement); err != nil {
		return nil, err
	}
	if replacement == nil {
		return nil, nil
	}
	owned := *replacement
	return &owned, nil
}

func validateRestorePlanReplacement(steps []plan.Step, replacement *plan.Replacement) error {
	if replacement == nil {
		if len(steps) > 0 {
			return errors.New("sessions: restore plan has steps without a Plan replacement")
		}
		return nil
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sessions: restore plan Plan replacement: %w", err)
	}
	if !slices.Equal(replacement.State().Steps(), steps) {
		return errors.New("sessions: restore plan Plan replacement differs from restored steps")
	}
	return nil
}

// Validate proves that the committed Session, every restored projection, and
// the optional Plan transition remain one coherent replacement.
func (r RestorePlan) Validate() error {
	if err := r.sessionReplacement.Validate(); err != nil {
		return fmt.Errorf("sessions: restore plan Session replacement: %w", err)
	}
	snapshot := r.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("sessions: restore plan snapshot: %w", err)
	}
	if snapshot.Session.Snapshot() != r.sessionReplacement.State().Snapshot() {
		return errors.New("sessions: restore plan snapshot differs from its Session replacement")
	}
	return validateRestorePlanReplacement(snapshot.Plan, r.planReplacement)
}

// SessionReplacement returns the exact initial insert or monotonic replacement.
func (r RestorePlan) SessionReplacement() session.Replacement { return r.sessionReplacement }

// Snapshot returns an ownership-isolated complete restored projection.
func (r RestorePlan) Snapshot() Snapshot {
	var steps []plan.Step
	if r.planReplacement != nil {
		steps = r.planReplacement.State().Steps()
	}
	return Snapshot{
		Session: r.snapshot.Session, Messages: cloneSnapshotMessages(r.snapshot.Messages),
		Runs: slices.Clone(r.snapshot.Runs), Items: slices.Clone(r.snapshot.Items),
		ToolResults: slices.Clone(r.snapshot.ToolResults), Plan: steps,
	}
}

// PlanReplacement returns an isolated copy of the restored Plan transition.
func (r RestorePlan) PlanReplacement() *plan.Replacement {
	if r.planReplacement == nil {
		return nil
	}
	replacement := *r.planReplacement
	return &replacement
}
