package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ChildRunBinding is the application identity assigned to one opaque executor
// child. Product lifecycle observers use Run identity; executor member identity
// remains an implementation detail.
type ChildRunBinding struct {
	MemberID    string
	RunID       identity.RunID
	ParentRunID identity.RunID
}

// Validate rejects incomplete or ambiguous child identity before it reaches a
// lifecycle observer.
func (c ChildRunBinding) Validate() error {
	if err := ValidateMember(c.MemberID); err != nil {
		return fmt.Errorf("runs: child Run binding: %w", err)
	}
	if err := c.RunID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run binding: %w", err)
	}
	if err := c.ParentRunID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run binding parent: %w", err)
	}
	if c.RunID == c.ParentRunID {
		return errors.New("runs: child Run binding refers to itself as parent")
	}
	return nil
}
