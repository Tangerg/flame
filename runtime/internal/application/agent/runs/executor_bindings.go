package runs

import (
	"fmt"
	"maps"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// A child binding is reserved before its durable opening commit, closing the gap
// in which the Run row exists but cancellation cannot address its member.
func (r *runTreeOwner) bindExecutorMember(runID identity.RunID, memberID string) error {
	if err := runID.Validate(); err != nil {
		return fmt.Errorf("runs: bind executor member: %w", err)
	}
	if err := ValidateMember(memberID); err != nil {
		return fmt.Errorf("runs: bind Run %q: %w", runID, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, bound := r.executorMembers[runID]; bound {
		if existing != memberID {
			return fmt.Errorf(
				"runs: Run %q executor member changed from %q to %q",
				runID,
				existing,
				memberID,
			)
		}
		return nil
	}
	for existingRunID, existing := range r.executorMembers {
		if existing == memberID {
			return fmt.Errorf(
				"runs: executor member %q is already bound to Run %q, not %q",
				memberID,
				existingRunID,
				runID,
			)
		}
	}
	r.executorMembers[runID] = memberID
	return nil
}

func (r *runTreeOwner) unbindExecutorMember(runID identity.RunID, memberID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, bound := r.executorMembers[runID]; bound && existing == memberID {
		delete(r.executorMembers, runID)
	}
}

func (r *runTreeOwner) executorMemberSnapshot() map[identity.RunID]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	members := make(map[identity.RunID]string, len(r.executorMembers))
	maps.Copy(members, r.executorMembers)
	return members
}
