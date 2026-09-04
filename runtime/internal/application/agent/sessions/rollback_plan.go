package sessions

import (
	"errors"
	"fmt"

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
	keepMessageMark   int
	dropRunIDs        []resourceid.RunID
	checkpointRootIDs []runtimeidentity.MemberID
	planReplacement   *plan.Replacement
}

// NewRollbackPlan binds one resolved transcript boundary, its parked executor
// roots, and its already-decided Plan replacement to the exact Session owner.
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
	dropRunIDs, err := parseRollbackRunIDs(boundary.DroppedRunIDs())
	if err != nil {
		return RollbackPlan{}, err
	}
	checkpointRoots, err := parseRollbackCheckpointRoots(checkpointRootIDs)
	if err != nil {
		return RollbackPlan{}, err
	}
	var ownedPlanReplacement *plan.Replacement
	if planReplacement != nil {
		if err := planReplacement.Validate(); err != nil {
			return RollbackPlan{}, fmt.Errorf("sessions: rollback plan replacement: %w", err)
		}
		cloned := *planReplacement
		ownedPlanReplacement = &cloned
	}
	rollback := RollbackPlan{
		sessionID: id, keepMessageMark: boundary.KeepMessageMark,
		dropRunIDs: dropRunIDs, checkpointRootIDs: checkpointRoots,
		planReplacement: ownedPlanReplacement,
	}
	if err := rollback.Validate(); err != nil {
		return RollbackPlan{}, err
	}
	return rollback, nil
}

// Validate proves all rollback identities are canonical and unique and the
// message coordinate is either exact or the one Domain-owned unknown sentinel.
func (r RollbackPlan) Validate() error {
	if err := r.sessionID.Validate(); err != nil {
		return fmt.Errorf("sessions: rollback plan session: %w", err)
	}
	if r.keepMessageMark < rundomain.UnknownMessageMark {
		return fmt.Errorf("sessions: rollback plan message mark %d is invalid", r.keepMessageMark)
	}
	if len(r.dropRunIDs) == 0 {
		return errors.New("sessions: rollback plan has no dropped runs")
	}
	seenRuns := make(map[string]struct{}, len(r.dropRunIDs))
	for index, id := range r.dropRunIDs {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("sessions: rollback plan dropped run[%d]: %w", index, err)
		}
		if _, duplicate := seenRuns[id.String()]; duplicate {
			return fmt.Errorf("sessions: rollback plan repeats dropped run %q", id.String())
		}
		seenRuns[id.String()] = struct{}{}
	}
	seenRoots := make(map[string]struct{}, len(r.checkpointRootIDs))
	for index, id := range r.checkpointRootIDs {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("sessions: rollback plan checkpoint root[%d]: %w", index, err)
		}
		if _, duplicate := seenRoots[id.String()]; duplicate {
			return fmt.Errorf("sessions: rollback plan repeats checkpoint root %q", id.String())
		}
		seenRoots[id.String()] = struct{}{}
	}
	if r.planReplacement != nil {
		if err := r.planReplacement.Validate(); err != nil {
			return fmt.Errorf("sessions: rollback plan replacement: %w", err)
		}
	}
	return nil
}

func parseRollbackRunIDs(values []string) ([]resourceid.RunID, error) {
	ids := make([]resourceid.RunID, len(values))
	for index, value := range values {
		id, err := resourceid.ParseRun(value)
		if err != nil {
			return nil, fmt.Errorf("sessions: rollback plan dropped run[%d]: %w", index, err)
		}
		ids[index] = id
	}
	return ids, nil
}

func parseRollbackCheckpointRoots(values []string) ([]runtimeidentity.MemberID, error) {
	ids := make([]runtimeidentity.MemberID, len(values))
	for index, value := range values {
		id, err := runtimeidentity.ParseMember(value)
		if err != nil {
			return nil, fmt.Errorf("sessions: rollback plan checkpoint root[%d]: %w", index, err)
		}
		ids[index] = id
	}
	return ids, nil
}

// SessionID returns the exact durable owner.
func (r RollbackPlan) SessionID() string { return r.sessionID.String() }

// TruncationMark returns the exact retained message count when the boundary has
// one. Unknown pre-watermark boundaries return false and leave history intact.
func (r RollbackPlan) TruncationMark() (int, bool) {
	return r.keepMessageMark, r.keepMessageMark != rundomain.UnknownMessageMark
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
	if r.planReplacement == nil {
		return nil
	}
	replacement := *r.planReplacement
	return &replacement
}
