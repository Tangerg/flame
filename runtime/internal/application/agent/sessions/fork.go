package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// ForkSpec describes where a session fork should branch.
type ForkSpec struct {
	ParentID  string
	FromRunID string
	Title     string
}

// ForkBoundary is where a fork branches: the parent history prefix the child is
// seeded with, and the run that prefix stops at. They travel together because the
// child's Plan is copied from that same run's boundary — a fork
// whose conversation and Plan came from different runs would hand the branch
// a plan that never went with what it remembers.
type ForkBoundary struct {
	Messages []chat.Message
	// RunIDs are the terminal Run facts whose user-visible transcript belongs to
	// Messages. A fork remaps them into the child instead of leaving the model with
	// context the client cannot see through runs.list/items.list.
	RunIDs []string
	// RunID is the boundary run, empty when the parent has no terminal run to
	// branch from (nothing to copy).
	RunID string
}

// ResolveForkBoundary applies a durable run boundary to parent history.
// Non-terminal runs never contribute messages: their current tail can still
// change and therefore is not a portable fork boundary. An explicit target
// must itself be terminal; an implicit whole-conversation fork stops at the
// latest terminal run.
func ResolveForkBoundary(msgs []chat.Message, runs []run.Run, fromRunID string) (ForkBoundary, error) {
	if fromRunID != "" {
		if _, err := resourceid.ParseRun(fromRunID); err != nil {
			return ForkBoundary{}, fmt.Errorf("sessions: fork boundary: %w", err)
		}
	}
	for _, run := range runs {
		if run.State().IsTerminal() && (run.MessageMark() < 0 || run.MessageMark() > len(msgs)) {
			return ForkBoundary{}, fmt.Errorf("sessions: terminal run %q has invalid message watermark %d", run.ID(), run.MessageMark())
		}
	}
	boundary, err := transcript.TimelineFromRuns(runs).PortableBoundaryAt(fromRunID)
	if err != nil {
		return ForkBoundary{}, err
	}
	return ForkBoundary{
		Messages: cloneSnapshotMessages(msgs[:boundary.KeepMessageMark]),
		RunIDs:   boundary.RunIDs,
		RunID:    boundary.KeepRunID,
	}, nil
}

// Fork creates a child session, seeds it with the resolved parent history prefix
// and the Plan that boundary held, and renames it as one atomic write-set.
// The application resolves the boundary and commits the branch through
// its persistence port.
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	if _, err := resourceid.ParseSession(spec.ParentID); err != nil {
		return session.Session{}, fmt.Errorf("sessions: fork: %w", err)
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	if err := snapshot.Session.ValidateFor(spec.ParentID); err != nil {
		return session.Session{}, fmt.Errorf("sessions: fork snapshot identity: %w", err)
	}
	boundary, err := ResolveForkBoundary(snapshot.Messages, snapshot.Runs, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}
	// A child starts fresh, so an unrecorded boundary and a recorded empty one seed
	// the same nothing — the distinction a rollback needs has no branch to make here.
	plan, err := c.planBoundary(ctx, boundary.RunID)
	if err != nil {
		return session.Session{}, err
	}
	planReplacement, err := c.prepareInitialPlanReplacement(plan.Steps)
	if err != nil {
		return session.Session{}, err
	}
	child, err := snapshot.Session.Fork(c.newID(), spec.Title, c.now())
	if err != nil {
		return session.Session{}, err
	}
	forked, err := c.copyForkSnapshot(snapshot, child, boundary, plan.Steps)
	if err != nil {
		return session.Session{}, err
	}
	fork, err := NewForkPlan(spec.ParentID, forked, planReplacement)
	if err != nil {
		return session.Session{}, err
	}
	if _, err := c.writes.ApplyFork(ctx, fork); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(child.ID())
	if len(forked.Runs) > 0 {
		c.publishRunsMoved(child.ID(), forked.Runs)
	}
	if planReplacement != nil {
		c.publishPlanMoved(child.ID())
	}
	return child, nil
}
