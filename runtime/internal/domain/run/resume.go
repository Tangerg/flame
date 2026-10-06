package run

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// TreeResumeDraft is the complete set of parked Runs reopened after one
// accepted answer claim, each as the Replacement that resumes it into its fresh
// continuation Segment. Runs is canonical postorder (descendants before
// ancestors, siblings by Run ID, root last), matching the already-claimed
// Pending continuation set. The root Run names the tree and its Session, and
// every Run reopens at the root's resume time.
type TreeResumeDraft struct {
	Runs []Replacement
}

// RootRunID names the tree's root, the final resumed Run.
func (t TreeResumeDraft) RootRunID() string {
	if len(t.Runs) == 0 {
		return ""
	}
	return t.Runs[len(t.Runs)-1].State().ID()
}

// SessionID names the Session that owns the resumed tree.
func (t TreeResumeDraft) SessionID() string {
	if len(t.Runs) == 0 {
		return ""
	}
	return t.Runs[len(t.Runs)-1].State().SessionID()
}

// ValidateForTree verifies the draft and both identities that scope it. A
// commit reasoning about one Session's root Run needs both, and asking for them
// together is what stops one being checked while the other is assumed.
func (t TreeResumeDraft) ValidateForTree(expectedSessionID, expectedRootRunID string) error {
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

// Validate checks that every Run moves from Waiting to Running inside the
// root's tree and Session at one resume time. Topology and exact postorder
// correspondence are checked while the owner creates the draft; persistence
// additionally proves that its root has a durable answer claim before
// reopening any Run.
func (t TreeResumeDraft) Validate() error {
	if len(t.Runs) == 0 {
		return errors.New("run: tree resume has no Runs")
	}
	root := t.Runs[len(t.Runs)-1].State()
	if !root.Lineage().IsRoot() {
		return fmt.Errorf("run: tree resume final Run %q is not a root", root.ID())
	}
	seen := make(map[string]struct{}, len(t.Runs))
	for index, replacement := range t.Runs {
		if err := replacement.Validate(); err != nil {
			return fmt.Errorf("run: tree resume Run[%d]: %w", index, err)
		}
		resumed := replacement.State()
		if replacement.Expected().State() != Waiting || resumed.State() != Running {
			return fmt.Errorf("run: tree resume Run %q does not move from waiting to running", resumed.ID())
		}
		if err := resourceid.ValidateSegment(resumed.ActiveSegmentID()); err != nil {
			return fmt.Errorf("run: tree resume Run %q: %w", resumed.ID(), err)
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
