package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type MaterialSnapshot struct {
	SteerInputs []runs.CommandAcceptance
	Session     session.Session
	Items       []transcript.Item
	Runs        []run.Run
	Interrupts  []runs.Pending
	Plan        plan.Current
	Goal        *goal.Goal
}

func (c *Coordinator) MaterialView(ctx context.Context, sessionID identity.SessionID) (MaterialSnapshot, View, error) {
	snapshot, err := c.materialSnapshots.ReadMaterialSnapshot(ctx, sessionID)
	if err != nil {
		return MaterialSnapshot{}, View{}, err
	}
	activity := activityIdle
	for _, value := range snapshot.Runs {
		activity = activity.with(value)
	}
	view, err := c.view(snapshot.Session, activity)
	if err != nil {
		return MaterialSnapshot{}, View{}, err
	}
	return snapshot, view, nil
}

type InterruptSet struct {
	SessionID  identity.SessionID
	Pending    runs.Pending
	Interrupts []transcript.Interrupt
}

func (m MaterialSnapshot) InterruptSets() ([]InterruptSet, error) {
	itemsByID := make(map[identity.ItemID]transcript.Item, len(m.Items))
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

func (m MaterialSnapshot) Run(runID identity.RunID) (run.Run, bool) {
	for _, value := range m.Runs {
		if value.ID() == runID {
			return value, true
		}
	}
	return run.Run{}, false
}
