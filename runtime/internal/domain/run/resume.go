package run

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type TreeResumeDraft struct {
	Runs []Replacement
}

func (t TreeResumeDraft) RootRunID() identity.RunID {
	if len(t.Runs) == 0 {
		return identity.RunID{}
	}
	return t.Runs[len(t.Runs)-1].State().ID()
}

func (t TreeResumeDraft) SessionID() identity.SessionID {
	if len(t.Runs) == 0 {
		return identity.SessionID{}
	}
	return t.Runs[len(t.Runs)-1].State().SessionID()
}

func (t TreeResumeDraft) ValidateForTree(expectedSessionID identity.SessionID, expectedRootRunID identity.RunID) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.SessionID() != expectedSessionID {
		return fmt.Errorf(
			"run: tree resume Session %q does not match requested identity %q",
			t.SessionID(),
			expectedSessionID,
		)
	}
	if t.RootRunID() != expectedRootRunID {
		return fmt.Errorf(
			"run: tree resume root %q does not match requested identity %q",
			t.RootRunID(),
			expectedRootRunID,
		)
	}
	return nil
}

func (t TreeResumeDraft) Validate() error {
	if len(t.Runs) == 0 {
		return errors.New("run: tree resume has no Runs")
	}
	root := t.Runs[len(t.Runs)-1].State()
	if !root.Lineage().IsRoot() {
		return fmt.Errorf("run: tree resume final Run %q is not a root", root.ID())
	}
	seen := make(map[identity.RunID]struct{}, len(t.Runs))
	for index, replacement := range t.Runs {
		if err := replacement.Validate(); err != nil {
			return fmt.Errorf("run: tree resume Run[%d]: %w", index, err)
		}
		resumed := replacement.State()
		if replacement.Expected().State() != Waiting || resumed.State() != Running {
			return fmt.Errorf("run: tree resume Run %q does not move from waiting to running", resumed.ID())
		}
		if resumed.SessionID() != root.SessionID() || resumed.Lineage().TreeRootID(resumed.ID()) != root.ID() {
			return fmt.Errorf("run: tree resume Run %q is outside root %q", resumed.ID(), root.ID())
		}
		if !resumed.UpdatedAt().Equal(root.UpdatedAt()) {
			return fmt.Errorf("run: tree resume Run %q reopens at another time than its root", resumed.ID())
		}
		if _, duplicate := seen[resumed.ID()]; duplicate {
			return fmt.Errorf("run: tree resume repeats Run %q", resumed.ID())
		}
		seen[resumed.ID()] = struct{}{}
	}
	return nil
}
