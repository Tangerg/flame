package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// MaterialSnapshot is the coherent durable state needed to reconstruct one
// mounted Session. It is a live read model, so active Runs and open interrupts
// are valid members and Plan revision metadata is retained.
type MaterialSnapshot struct {
	Session    session.Session
	Items      []transcript.Item
	Runs       []run.Run
	Interrupts []runs.Pending
	Plan       plan.Current
	Goal       *goal.Goal
}

// SnapshotView projects one storage snapshot for a mounted Session. Session
// activity comes from these Runs; workspace availability is a live observation
// of the same stored workspace identity.
type SnapshotView struct {
	Session    View
	Items      []transcript.Item
	Runs       []run.Run
	Interrupts []runs.Pending
	Plan       plan.Current
	Goal       *goal.Goal
}

// MaterialSnapshot reads and projects the complete mounted-session state from
// one database snapshot. Concurrent writes cannot split Session metadata from
// the returned Run, interrupt, transcript, Plan, and Goal facts.
func (c *Coordinator) MaterialSnapshot(ctx context.Context, sessionID string) (SnapshotView, error) {
	snapshot, err := c.materialSnapshots.ReadMaterialSnapshot(ctx, sessionID)
	if err != nil {
		return SnapshotView{}, err
	}
	if err := snapshot.Session.ValidateFor(sessionID); err != nil {
		return SnapshotView{}, fmt.Errorf("sessions: material snapshot identity: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return SnapshotView{}, err
	}
	activity := ActivityIdle
	for _, record := range snapshot.Runs {
		activity = resolveActivity(activity, record.State())
	}
	view, err := c.view(snapshot.Session, activity)
	if err != nil {
		return SnapshotView{}, err
	}
	return SnapshotView{
		Session: view, Items: snapshot.Items, Runs: snapshot.Runs,
		Interrupts: snapshot.Interrupts, Plan: snapshot.Plan, Goal: snapshot.Goal,
	}, nil
}

// Validate checks the cross-projection identities a storage transaction must
// preserve before the snapshot crosses the Application boundary.
func (m MaterialSnapshot) Validate() error {
	validator, err := newMaterialSnapshotValidator(m)
	if err != nil {
		return err
	}
	if err := validator.indexRuns(); err != nil {
		return err
	}
	if err := validator.indexItems(); err != nil {
		return err
	}
	if err := validateSnapshotRunTree(m.Runs, validator.itemsByID); err != nil {
		return fmt.Errorf("sessions: material snapshot Run tree: %w", err)
	}
	if err := validator.indexInterrupts(); err != nil {
		return err
	}
	if err := validator.validateWaitingOwnership(); err != nil {
		return err
	}
	return validator.validateGoal()
}

type materialSnapshotValidator struct {
	snapshot         MaterialSnapshot
	sessionID        string
	runsByID         map[string]run.Run
	itemsByID        map[string]transcript.Item
	interruptsByRoot map[string]struct{}
}

func newMaterialSnapshotValidator(snapshot MaterialSnapshot) (*materialSnapshotValidator, error) {
	if snapshot.Session.IsZero() {
		return nil, fmt.Errorf("sessions: session is required")
	}
	return &materialSnapshotValidator{
		snapshot:         snapshot,
		sessionID:        snapshot.Session.ID(),
		runsByID:         make(map[string]run.Run, len(snapshot.Runs)),
		itemsByID:        make(map[string]transcript.Item, len(snapshot.Items)),
		interruptsByRoot: make(map[string]struct{}, len(snapshot.Interrupts)),
	}, nil
}

func (validator *materialSnapshotValidator) indexRuns() error {
	for _, record := range validator.snapshot.Runs {
		if record.SessionID() != validator.sessionID {
			return fmt.Errorf("sessions: material snapshot Run %q belongs to Session %q, want %q", record.ID(), record.SessionID(), validator.sessionID)
		}
		if _, duplicate := validator.runsByID[record.ID()]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats Run %q", record.ID())
		}
		validator.runsByID[record.ID()] = record
	}
	return nil
}

func (validator *materialSnapshotValidator) indexItems() error {
	for _, item := range validator.snapshot.Items {
		if item.SessionID() != validator.sessionID {
			return fmt.Errorf("sessions: material snapshot Item %q belongs to Session %q, want %q", item.ID(), item.SessionID(), validator.sessionID)
		}
		owner, found := validator.runsByID[item.RunID()]
		if !found {
			return fmt.Errorf("sessions: material snapshot Item %q references unknown Run %q", item.ID(), item.RunID())
		}
		if item.Status() == transcript.ItemRunning && owner.State().IsTerminal() {
			return fmt.Errorf("sessions: material snapshot terminal Run Item %q is still running", item.ID())
		}
		if _, duplicate := validator.itemsByID[item.ID()]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats Item %q", item.ID())
		}
		validator.itemsByID[item.ID()] = item
	}
	return nil
}

func (validator *materialSnapshotValidator) indexInterrupts() error {
	for _, pending := range validator.snapshot.Interrupts {
		if err := pending.ValidateProjection(validator.snapshot.Runs, validator.snapshot.Items); err != nil {
			return fmt.Errorf("sessions: material snapshot interrupt %q: %w", pending.RootRunID, err)
		}
		if pending.SessionID != validator.sessionID {
			return fmt.Errorf("sessions: material snapshot interrupt %q belongs to Session %q, want %q", pending.RootRunID, pending.SessionID, validator.sessionID)
		}
		root, found := validator.runsByID[pending.RootRunID]
		if !found {
			return fmt.Errorf("sessions: material snapshot interrupt references unknown root Run %q", pending.RootRunID)
		}
		if root.Lineage().IsChild() || root.State() != run.Waiting {
			return fmt.Errorf("sessions: material snapshot interrupt Run %q is not a waiting root", pending.RootRunID)
		}
		if _, duplicate := validator.interruptsByRoot[pending.RootRunID]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats interrupt root %q", pending.RootRunID)
		}
		validator.interruptsByRoot[pending.RootRunID] = struct{}{}
	}
	return nil
}

func (validator *materialSnapshotValidator) validateWaitingOwnership() error {
	for _, record := range validator.snapshot.Runs {
		if record.State() != run.Waiting {
			continue
		}
		rootRunID := record.Lineage().TreeRootID(record.ID())
		if _, found := validator.interruptsByRoot[rootRunID]; !found {
			return fmt.Errorf(
				"sessions: material snapshot waiting Run %q has no Pending owner for root %q",
				record.ID(),
				rootRunID,
			)
		}
	}
	return nil
}

func (validator *materialSnapshotValidator) validateGoal() error {
	if validator.snapshot.Goal != nil {
		if err := validator.snapshot.Goal.ValidateSnapshot(); err != nil {
			return fmt.Errorf("sessions: material snapshot Goal: %w", err)
		}
		if validator.snapshot.Goal.SessionID() != validator.sessionID {
			return fmt.Errorf(
				"sessions: material snapshot Goal belongs to Session %q, want %q",
				validator.snapshot.Goal.SessionID(),
				validator.sessionID,
			)
		}
	}
	return nil
}
