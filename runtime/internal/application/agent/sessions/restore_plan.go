package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type RestorePlan struct {
	sessionReplacement session.Replacement
	history            HistorySnapshot
	planReplacement    *plan.Replacement
	retirement         *ResourceRetirement
}

func NewRestorePlan(
	snapshot Snapshot,
	sessionReplacement session.Replacement,
	planReplacement *plan.Replacement,
) (RestorePlan, error) {
	if err := sessionReplacement.Validate(); err != nil {
		return RestorePlan{}, fmt.Errorf("sessions: restore plan Session replacement: %w", err)
	}
	owned, err := ownHistory(snapshot.HistorySnapshot)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("sessions: restore plan snapshot: %w", err)
	}
	restoredSession := sessionReplacement.State()
	if owned.Session.ID() != restoredSession.ID() {
		return RestorePlan{}, errors.New("sessions: restore plan Session replacement has a different identity")
	}
	replacement, err := ownRestorePlanReplacement(snapshot.Plan, planReplacement)
	if err != nil {
		return RestorePlan{}, err
	}
	owned.Session = restoredSession
	var retirement *ResourceRetirement
	if _, replacing := sessionReplacement.Expected(); replacing {
		value, err := newResourceRetirement(restoredSession.ID())
		if err != nil {
			return RestorePlan{}, err
		}
		retirement = &value
	}
	return RestorePlan{
		sessionReplacement: sessionReplacement,
		history:            owned,
		planReplacement:    replacement,
		retirement:         retirement,
	}, nil
}

func (r RestorePlan) Retirement() (ResourceRetirement, bool) {
	if r.retirement == nil {
		return ResourceRetirement{}, false
	}
	return *r.retirement, true
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

func (r RestorePlan) SessionReplacement() session.Replacement { return r.sessionReplacement }

func (r RestorePlan) History() HistorySnapshot { return r.history.clone() }

func (r RestorePlan) PlanReplacement() *plan.Replacement {
	clonedPlanReplacement := r.planReplacement
	if clonedPlanReplacement != nil {
		clonedPlanReplacement = new(*clonedPlanReplacement)
	}
	return clonedPlanReplacement
}
