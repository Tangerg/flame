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
	ParentID  identity.SessionID
	FromRunID identity.RunID
	Title     string
}

type ForkSource struct {
	Path     string
	Revision uint64
	RunID    identity.RunID
	Runs     int
	Messages int
}

type forkBasis struct {
	material ForkMaterial
	boundary ForkBoundary
}

func (b forkBasis) source() ForkSource {
	source := b.material.Session
	return ForkSource{Path: source.Workspace().Path(), Revision: source.Revision(),
		RunID: b.boundary.RunID, Runs: len(b.boundary.RunIDs), Messages: len(b.boundary.Messages)}
}

func (c *Coordinator) readForkBasis(ctx context.Context, id identity.SessionID, from identity.RunID) (forkBasis, error) {
	material, err := c.snapshots.ReadForkMaterial(ctx, id)
	if err != nil {
		return forkBasis{}, err
	}
	boundary, err := resolveForkBoundary(material.Messages, material.Runs, from)
	return forkBasis{material: material, boundary: boundary}, err
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
	basis, err := c.readForkBasis(ctx, spec.ParentID, spec.FromRunID)
	if err != nil {
		return err
	}
	fork, err := c.prepareFork(spec, basis, "")
	if err != nil {
		return err
	}
	view, err := c.view(fork.history.Session, activityIdle)
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (View, error) {
		return view, c.writes.ApplyFork(tx, fork)
	}); err != nil {
		return err
	}
	c.publishFork(fork.history, fork.planReplacement != nil)
	return nil
}

func (c *Coordinator) prepareFork(spec ForkSpec, basis forkBasis, checkout string) (ForkPlan, error) {
	source, boundary := basis.material, basis.boundary
	planReplacement, err := c.prepareInitialPlanReplacement(source.planAt(boundary.RunID))
	if err != nil {
		return ForkPlan{}, err
	}
	child, err := source.Session.Fork(c.newID(), spec.Title, c.now())
	if err != nil {
		return ForkPlan{}, err
	}
	var expectedRevision uint64
	if checkout != "" {
		workspace, err := session.NewWorkspace(checkout)
		if err != nil {
			return ForkPlan{}, err
		}
		child, err = child.InstallRestoredWorkspace(workspace)
		if err != nil {
			return ForkPlan{}, err
		}
		expectedRevision = source.Session.Revision()
	}
	forked, err := c.copyForkMaterial(source, child, boundary)
	if err != nil {
		return ForkPlan{}, err
	}
	return newForkPlan(source.Session, expectedRevision, forked, planReplacement)
}

func (c *Coordinator) publishFork(forked HistorySnapshot, hasPlan bool) {
	child := forked.Session.ID()
	c.publishSessionMoved(child)
	if len(forked.Runs) > 0 {
		c.publishRunsMoved(child, forked.Runs)
	}
	if hasPlan {
		c.publishPlanMoved(child)
	}
}
