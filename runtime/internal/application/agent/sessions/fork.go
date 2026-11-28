package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/chat"
)

type ForkSpec struct {
	ParentID         identity.SessionID
	FromRunID        identity.RunID
	Title            string
	ChildID          identity.SessionID
	PrepareWorkspace func(context.Context, ForkSource) (string, func(), error)
}

type ForkSource struct {
	Path     string
	Revision uint64
	RunID    identity.RunID
	Runs     int
	Messages int
}

func forkSource(source session.Session, boundary ForkBoundary) ForkSource {
	return ForkSource{Path: source.Workspace().Path(), Revision: source.Revision(),
		RunID: boundary.RunID, Runs: len(boundary.RunIDs), Messages: len(boundary.Messages)}
}

func (c *Coordinator) InspectFork(ctx context.Context, id identity.SessionID, from identity.RunID) (ForkSource, error) {
	material, err := c.snapshots.ReadForkMaterial(ctx, id)
	if err != nil {
		return ForkSource{}, err
	}
	boundary, err := resolveForkBoundary(material.Messages, material.Runs, from)
	return forkSource(material.Session, boundary), err
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
	if spec.PrepareWorkspace != nil {
		claim, err := c.ClaimSessionMutation(ctx, spec.ParentID)
		if err != nil {
			return err
		}
		defer claim.Release()
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
	childID := spec.ChildID
	if childID.IsZero() {
		childID = c.newID()
	}
	child, err := source.Session.Fork(childID, spec.Title, c.now())
	if err != nil {
		return err
	}
	var expectedRevision uint64
	if spec.PrepareWorkspace != nil {
		path, release, err := spec.PrepareWorkspace(ctx, forkSource(source.Session, boundary))
		if err != nil {
			return err
		}
		defer release()
		workspace, releaseBinding, err := c.bindWorkspace(path)
		if err != nil {
			return err
		}
		defer releaseBinding()
		child, err = child.InstallRestoredWorkspace(workspace)
		if err != nil {
			return err
		}
		expectedRevision = source.Session.Revision()
	}
	forked, err := c.copyForkMaterial(source, child, boundary)
	if err != nil {
		return err
	}
	fork, err := newForkPlan(source.Session, expectedRevision, forked, planReplacement)
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
