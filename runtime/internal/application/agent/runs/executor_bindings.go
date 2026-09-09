package runs

import (
	"fmt"
	"maps"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// bindExecutorMember records the immutable application-Run to opaque executor
// identity owned by this root segment. Executor parent/spawn topology stays in
// executorRoutes; cancellation needs only the exact member identity it must
// address. A child binding is reserved before its durable opening commit,
// closing the otherwise observable gap in which the Run row exists but
// cancellation cannot address its member.
func (r *runTreeOwner) bindExecutorMember(runID, memberID string) error {
	if err := resourceid.ValidateRun(runID); err != nil {
		return fmt.Errorf("runs: bind executor member: %w", err)
	}
	if _, err := runtimeidentity.ParseMember(memberID); err != nil {
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

// unbindExecutorMember rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (r *runTreeOwner) unbindExecutorMember(runID, memberID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, bound := r.executorMembers[runID]; bound && existing == memberID {
		delete(r.executorMembers, runID)
	}
}

func (r *runTreeOwner) executorMemberSnapshot() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	members := make(map[string]string, len(r.executorMembers))
	maps.Copy(members, r.executorMembers)
	return members
}
