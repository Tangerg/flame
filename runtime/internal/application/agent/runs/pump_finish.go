package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
)

func (s *segmentPump) fail(err error) {
	if s.ctx.Err() == nil && s.ownerCtx.Err() == nil {
		trace.SpanFromContext(s.ctx).RecordError(err)
		s.routes.abortUnfinished(err)
	}
}

func (s *segmentPump) finish() {
	<-s.owner.activation.done
	s.owner.observation.Lock()
	defer s.owner.observation.Unlock()
	// Deferred so a terminal projection that cannot complete still ends the Segment
	// for waiting clients.
	defer s.finishBoundary()
	defer s.releaseExecutorTree()
	for memberID, managed := range s.childStarts {
		if !managed.outcome.Valid() {
			s.abortPreparedChildStart(managed.prepared)
		}
		delete(s.childStarts, memberID)
	}
	if !s.routes.root.boundary.finished() {
		s.synthesizeUnfinished()
	}
}

func (s *segmentPump) releaseExecutorTree() {
	if !s.routes.root.boundary.parked() {
		s.tearDownExecutor()
	}
}

// Children close in canonical postorder and the root only if every child did, so
// persistence never shows a closed tree with a live descendant.
func (s *segmentPump) synthesizeUnfinished() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	ordered, err := s.routes.unfinishedInPostorder()
	childrenClosed := err == nil
	if err != nil {
		s.fail(fmt.Errorf("runs: order unfinished tree for terminal synthesis: %w", err))
	}
	for _, route := range ordered {
		if route != s.routes.root {
			childrenClosed = s.synthesizeRoute(ctx, route) && childrenClosed
		}
	}
	if childrenClosed && !s.routes.root.boundary.finished() {
		s.synthesizeRoute(ctx, s.routes.root)
	}
}

func (s *segmentPump) synthesizeRoute(ctx context.Context, route *executorRoute) bool {
	var reductions reductionBatch
	var cancellation CancellationID
	var err error
	if route == s.routes.root && s.deferredRootTerminal != nil {
		fact, id := s.owner.projectCancellation(s.deferredRootTerminal, route.lineage)
		cancellation = id
		reductions, err = route.reducer.reduce(fact)
	} else {
		var reason string
		if route == s.routes.root {
			if intent := s.owner.suppressedActivationCancellation(); intent != nil {
				cancellation, reason = intent.id, intent.reason
			}
		}
		reductions, err = route.reducer.synthesizeTerminal(reason)
	}
	if err != nil {
		s.fail(err)
		return false
	}
	publication, err := s.publisher.publishTerminalAtomically(ctx, route, reductions, abortOnFailure, cancellation)
	if err != nil {
		s.fail(err)
		return false
	}
	if !publication.finished() || publication.parked() {
		s.fail(fmt.Errorf(
			"runs: synthesized terminal for run %q produced finished=%t parked=%t",
			route.runID,
			publication.finished(),
			publication.parked(),
		))
		return false
	}
	route.boundary = boundaryFinished
	return true
}

func (s *segmentPump) tearDownExecutor() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	if err := s.segments.releases.Release(ctx, s.spec.executorRef()); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		s.owner.completionErr = fmt.Errorf("runs: tear down executor %q: %w", s.spec.ExecutorID, err)
		recordRunCleanupError(ctx, s.owner.completionErr)
	}
}

func (s *segmentPump) finishBoundary() {
	defer s.segments.registry.RemoveSegment(s.spec.RunID, s.spec.SegmentID)
	defer s.closeJournal()
	s.settleMaintenanceFence()
}

func (s *segmentPump) closeJournal() {
	if err := s.owner.hub.close(); err != nil {
		s.owner.completionErr = fmt.Errorf("runs: close replay journal: %w", err)
		recordRunCleanupError(s.ownerCtx, s.owner.completionErr)
	}
}

// Maintenance runs while admission still holds the Session and working tree so a
// following Run cannot write into this Run's snapshot.
func (s *segmentPump) settleMaintenanceFence() {
	releaseMaintenance, maintenanceHeld := s.segments.admission.BeginMaintenance(s.spec.RunID)
	if maintenanceHeld {
		defer releaseMaintenance()
	}
	entry, tracked := s.segments.registry.Get(s.spec.RunID)
	if tracked && !s.routes.root.boundary.parked() {
		entry.owner.stop()
	}
	if !s.routes.root.boundary.finished() {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	finish := Finish{
		SessionID: s.spec.SessionID, RunID: s.spec.RunID, WorkspaceCWD: s.spec.WorkspaceCWD,
		Parked: s.routes.root.boundary.parked(),
	}
	if maintenanceHeld {
		finish.OpeningUserText = s.spec.openingUserText()
	}
	if err := s.segments.finalizer.Finish(ctx, finish); err != nil {
		recordRunCleanupError(ctx, err)
	}
}

func recordRunCleanupError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	slog.ErrorContext(ctx, "runs: Run cleanup failed", telemetry.Error(err))
	trace.SpanFromContext(ctx).RecordError(err)
}
