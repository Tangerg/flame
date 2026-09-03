package run

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// ErrSessionBusy reports that admitting a root Run was rejected because the
// Session already holds a non-terminal root tree. Descendant Runs share that
// tree's admission and do not compete for a second Session slot.
var ErrSessionBusy = errors.New("run: session has a non-terminal root Run")

// Draft is the fresh root or child Run recorded as it enters [Running]. A
// root claims the Session's one non-terminal tree slot; a child carries all
// lineage edges and shares that claim. Streamed segments, usage, and terminal
// Outcome accrue afterward. Executor recovery handles do not belong on the Run
// row; a parked interrupt records its opaque executor member identity when known.
type Draft struct {
	RunID           string
	SessionID       string
	SpawnedByItemID string
	ParentRunID     string
	RootRunID       string
	// SegmentID is the first segment this Run opens with. Admission records it
	// with the Run, because a Running Run without the segment driving it is a Run
	// nothing can attach to.
	SegmentID      string
	ModelSelection modelref.Selection
	// GoalIncarnationID is the autonomous-goal incarnation that admitted this root
	// Run. It is durable admission provenance so crash recovery can terminalize
	// the Run and charge the exact incarnation in one transaction. Child Runs leave it
	// empty because the root is the single Goal Run.
	GoalIncarnationID string
	// Limits is the allowance this Run is admitted under. It is recorded with the
	// admission and never changes: a resume answers an interrupt, it does not
	// renegotiate the budget the Run was accepted with.
	Limits Limits
	// Capabilities is the optional behavior enabled for this Run. Like Limits it
	// is fixed here — and unlike Limits, admission is its ONLY
	// writer: no later transition mentions it, which is how "immutable for the
	// Run's whole life" is kept by construction rather than by a check.
	Capabilities Capabilities
	CreatedAt    time.Time
}

// Validate checks the complete fresh Run value before it enters the lifecycle.
func (d Draft) Validate() error {
	if _, err := resourceid.ParseRun(d.RunID); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if _, err := resourceid.ParseSession(d.SessionID); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if _, err := resourceid.ParseSegment(d.SegmentID); err != nil {
		return fmt.Errorf("run: opening %w", err)
	}
	if _, _, err := goalref.ParseOptionalIncarnation(d.GoalIncarnationID); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	switch {
	case d.CreatedAt.IsZero():
		return errors.New("run: admission time is required")
	}
	lineage := d.Lineage()
	if err := lineage.Validate(d.RunID); err != nil {
		return err
	}
	if err := validateExactModelSelection(d.ModelSelection); err != nil {
		return err
	}
	if err := d.Limits.Validate(); err != nil {
		return err
	}
	if err := d.Capabilities.Validate(); err != nil {
		return err
	}
	if lineage.IsChild() && d.GoalIncarnationID != "" {
		return errors.New("run: child carries a root Goal incarnation")
	}
	return nil
}

func validateExactModelSelection(selection modelref.Selection) error {
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("run: model selection: %w", err)
	}
	if !selection.Configured() {
		return errors.New("run: model selection is required")
	}
	return nil
}

// Lineage returns the draft's immutable root/child identity as one value for
// validation and tree routing.
func (d Draft) Lineage() Lineage {
	return Lineage{
		SpawnedByItemID: d.SpawnedByItemID,
		ParentRunID:     d.ParentRunID,
		RootRunID:       d.RootRunID,
	}
}
