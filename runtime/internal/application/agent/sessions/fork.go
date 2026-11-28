package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/chat"
)

type ForkSpec struct {
	ParentID  identity.SessionID
	FromRunID identity.RunID
	Title     string
}

type RunPlanBoundary struct {
	RunID    identity.RunID
	Document plan.Document
}

type ForkMaterial struct {
	HistorySnapshot
	PlanBoundaries []RunPlanBoundary
}

func (m ForkMaterial) planAt(runID identity.RunID) plan.Document {
	if runID.IsZero() {
		return plan.Document{Steps: []plan.Step{}}
	}
	for _, boundary := range m.PlanBoundaries {
		if boundary.RunID == runID {
			return boundary.Document
		}
	}
	return plan.Document{}
}

type ForkBoundary struct {
	Messages []chat.Message
	RunIDs   []identity.RunID
	RunID    identity.RunID
}

func resolveForkBoundary(msgs []chat.Message, runs []run.Run, fromRunID identity.RunID) (ForkBoundary, error) {
	for _, run := range runs {
		if !run.State().IsTerminal() || !run.Lineage().IsRoot() {
			continue
		}
		if count, known := run.MessageMark().Count(); !known || count > len(msgs) {
			return ForkBoundary{}, fmt.Errorf("sessions: terminal run %q has invalid message watermark %s", run.ID(), run.MessageMark())
		}
	}
	boundary, err := transcript.TimelineFromRuns(runs).PortableBoundaryAt(fromRunID)
	if err != nil {
		return ForkBoundary{}, err
	}
	keep, known := boundary.KeepMessageMark.Count()
	if !known {
		return ForkBoundary{}, fmt.Errorf("sessions: fork boundary at %q has no message watermark", boundary.KeepRunID)
	}
	return ForkBoundary{
		Messages: cloneSnapshotMessages(msgs[:keep]),
		RunIDs:   boundary.RunIDs,
		RunID:    boundary.KeepRunID,
	}, nil
}

func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec, complete operation.Reply[View]) error {
	if err := spec.ParentID.Validate(); err != nil {
		return fmt.Errorf("sessions: fork: %w", err)
	}
	source, err := c.snapshots.ReadForkMaterial(ctx, spec.ParentID)
	if err != nil {
		return err
	}
	boundary, err := resolveForkBoundary(source.Messages, source.Runs, spec.FromRunID)
	if err != nil {
		return err
	}
	planReplacement, err := c.prepareInitialPlanReplacement(source.planAt(boundary.RunID))
	if err != nil {
		return err
	}
	child, err := source.Session.Fork(c.newID(), spec.Title, c.now())
	if err != nil {
		return err
	}
	forked, err := c.copyForkMaterial(source, child, boundary)
	if err != nil {
		return err
	}
	fork, err := newForkPlan(spec.ParentID, forked, planReplacement)
	if err != nil {
		return err
	}
	view, err := c.view(child, activityIdle)
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (View, error) {
		return view, c.writes.ApplyFork(tx, fork)
	}); err != nil {
		return err
	}
	c.publishSessionMoved(child.ID())
	if len(forked.Runs) > 0 {
		c.publishRunsMoved(child.ID(), forked.Runs)
	}
	if planReplacement != nil {
		c.publishPlanMoved(child.ID())
	}
	return nil
}
