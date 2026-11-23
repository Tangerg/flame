package sessions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type RollbackPlan struct {
	sessionID         identity.SessionID
	keepMessageMark   rundomain.MessageMark
	dropRunIDs        []identity.RunID
	checkpointRootIDs []runs.MemberID
	planReplacement   *plan.Replacement
}

func NewRollbackPlan(
	sessionID identity.SessionID,
	boundary transcript.Boundary,
	checkpointRootIDs []string,
	planReplacement *plan.Replacement,
) (RollbackPlan, error) {
	if err := sessionID.Validate(); err != nil {
		return RollbackPlan{}, fmt.Errorf("sessions: rollback plan session: %w", err)
	}
	if count, known := boundary.KeepMessageMark.Count(); known && count < 0 {
		return RollbackPlan{}, fmt.Errorf("sessions: rollback plan message mark %s is invalid", boundary.KeepMessageMark)
	}
	dropRunIDs, err := validateRollbackRunIDs(boundary.DroppedRunIDs())
	if err != nil {
		return RollbackPlan{}, err
	}
	checkpointRoots, err := parseRollbackCheckpointRoots(checkpointRootIDs)
	if err != nil {
		return RollbackPlan{}, err
	}
	if planReplacement != nil {
		if err := planReplacement.Validate(); err != nil {
			return RollbackPlan{}, fmt.Errorf("sessions: rollback plan replacement: %w", err)
		}
	}
	clonedPlanReplacement := planReplacement
	if clonedPlanReplacement != nil {
		clonedPlanReplacement = new(*clonedPlanReplacement)
	}
	return RollbackPlan{
		sessionID: sessionID, keepMessageMark: boundary.KeepMessageMark,
		dropRunIDs: dropRunIDs, checkpointRootIDs: checkpointRoots,
		planReplacement: clonedPlanReplacement,
	}, nil
}

func validateRollbackRunIDs(values []identity.RunID) ([]identity.RunID, error) {
	if len(values) == 0 {
		return nil, errors.New("sessions: rollback plan has no dropped runs")
	}
	seen := make(map[identity.RunID]struct{}, len(values))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("sessions: rollback plan dropped run[%d]: %w", index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("sessions: rollback plan repeats dropped run %q", value)
		}
		seen[value] = struct{}{}
	}
	return slices.Clone(values), nil
}

func parseRollbackCheckpointRoots(values []string) ([]runs.MemberID, error) {
	ids := make([]runs.MemberID, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		id, err := runs.ParseMember(value)
		if err != nil {
			return nil, fmt.Errorf("sessions: rollback plan checkpoint root[%d]: %w", index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("sessions: rollback plan repeats checkpoint root %q", value)
		}
		seen[value] = struct{}{}
		ids[index] = id
	}
	return ids, nil
}

func (r RollbackPlan) SessionID() identity.SessionID { return r.sessionID }

func (r RollbackPlan) TruncationMark() (int, bool) {
	return r.keepMessageMark.Count()
}

func (r RollbackPlan) DropRunIDs() []identity.RunID { return slices.Clone(r.dropRunIDs) }

func (r RollbackPlan) CheckpointRootIDs() []string {
	ids := make([]string, len(r.checkpointRootIDs))
	for index, id := range r.checkpointRootIDs {
		ids[index] = id.String()
	}
	return ids
}

func (r RollbackPlan) PlanReplacement() *plan.Replacement {
	clonedPlanReplacement := r.planReplacement
	if clonedPlanReplacement != nil {
		clonedPlanReplacement = new(*clonedPlanReplacement)
	}
	return clonedPlanReplacement
}
