package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type treeBarrierReduction struct {
	route      *executorRoute
	next       *reducer
	batch      reductionBatch
	interrupts []transcript.Interrupt
}

type treeBarrierProjection struct {
	pending    Pending
	reductions []treeBarrierReduction
	commits    []EventCommit
}

func (t treePublisher) publishTreeBarrier(
	ctx context.Context,
	routes *executorRoutes,
	barrier TreeInterrupted,
	boundaryAt time.Time,
) error {
	var projection treeBarrierProjection
	committed, err := t.owner.commitInterrupt(ctx, func(interruptCtx context.Context) error {
		prepared, err := t.reduceTreeBarrier(routes, barrier, boundaryAt)
		if err != nil {
			return err
		}
		projection = prepared
		commit, err := newTreeBarrierCommit(
			newRunCommitID(), projection.pending, projection.commits, barrier.Checkpoint(),
		)
		if err != nil {
			return err
		}
		return t.publications.barriers.CommitTreeBarrier(interruptCtx, commit)
	})
	if err != nil {
		return fmt.Errorf("runs: commit tree interrupt barrier: %w", err)
	}
	if !committed {
		return nil
	}
	for _, projected := range projection.reductions {
		projected.route.reducer = projected.next
		projected.route.boundary = boundaryParked
	}
	for _, projected := range projection.reductions {
		for _, reduced := range projected.batch.events {
			if err := t.append(projected.route, reduced); err != nil {
				return err
			}
		}
	}
	for _, projected := range projection.reductions {
		t.publications.publishWaitingMoved(t.rootSpec.SessionID, projected.route.runID)
	}
	return nil
}

func (t treePublisher) reduceTreeBarrier(
	routes *executorRoutes,
	barrier TreeInterrupted,
	boundaryAt time.Time,
) (treeBarrierProjection, error) {
	if routes == nil || routes.root == nil {
		return treeBarrierProjection{}, errors.New("runs: publish tree barrier without a root executor route")
	}
	if err := barrier.validateFor(routes.root.member.MemberID, t.rootSpec.SessionID); err != nil {
		return treeBarrierProjection{}, err
	}
	interruptionsByMemberID, err := activeInterruptionsByMemberID(routes, barrier.Interruptions())
	if err != nil {
		return treeBarrierProjection{}, err
	}
	activeRoutes, err := routes.unfinishedInPostorder()
	if err != nil {
		return treeBarrierProjection{}, err
	}

	projection := treeBarrierProjection{pending: Pending{
		RootRunID:  routes.root.runID,
		ExecutorID: t.rootSpec.ExecutorID,
		CreatedAt:  boundaryAt,
	},
		reductions: make([]treeBarrierReduction, 0, len(activeRoutes)),
		commits:    make([]EventCommit, 0, len(activeRoutes)),
	}
	for _, route := range activeRoutes {
		directInterruptions := interruptionsByMemberID[route.member.MemberID]
		reduction, bindings, continuation, err := t.reduceInterruptedRoute(
			route,
			directInterruptions,
			boundaryAt,
		)
		if err != nil {
			return treeBarrierProjection{}, err
		}
		projection.pending.Interrupts = append(projection.pending.Interrupts, OpenInterruptsOf(reduction.interrupts)...)
		projection.pending.Bindings = append(projection.pending.Bindings, bindings...)
		projection.pending.Continuations = append(projection.pending.Continuations, continuation)
		projection.reductions = append(projection.reductions, reduction)
		projection.commits = append(projection.commits, *reduction.batch.parkCommit)
	}
	if err := projection.pending.Validate(); err != nil {
		return treeBarrierProjection{}, fmt.Errorf("runs: build pending interrupt set: %w", err)
	}
	return projection, nil
}

func activeInterruptionsByMemberID(
	routes *executorRoutes,
	interruptions []MemberInterruption,
) (map[string][]MemberInterruption, error) {
	interruptionsByMemberID := make(map[string][]MemberInterruption, len(interruptions))
	for _, interruption := range interruptions {
		route := routes.byMember[interruption.MemberID]
		if route == nil || route.boundary.closed() {
			return nil, fmt.Errorf(
				"runs: request source member %q has no active Run",
				interruption.MemberID,
			)
		}
		interruptionsByMemberID[interruption.MemberID] = append(
			interruptionsByMemberID[interruption.MemberID],
			interruption,
		)
	}
	return interruptionsByMemberID, nil
}

func (t treePublisher) reduceInterruptedRoute(
	route *executorRoute,
	directInterruptions []MemberInterruption,
	boundaryAt time.Time,
) (treeBarrierReduction, []InterruptBinding, Continuation, error) {
	next := route.reducer.clone()
	var reduced factReduction
	var err error
	if len(directInterruptions) > 0 {
		interrupts := make([]Interrupt, len(directInterruptions))
		for index, interruption := range directInterruptions {
			interrupts[index] = interruption.Interrupt
		}
		reduced, err = next.interrupt(interrupts, route.activeDuration(boundaryAt))
	} else {
		reduced, err = next.suspend(route.activeDuration(boundaryAt))
	}
	if err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: reduce tree barrier for run %q: %w",
			route.runID,
			err,
		)
	}
	batch, err := next.projectFact(reduced)
	if err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: project tree barrier for run %q: %w",
			route.runID,
			err,
		)
	}
	if err := validateRouteReductionBatch(route, t.rootSpec.SessionID, batch); err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, err
	}
	if batch.parkCommit == nil || batch.parkCommit.Run == nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: tree barrier for run %q produced no suspend commit",
			route.runID,
		)
	}
	projectedInterrupts := suspendedInterrupts(reduced.events)
	if len(projectedInterrupts) != len(directInterruptions) {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: run %q projected %d interrupts from %d input requests",
			route.runID,
			len(projectedInterrupts),
			len(directInterruptions),
		)
	}
	bindings := make([]InterruptBinding, len(directInterruptions))
	for index, interruption := range directInterruptions {
		bindings[index] = InterruptBinding{
			InterruptItemID: projectedInterrupts[index].ItemID,
			MemberID:        interruption.MemberID,
			RequestID:       interruption.RequestID,
		}
		if approval, ok := interruption.Interrupt.(ApprovalPrompt); ok {
			bindings[index].ToolCallID = approval.CallID
			bindings[index].ToolRef = approval.Tool
			bindings[index].SourceFingerprint = approval.SourceFingerprint
		}
	}
	continuation := Continuation{
		RunID:        route.runID,
		MemberID:     route.member.MemberID,
		DrainedTools: slices.Clone(next.drained),
	}
	return treeBarrierReduction{route: route, next: next, batch: batch, interrupts: projectedInterrupts}, bindings, continuation, nil
}

func suspendedInterrupts(events []ProjectionEvent) []transcript.Interrupt {
	for _, event := range events {
		if finished, ok := event.(SegmentFinished); ok {
			return slices.Clone(finished.Interrupts)
		}
	}
	return nil
}
