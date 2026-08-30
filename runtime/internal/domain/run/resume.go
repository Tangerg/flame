package run

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// ResumeDraft is one parked Run whose fresh continuation Segment is opening.
// It is always applied as part of a root-owned [TreeResumeDraft]; a descendant
// cannot resume independently of the barrier that suspended the complete tree.
type ResumeDraft struct {
	RunID string
	// SegmentID is the continuation's fresh segment, which replaces the one the
	// park cleared — in the same transaction that moves the Run back to Running.
	SegmentID string
}

// TreeResumeDraft is the complete durable identity set reopened after one
// accepted answer claim. Runs is canonical postorder (descendants before
// ancestors, siblings by Run ID, root last), matching the already-claimed
// Pending continuation set.
type TreeResumeDraft struct {
	RootRunID string
	SessionID string
	// ResumedAt is the single tree-opening timestamp used by every Run row.
	// Recording it on the draft preserves the exact committed
	// root snapshot instead of approximating a store-owned clock.
	ResumedAt time.Time
	Runs      []ResumeDraft
}

// Validate checks the tree-resume identity frame. Topology and exact postorder
// correspondence are checked while the owner creates the draft; persistence
// additionally proves that its root has a durable answer claim before
// reopening any Run.
func (t TreeResumeDraft) Validate() error {
	if _, err := resourceid.ParseRun(t.RootRunID); err != nil {
		return fmt.Errorf("run: tree resume root %w", err)
	}
	if _, err := resourceid.ParseSession(t.SessionID); err != nil {
		return fmt.Errorf("run: tree resume %w", err)
	}
	switch {
	case t.ResumedAt.IsZero():
		return errors.New("run: tree resume time is required")
	case len(t.Runs) == 0:
		return errors.New("run: tree resume has no Runs")
	}
	seen := make(map[string]struct{}, len(t.Runs))
	for index, run := range t.Runs {
		if _, err := resourceid.ParseRun(run.RunID); err != nil {
			return fmt.Errorf("run: tree resume Run[%d]: %w", index, err)
		}
		if _, err := resourceid.ParseSegment(run.SegmentID); err != nil {
			return fmt.Errorf("run: tree resume Run[%d]: %w", index, err)
		}
		if _, duplicate := seen[run.RunID]; duplicate {
			return fmt.Errorf("run: tree resume repeats Run %q", run.RunID)
		}
		seen[run.RunID] = struct{}{}
	}
	if t.Runs[len(t.Runs)-1].RunID != t.RootRunID {
		return fmt.Errorf(
			"run: tree resume root %q must be the final Run, got %q",
			t.RootRunID,
			t.Runs[len(t.Runs)-1].RunID,
		)
	}
	return nil
}
