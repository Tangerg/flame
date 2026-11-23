package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
)

func (c *Segments) pump(
	ctx context.Context,
	ownerCtx context.Context,
	spec segmentSpec,
	executorEvents iter.Seq[ExecutorEvent],
	owner *runTreeOwner,
	routes *executorRoutes,
	initialErr error,
) {
	pump := &segmentPump{
		segments: c,
		ctx:      ctx,
		ownerCtx: ownerCtx,
		spec:     spec,
		events:   executorEvents,
		owner:    owner,
		routes:   routes,
	}
	pump.publisher = treePublisher{publications: &c.publications, rootSpec: spec, owner: owner}
	defer pump.recoverAbandonedRun()
	pump.run(initialErr)
}

type segmentPump struct {
	segments  *Segments
	ctx       context.Context
	ownerCtx  context.Context
	spec      segmentSpec
	events    iter.Seq[ExecutorEvent]
	owner     *runTreeOwner
	routes    *executorRoutes
	publisher treePublisher

	childStarts          map[string]*managedChildStart
	deferredRootTerminal ExecutionFact
}

type managedChildStart struct {
	prepared *preparedChildStart
	outcome  ChildRunStartOutcome
}

func (s *segmentPump) run(initialErr error) {
	defer close(s.owner.done)
	defer s.finish()
	defer s.recoverProjectionDefect()
	if initialErr != nil {
		s.fail(initialErr)
		return
	}
	for event := range s.events {
		if !s.processEvent(event) {
			return
		}
	}
}

func refuseAfterFinish(payload executorPayload) {
	switch pending := payload.(type) {
	case ExecutionFactCommit:
		pending.Complete(ErrRunFinished)
	case ChildRunReservationRequest:
		if claimed, ok := pending.claim(); ok {
			claimed.refuse(ErrRunFinished)
		}
	}
}

// A rejected write still yields the executor's settlement; stopping here would
// tear down the only source able to report it.
func (s *segmentPump) commitAuthoritative(member ExecutorMember, commit ExecutionFactCommit) bool {
	if err := commit.validate(); err != nil {
		commit.Complete(err)
		s.fail(err)
		return false
	}
	fact := commit.Fact()
	err := s.handleAuthoritativeFact(member, fact)
	commit.Complete(err)
	if err == nil && s.routes.root.boundary.finished() {
		// Scope still has to acknowledge its Exit append and close the
		// stream before the Run owner can release that writer.
		_, settlingTree := fact.(ExecutionTreeSettled)
		return settlingTree
	}
	return true
}

func (s *segmentPump) processEvent(event ExecutorEvent) bool {
	s.owner.observation.Lock()
	defer s.owner.observation.Unlock()
	if s.routes.root.boundary.finished() {
		refuseAfterFinish(event.Payload)
		return true
	}
	if request, reserving := event.Payload.(ChildRunReservationRequest); reserving {
		return s.handleChildRunReservation(event, request)
	}
	if err := event.Validate(); err != nil {
		s.fail(err)
		return false
	}
	if commit, authoritative := event.Payload.(ExecutionFactCommit); authoritative {
		return s.commitAuthoritative(event.Member, commit)
	}
	if barrier, interrupted := event.Payload.(TreeInterrupted); interrupted {
		return s.handleTreeBarrier(event, barrier)
	}
	executionFact, ok := event.Payload.(ExecutionFact)
	if !ok {
		s.fail(fmt.Errorf("runs: unsupported executor payload %T", event.Payload))
		return false
	}
	if compacted, ok := executionFact.(CompactionBoundary); ok && compacted.Rewrite != nil {
		s.fail(errors.New("runs: durable compaction requires a commit receipt"))
		return false
	}
	if _, unacknowledged := executionFact.(ToolResultsCommitted); unacknowledged {
		s.fail(errors.New("runs: Tool result publication requires a commit receipt"))
		return false
	}
	keep, err := s.handleExecutionFact(event.Member, executionFact)
	if err != nil {
		s.fail(err)
		return false
	}
	return keep
}

func (s *segmentPump) handleChildRunReservation(
	event ExecutorEvent,
	request ChildRunReservationRequest,
) bool {
	if err := event.Validate(); err != nil {
		if claimed, ok := request.claim(); ok {
			claimed.refuse(err)
		}
		s.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		s.fail(err)
		return false
	}
	claimed, ok := request.claim()
	if !ok {
		return true
	}
	if existing := s.childStarts[event.Member.MemberID]; existing != nil {
		if existing.prepared.member != event.Member {
			err := fmt.Errorf("runs: child member %q repeated a different start reservation", event.Member.MemberID)
			claimed.refuse(err)
			return true
		}
		claimed.grant(existing.prepared.reservation.Binding)
		return true
	}
	if s.activeChildReservations(event.Member.ParentID) >= MaxActiveChildRuns {
		claimed.refuse(errors.New("runs: active child Run limit reached"))
		return true
	}
	prepared, err := s.segments.prepareChildStart(
		s.spec, s.owner, s.routes, event.Member,
	)
	if err == nil {
		err = s.segments.childStarts.ReserveChildRunStart(s.ownerCtx, prepared.reservation)
	}
	if err != nil {
		if prepared != nil {
			prepared.releaseBinding(s.owner)
		}
		claimed.refuse(err)
		return true
	}
	if s.childStarts == nil {
		s.childStarts = make(map[string]*managedChildStart)
	}
	s.childStarts[event.Member.MemberID] = &managedChildStart{prepared: prepared}
	claimed.grant(prepared.reservation.Binding)
	return true
}

func (s *segmentPump) abortPreparedChildStart(prepared *preparedChildStart) {
	if prepared == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	recordRunCleanupError(ctx, s.segments.childStarts.AbortChildRunStart(ctx, prepared.reservation))
	prepared.releaseBinding(s.owner)
}

func (s *segmentPump) authoritativeRoute(member ExecutorMember, fact ExecutionFact) (*executorRoute, error) {
	route, err := s.routes.resolve(member)
	if err != nil {
		return nil, err
	}
	if prepared, ok := fact.(ToolCallPrepared); ok {
		if err := prepared.Context.ValidateOwnership(s.routes.root.member.MemberID, s.spec.SessionID); err != nil {
			return nil, err
		}
	}
	if route.member.MemberID != "" {
		if err := s.owner.bindExecutorMember(route.runID, route.member.MemberID); err != nil {
			return nil, err
		}
	}
	if route.reducer == nil {
		return nil, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	return route, nil
}

func (s *segmentPump) handleAuthoritativeFact(member ExecutorMember, fact ExecutionFact) error {
	if tree, ok := fact.(ExecutionTreeSettled); ok {
		return s.commitExecutionTree(tree)
	}
	route, err := s.authoritativeRoute(member, fact)
	if err != nil {
		return err
	}
	fact = s.classifyChildCancellationFact(route, fact, nil)
	if engineEventEndsSegment(fact) {
		if route == s.routes.root && s.routes.unfinishedCount() > 1 {
			return errors.New("runs: authoritative root terminal precedes children")
		}
		_, err := s.projectFact(route, fact)
		return err
	}
	speculative := route.reducer.clone()
	batch, err := speculative.reduce(fact)
	if err != nil {
		return err
	}
	if compacted, ok := fact.(CompactionBoundary); ok && compacted.Rewrite != nil {
		if err := s.attachCompactionPlan(*compacted.Rewrite, &batch); err != nil {
			return err
		}
	}
	publication, err := s.publisher.publishAuthoritativeAtomically(s.ownerCtx, route, batch)
	if err != nil {
		return err
	}
	if publication.finished() {
		return errors.New("runs: authoritative model/tool fact crossed a segment boundary")
	}
	route.reducer = speculative
	return nil
}

func (s *segmentPump) attachCompactionPlan(rewrite conversation.Compaction, batch *reductionBatch) error {
	current, err := s.segments.runs.ListRuns(s.ownerCtx, s.spec.SessionID)
	if err != nil {
		return err
	}
	plan, err := PlanConversationCompaction(s.spec.SessionID, rewrite, current)
	if err != nil {
		return err
	}
	if len(batch.events) != 1 || batch.events[0].Commit == nil {
		return errors.New("runs: compaction has no Item commit")
	}
	batch.events[0].Commit.Compaction = new(plan)
	return nil
}

func (s *segmentPump) handleTreeBarrier(event ExecutorEvent, barrier TreeInterrupted) bool {
	root, err := s.routes.resolve(event.Member)
	if err != nil {
		s.fail(err)
		return false
	}
	if root != s.routes.root {
		s.fail(errors.New("runs: tree interrupt must be emitted by the root executor member"))
		return false
	}
	published, err := s.publisher.publishTreeBarrier(
		s.ownerCtx,
		s.routes,
		barrier,
		s.segments.publications.nowUTC(),
	)
	if err != nil {
		s.fail(err)
		return false
	}
	return !published.parked()
}

func (s *segmentPump) handleExecutionFact(member ExecutorMember, executionFact ExecutionFact) (bool, error) {
	route, err := s.routes.resolve(member)
	if err != nil {
		return false, err
	}
	if route.member.MemberID != "" {
		if bindExecutorMemberErr := s.owner.bindExecutorMember(route.runID, route.member.MemberID); bindExecutorMemberErr != nil {
			return false, bindExecutorMemberErr
		}
	}
	if route.reducer == nil {
		return false, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	executionFact = s.classifyChildCancellationFact(route, executionFact, nil)
	if route == s.routes.root {
		if s.deferredRootTerminal != nil {
			return false, fmt.Errorf(
				"runs: root run %q reported %T after its segment already ended",
				route.runID,
				executionFact,
			)
		}
		if engineEventEndsSegment(executionFact) && s.routes.unfinishedCount() > 1 {
			s.deferredRootTerminal = executionFact
			return true, nil
		}
	}
	return s.projectFact(route, executionFact)
}

func (s *segmentPump) projectFact(route *executorRoute, executionFact ExecutionFact) (bool, error) {
	executionFact, cancellation := s.owner.projectCancellation(executionFact, route.lineage)
	projecting := route.reducer
	terminalFact := engineEventEndsSegment(executionFact)
	if terminalFact {
		projecting = route.reducer.clone()
		if projecting == nil {
			return false, fmt.Errorf("runs: run %q has no cloneable terminal reducer", route.runID)
		}
	}
	reductions, err := projecting.reduce(executionFact)
	if err != nil {
		return false, err
	}
	var publication reductionPublication
	if terminalFact {
		publication, err = s.publisher.publishTerminalAtomically(s.ownerCtx, route, reductions, endingDurability(executionFact), cancellation)
	} else {
		publication, err = s.publisher.publish(s.ownerCtx, route, reductions)
	}
	if err != nil {
		return false, err
	}
	if terminalFact {
		route.reducer = projecting
	}
	if !publication.published {
		return false, nil
	}
	route.boundary = publication.boundary
	if route != s.routes.root {
		return s.resumeDeferredRootTerminal()
	}
	return !s.routes.root.boundary.finished(), nil
}

// A non-authoritative executor may report the root boundary before its children;
// synthesizing it would discard the root's outcome, so the exact fact is held
// until the children finish.
func (s *segmentPump) resumeDeferredRootTerminal() (bool, error) {
	if s.deferredRootTerminal == nil || s.routes.unfinishedCount() != 1 {
		return true, nil
	}
	held := s.deferredRootTerminal
	s.deferredRootTerminal = nil
	return s.projectFact(s.routes.root, held)
}

func (s *segmentPump) classifyChildCancellationFact(
	route *executorRoute,
	fact ExecutionFact,
	prior []EventCommit,
) ExecutionFact {
	if route == nil || route.reducer == nil {
		return fact
	}
	if batch, ok := fact.(ToolResultsCommitted); ok {
		batch = batch.clone()
		for index, result := range batch.Results {
			if itemID, open := route.reducer.openToolItemID(result.CallID); open {
				batch.Results[index] = s.owner.classifyChildCancellationTool(route.runID, itemID, result, prior)
			}
		}
		return batch
	}
	return fact
}

// Deferred directly so recover() sees the panic; it keeps one Run's impossible
// state from ending every other Run in the process.
func (s *segmentPump) recoverProjectionDefect() {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.ErrorContext(s.ownerCtx, "runs: run projection panicked",
		telemetry.SessionID(s.spec.SessionID), telemetry.RunID(s.spec.RunID),
		telemetry.Panic(recovered))
	defect := fmt.Errorf("runs: Run %q projection panicked: %v", s.spec.RunID, recovered)
	trace.SpanFromContext(s.ownerCtx).RecordError(defect)
	s.fail(defect)
}

func (s *segmentPump) recoverAbandonedRun() {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.ErrorContext(s.ownerCtx, "runs: run projection abandoned its Run",
		telemetry.SessionID(s.spec.SessionID), telemetry.RunID(s.spec.RunID),
		telemetry.Panic(recovered))
	trace.SpanFromContext(s.ownerCtx).RecordError(
		fmt.Errorf("runs: Run %q projection abandoned its Run: %v", s.spec.RunID, recovered),
	)
}

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

func engineEventEndsSegment(event ExecutionFact) bool {
	switch event.(type) {
	case SegmentEnded:
		return true
	default:
		return false
	}
}

func recordRunCleanupError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	slog.ErrorContext(ctx, "runs: Run cleanup failed", telemetry.Error(err))
	trace.SpanFromContext(ctx).RecordError(err)
}

func (s *segmentPump) activeChildReservations(parentID string) int {
	count := 0
	for _, route := range s.routes.byMember {
		if route.member.ParentID == parentID && !route.boundary.finished() {
			count++
		}
	}
	for _, child := range s.childStarts {
		if child.prepared.member.ParentID == parentID && child.outcome == "" {
			count++
		}
	}
	return count
}
