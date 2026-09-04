package goals

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

// Store is the autonomous-goal use case's durable state. It is deliberately
// owned here: the domain owns goal values and invariants, while application
// workflows decide when those values are read, persisted, cleared, or
// reconciled.
type Store interface {
	Get(ctx context.Context, sessionID string) (goal.Current, error)
	// Save executes the domain-decided next durable revision. Expected is the
	// sole CAS authority; persistence never assigns or rewrites Goal identity.
	// A lost compare-and-swap returns applied=false.
	Save(ctx context.Context, g goal.Goal, expected goal.Version) (saved goal.Goal, applied bool, err error)
	ClearIf(ctx context.Context, sessionID string, expected goal.Version) (applied bool, err error)
	List(ctx context.Context) ([]goal.Goal, error)
}

func loadGoal(ctx context.Context, store Store, sessionID string) (goal.Goal, bool, error) {
	current, err := store.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, false, err
	}
	if err := current.Validate(); err != nil {
		return goal.Goal{}, false, fmt.Errorf("goals: store Get(%q) returned invalid Current: %w", sessionID, err)
	}
	if current.SessionID() != sessionID {
		return goal.Goal{}, false, fmt.Errorf(
			"goals: store Get(%q) returned Session %q",
			sessionID,
			current.SessionID(),
		)
	}
	value, exists := current.Goal()
	return value, exists, nil
}

func validateGoalCatalog(values []goal.Goal) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := value.ValidateSnapshot(); err != nil {
			return fmt.Errorf("goals: store List item[%d] is invalid: %w", index, err)
		}
		if _, duplicate := seen[value.SessionID()]; duplicate {
			return fmt.Errorf("goals: store List returned duplicate Session %q", value.SessionID())
		}
		seen[value.SessionID()] = struct{}{}
	}
	return nil
}

// RunRecorder records one terminal goal-owned Run exactly once. It joins the
// terminal Run transaction, rather than asking the drive to reconstruct durable
// accounting after it has observed a streamed terminal event.
type RunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// DurableStore is the complete persistence surface required by a Run
// terminalizer. The Driver, Reader, and OutcomeReporter consume Store.
type DurableStore interface {
	Store
	RunRecorder
	// Clear is reserved for the session aggregate's atomic delete write-set.
	// Goal lifecycle and boot recovery use versioned ClearIf instead.
	Clear(ctx context.Context, sessionID string) error
}
