package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ChildRunBinding is the application identity assigned to one opaque executor
// child. Product lifecycle observers use Run identity; executor member identity
// remains an implementation detail.
type ChildRunBinding struct {
	MemberID    string
	RunID       string
	ParentRunID string
}

// Validate rejects incomplete or ambiguous child identity before it reaches a
// lifecycle observer.
func (c ChildRunBinding) Validate() error {
	if _, err := runtimeidentity.ParseMember(c.MemberID); err != nil {
		return fmt.Errorf("runs: child Run binding: %w", err)
	}
	if _, err := resourceid.ParseRun(c.RunID); err != nil {
		return fmt.Errorf("runs: child Run binding: %w", err)
	}
	if _, err := resourceid.ParseRun(c.ParentRunID); err != nil {
		return fmt.Errorf("runs: child Run binding parent: %w", err)
	}
	if c.RunID == c.ParentRunID {
		return errors.New("runs: child Run binding refers to itself as parent")
	}
	return nil
}
