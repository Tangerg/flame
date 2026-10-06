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

// MaterialSnapshot reads the complete mounted-session projection at one
// database snapshot. No process-local admission is required: concurrent writes
// either precede or follow the storage transaction and can never split the
// returned Run, interrupt, transcript, and Plan facts.
func (c *Coordinator) MaterialSnapshot(ctx context.Context, sessionID string) (MaterialSnapshot, error) {
	return c.materialSnapshots.ReadMaterialSnapshot(ctx, sessionID)
}

// InterruptSet is one open waiting hand-off with its interrupts projected from
// the Items they name.
type InterruptSet struct {
	// SessionID is the Session of the set's root Run, which owns it.
	SessionID  string
	Pending    runs.Pending
	Interrupts []transcript.Interrupt
}

// InterruptSets projects every open hand-off's interrupts from the snapshot's
// Items.
func (m MaterialSnapshot) InterruptSets() ([]InterruptSet, error) {
	itemsByID := make(map[string]transcript.Item, len(m.Items))
	for _, item := range m.Items {
		itemsByID[item.ID()] = item
	}
	sets := make([]InterruptSet, len(m.Interrupts))
	for index, pending := range m.Interrupts {
		interrupts, err := pending.ProjectInterrupts(itemsByID)
		if err != nil {
			return nil, fmt.Errorf("sessions: material snapshot interrupt %q: %w", pending.RootRunID, err)
		}
		sets[index] = InterruptSet{SessionID: m.Session.ID(), Pending: pending, Interrupts: interrupts}
	}
	return sets, nil
}

// Run returns the snapshot's Run with runID; an open interrupt set's root Run
// is always present, and owns the set's capabilities.
func (m MaterialSnapshot) Run(runID string) (run.Run, bool) {
	for _, value := range m.Runs {
		if value.ID() == runID {
			return value, true
		}
	}
	return run.Run{}, false
}
