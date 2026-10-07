package sessions

import (
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/optional"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// RollbackPlan is the atomic durable command for truncating a Session back to
// one Domain-resolved Run boundary. A parked Run among the dropped identities
// needs no terminalization: dropping its record also releases the admission slot.
type RollbackPlan struct {
	sessionID         resourceid.SessionID
	keepMessageMark   rundomain.MessageMark
	dropRunIDs        []resourceid.RunID
	checkpointRootIDs []runtimeidentity.MemberID
	planReplacement   *plan.Replacement
}

// NewRollbackPlan binds one resolved transcript boundary, its parked executor
// roots, and its already-decided Plan replacement to the exact Session owner.
// It is the plan's only validation: every field is parsed or proven here once.
func NewRollbackPlan(
	sessionID string,
	boundary transcript.Boundary,
	checkpointRootIDs []string,
	planReplacement *plan.Replacement,
) (RollbackPlan, error) {
	id, err := resourceid.ParseSession(sessionID)
	if err != nil {
		return RollbackPlan{}, fmt.Errorf("sessions: rollback plan session: %w", err)
	}
	if count, known := boundary.KeepMessageMark.Count(); known && count < 0 {
		return RollbackPlan{}, fmt.Errorf("sessions: rollback plan message mark %s is invalid", boundary.KeepMessageMark)
	}
	dropRunIDs, err := parseRollbackRunIDs(boundary.DroppedRunIDs())
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
	return RollbackPlan{
		sessionID: id, keepMessageMark: boundary.KeepMessageMark,
		dropRunIDs: dropRunIDs, checkpointRootIDs: checkpointRoots,
		planReplacement: optional.Clone(planReplacement),
	}, nil
}

func parseRollbackRunIDs(values []string) ([]resourceid.RunID, error) {
	if len(values) == 0 {
		return nil, errors.New("sessions: rollback plan has no dropped runs")
	}
	ids := make([]resourceid.RunID, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		id, err := resourceid.ParseRun(value)
		if err != nil {
			return nil, fmt.Errorf("sessions: rollback plan dropped run[%d]: %w", index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("sessions: rollback plan repeats dropped run %q", value)
		}
		seen[value] = struct{}{}
		ids[index] = id
	}
	return ids, nil
}

func parseRollbackCheckpointRoots(values []string) ([]runtimeidentity.MemberID, error) {
	ids := make([]runtimeidentity.MemberID, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		id, err := runtimeidentity.ParseMember(value)
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

// SessionID returns the exact durable owner.
func (r RollbackPlan) SessionID() string { return r.sessionID.String() }

// TruncationMark returns the exact retained message count when the boundary has
// one. Unknown pre-watermark boundaries return false and leave history intact.
func (r RollbackPlan) TruncationMark() (int, bool) {
	return r.keepMessageMark.Count()
}

// DropRunIDs returns the isolated canonical Run deletion order.
func (r RollbackPlan) DropRunIDs() []string {
	ids := make([]string, len(r.dropRunIDs))
	for index, id := range r.dropRunIDs {
		ids[index] = id.String()
	}
	return ids
}

// CheckpointRootIDs returns the isolated parked-executor roots to remove.
func (r RollbackPlan) CheckpointRootIDs() []string {
	ids := make([]string, len(r.checkpointRootIDs))
	for index, id := range r.checkpointRootIDs {
		ids[index] = id.String()
	}
	return ids
}

// PlanReplacement returns an isolated copy of the boundary Plan transition.
func (r RollbackPlan) PlanReplacement() *plan.Replacement {
	return optional.Clone(r.planReplacement)
}
