package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// pump starts the single goroutine that owns every mutable route and reducer in
// this Segment. Event handling and cleanup are methods on one concrete object;
// no state is shared with another worker.
func (c *Coordinator) pump(
	ctx context.Context,
	ownerCtx context.Context,
	spec segmentSpec,
	executorEvents iter.Seq[ExecutorEvent],
	owner *runTreeOwner,
	routes *executorRoutes,
	initialErr error,
) {
	pump := &segmentPump{
		coordinator: c,
		ctx:         ctx,
		ownerCtx:    ownerCtx,
		spec:        spec,
		events:      executorEvents,
		owner:       owner,
		routes:      routes,
	}
	pump.publisher = treePublisher{publications: &c.publications, rootSpec: spec, owner: owner}
	// The inner recovery turns a projection defect into this Run's failure; this
	// one covers a defect in that failure path itself. Either way the process
	// keeps every other Session, and the Run this pump abandoned is left for boot
	// recovery, which already owns a Run whose owner disappeared.
	defer pump.recoverAbandonedRun()
	pump.run(initialErr)
}

// segmentPump serializes executor events into durable Run-tree projections. A
// successful terminal or park is committed before publication; a projection
// failure aborts execution rather than exposing an event without durable facts.
type segmentPump struct {
	coordinator *Coordinator
	ctx         context.Context
	ownerCtx    context.Context
	spec        segmentSpec
	events      iter.Seq[ExecutorEvent]
	owner       *runTreeOwner
	routes      *executorRoutes
	publisher   treePublisher

	rootFinished bool
	rootParked   bool
	childStarts  map[string]*managedChildStart
	// deferredRootTerminal is the root's own segment boundary, withheld until
	// every child run has terminalized. See resumeDeferredRootTerminal.
	deferredRootTerminal ExecutionFact
}

type managedChildStart struct {
	prepared  *preparedChildStart
	outcome   ChildRunStartOutcome
	startedAt time.Time
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

func (s *segmentPump) processEvent(event ExecutorEvent) bool {
	s.owner.observation.Lock()
	defer s.owner.observation.Unlock()
	if request, reserving := event.Payload.(ChildRunReservationRequest); reserving {
		return s.handleChildRunReservation(event, request)
	}
	if request, concluding := event.Payload.(ChildRunStartOutcomeRequest); concluding {
		return s.handleChildRunStartOutcome(event, request)
	}
	if err := event.Validate(); err != nil {
		s.fail(err)
		return false
	}
	if lookup, checking := event.Payload.(ResultPublicationLookup); checking {
		committed, err := s.lookupResultPublication(event.Member, lookup)
		lookup.Complete(committed, err)
		return true
	}
	if commit, authoritative := event.Payload.(ExecutionFactCommit); authoritative {
		if err := commit.validate(); err != nil {
			commit.Complete(err)
			s.fail(err)
			return false
		}
		fact := commit.Fact()
		err := s.handleAuthoritativeFact(event.Member, fact)
		commit.Complete(err)
		// A rejected authoritative write is reported synchronously to the
		// executor. It then produces either a definite failed result or
		// an unknown settlement; stopping this pump here would race that decision
		// and tear down the only source able to report it.
		return true
	}
	if unknown, detected := event.Payload.(UnknownEffectsDetected); detected {
		if err := s.handleUnknownEffects(event.Member, unknown); err != nil {
			s.fail(err)
		}
		return false
	}
	if barrier, interrupted := event.Payload.(TreeInterrupted); interrupted {
		s.handleTreeBarrier(event, barrier)
		return false
	}
	executionFact, ok := event.Payload.(ExecutionFact)
	if !ok {
		s.fail(fmt.Errorf("runs: unsupported executor payload %T", event.Payload))
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
		if request.claim() {
			_ = request.complete(ChildRunBinding{}, err)
		}
		s.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		s.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	if existing := s.childStarts[event.Member.MemberID]; existing != nil {
		if existing.prepared.member != event.Member {
			err := fmt.Errorf("runs: child member %q repeated a different start reservation", event.Member.MemberID)
			_ = request.complete(ChildRunBinding{}, err)
			return true
		}
		_ = request.complete(existing.prepared.reservation.Binding, nil)
		return true
	}
	prepared, err := s.coordinator.prepareChildStart(
		s.spec, s.owner, s.routes, event.Member,
	)
	if err == nil {
		err = s.coordinator.childStarts.ReserveChildRunStart(s.ownerCtx, prepared.reservation)
	}
	if err != nil {
		if prepared != nil {
			prepared.releaseBinding(s.owner)
		}
		_ = request.complete(ChildRunBinding{}, err)
		return true
	}
	if s.childStarts == nil {
		s.childStarts = make(map[string]*managedChildStart)
	}
	s.childStarts[event.Member.MemberID] = &managedChildStart{prepared: prepared}
	if err := request.complete(prepared.reservation.Binding, nil); err != nil {
		s.abortPreparedChildStart(prepared)
		delete(s.childStarts, event.Member.MemberID)
		s.fail(err)
		return false
	}
	return true
}

func (s *segmentPump) handleChildRunStartOutcome(
	event ExecutorEvent,
	request ChildRunStartOutcomeRequest,
) bool {
	if err := event.Validate(); err != nil {
		if request.claim() {
			_ = request.complete(err)
		}
		s.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		s.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	managed := s.childStarts[event.Member.MemberID]
	if managed == nil || managed.prepared.member != event.Member ||
		managed.prepared.reservation.Binding != request.Binding {
		err := fmt.Errorf("runs: child member %q has no matching start reservation", event.Member.MemberID)
		_ = request.complete(err)
		return true
	}
	if managed.outcome.Valid() {
		if managed.outcome != request.Outcome || !managed.startedAt.Equal(request.StartedAt) {
			err := fmt.Errorf("runs: child member %q repeated a contradictory start outcome", event.Member.MemberID)
			_ = request.complete(err)
			return true
		}
		_ = request.complete(nil)
		return true
	}
	prepared := managed.prepared
	var err error
	keep := true
	switch request.Outcome {
	case ChildRunStarted:
		err = s.coordinator.finalizeChildOpening(s.spec, s.owner, prepared, request.StartedAt)
		if err == nil {
			err = s.coordinator.childStarts.CommitStartedChildRun(
				s.ownerCtx, prepared.reservation, prepared.opening,
			)
		}
		if err == nil {
			managed.outcome = request.Outcome
			managed.startedAt = request.StartedAt
			s.coordinator.activatePreparedChild(s.spec, s.routes, prepared)
			publication, publishErr := s.publisher.publish(s.ownerCtx, prepared.route, prepared.batch)
			if publishErr != nil || publication.finished() {
				if publishErr == nil {
					publishErr = fmt.Errorf("runs: child member %q start unexpectedly reached a boundary", event.Member.MemberID)
				}
				// The durable child Run now exists. Rejecting the executor's started
				// outcome would create a public Run without its executor member, so acknowledge
				// the conclusive start and fail this projection pump instead.
				s.fail(publishErr)
				keep = false
			}
		}
		if err != nil {
			// The executor will not publish a child when this receipt fails. Consume
			// the invisible reservation as aborted; a failed cleanup remains hidden
			// and is reconciled at startup rather than becoming a ghost Run.
			// Detached because the owner may already be canceled, and bounded
			// because this pump is the only thing driving the Run to a terminal
			// state: an unbounded store call here strands the whole Session.
			cleanupCtx, cancelCleanup := context.WithTimeout(
				context.WithoutCancel(s.ownerCtx), runCleanupTimeout,
			)
			cleanupErr := s.coordinator.childStarts.AbortChildRunStart(
				cleanupCtx, prepared.reservation,
			)
			cancelCleanup()
			err = errors.Join(err, cleanupErr)
			prepared.releaseBinding(s.owner)
			prepared.route.reducer = nil
			prepared.batch = reductionBatch{}
			prepared.opening = OpeningCommit{}
		}
	case ChildRunStartAborted:
		err = s.coordinator.childStarts.AbortChildRunStart(s.ownerCtx, prepared.reservation)
		if err == nil {
			managed.outcome = request.Outcome
			managed.startedAt = time.Time{}
			prepared.releaseBinding(s.owner)
		}
	default:
		err = errors.New("runs: invalid child Run start outcome")
	}
	if completionErr := request.complete(err); completionErr != nil {
		s.fail(errors.Join(err, completionErr))
		return false
	}
	return keep
}

func (s *segmentPump) abortPreparedChildStart(prepared *preparedChildStart) {
	if prepared == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	recordRunCleanupError(ctx, s.coordinator.childStarts.AbortChildRunStart(ctx, prepared.reservation))
	prepared.releaseBinding(s.owner)
}

func (s *segmentPump) lookupResultPublication(member ExecutorMember, lookup ResultPublicationLookup) (bool, error) {
	if err := lookup.validate(); err != nil {
		return false, err
	}
	route, err := s.routes.resolve(member)
	if err != nil {
		return false, err
	}
	return s.coordinator.publications.events.ResultPublicationCommitted(
		s.ownerCtx, s.spec.SessionID, route.runID, route.segmentID, lookup.Publication(),
	)
}

func (s *segmentPump) handleAuthoritativeFact(member ExecutorMember, fact ExecutionFact) error {
	route, err := s.routes.resolve(member)
	if err != nil {
		return err
	}
	if route.member.MemberID != "" {
		if err := s.owner.bindExecutorMember(route.runID, route.member.MemberID); err != nil {
			return err
		}
	}
	if route.reducer == nil {
		return fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	fact = s.classifyChildCancellationFact(route, fact)
	speculative := route.reducer.clone()
	batch, err := speculative.reduce(fact)
	if err != nil {
		return err
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

func (s *segmentPump) handleUnknownEffects(
	member ExecutorMember,
	unknown UnknownEffectsDetected,
) error {
	if err := unknown.validate(); err != nil {
		return err
	}
	route, err := s.routes.resolve(member)
	if err != nil {
		return err
	}
	if route != s.routes.root {
		return errors.New("runs: root-only execution reported unknown Effects for a child member")
	}
	details := []string{"external operations have no provable durable result"}
	for _, effect := range unknown.Effects() {
		detail := effect.ID
		if effect.Detail != "" {
			detail += ": " + effect.Detail
		}
		details = append(details, detail)
	}
	failure := run.Failure{Kind: run.FailureLost, Detail: strings.Join(details, "\n")}
	ordered, err := s.routes.unfinishedInPostorder()
	if err != nil {
		return fmt.Errorf("runs: order unknown Effect loss: %w", err)
	}
	for _, unfinished := range ordered {
		if err := s.commitUnknownEffectLoss(unfinished, failure); err != nil {
			return err
		}
	}
	return nil
}

func (s *segmentPump) commitUnknownEffectLoss(route *executorRoute, failure run.Failure) error {
	batch, err := route.reducer.reduce(NewSegmentEnded(
		run.OutcomeLost,
		&failure,
		nil,
		route.activeDuration(s.coordinator.publications.nowUTC()),
	))
	if err != nil {
		return fmt.Errorf("runs: reduce unknown Effect loss: %w", err)
	}
	retry := unknownEffectCommitRetry{}
	reported := false
	for {
		publication, publishErr := s.publisher.publishTerminalAtomically(s.ownerCtx, route, batch)
		if publishErr == nil {
			route.segmentFinished = publication.finished()
			if route == s.routes.root {
				s.rootFinished = publication.finished()
				s.rootParked = false
			}
			return nil
		}
		if !reported {
			// This retry is unbounded by design, and the Run stays blocked until it
			// commits. A span reaches nobody on a host that configured no tracing,
			// so a store outage here would stall a Run for as long as it lasts with
			// no diagnostic anywhere. Reported once: the retry cadence backs off,
			// and repeating one outage every attempt buries it.
			slog.ErrorContext(s.ownerCtx, "runs: unknown Effect loss commit failed, retrying until it commits",
				"session.id", s.spec.SessionID, "run.id", route.runID, "error", publishErr)
			reported = true
		}
		trace.SpanFromContext(s.ctx).RecordError(fmt.Errorf("runs: retry unknown Effect loss: %w", publishErr))
		if waitErr := retry.wait(s.ownerCtx); waitErr != nil {
			return errors.Join(publishErr, waitErr)
		}
	}
}

func (s *segmentPump) handleTreeBarrier(event ExecutorEvent, barrier TreeInterrupted) {
	root, err := s.routes.resolve(event.Member)
	if err != nil {
		s.fail(err)
		return
	}
	if root != s.routes.root {
		s.fail(errors.New("runs: tree interrupt must be emitted by the root executor member"))
		return
	}
	publication, err := s.publisher.publishTreeBarrier(
		s.ownerCtx,
		s.routes,
		barrier,
		s.coordinator.publications.nowUTC(),
	)
	if err != nil {
		s.fail(err)
		return
	}
	if publication.published {
		s.rootFinished = publication.finished()
		s.rootParked = publication.parked()
	}
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
	if _, interrupted := executionFact.(SegmentInterrupted); interrupted {
		return false, errors.New("runs: executor emitted a per-Run interrupt instead of a tree barrier")
	}
	executionFact = s.classifyChildCancellationFact(route, executionFact)
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

// projectFact commits one route's reduction and reports whether this pump may
// keep consuming.
func (s *segmentPump) projectFact(route *executorRoute, executionFact ExecutionFact) (bool, error) {
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
		publication, err = s.publisher.publishTerminalAtomically(s.ownerCtx, route, reductions)
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
	route.segmentFinished = publication.finished()
	if route != s.routes.root {
		return s.resumeDeferredRootTerminal()
	}
	s.rootFinished = s.rootFinished || publication.finished()
	s.rootParked = s.rootParked || publication.parked()
	// A committed root boundary is the last event this Segment can durably
	// support. Leave a park alive for resume and never consume buffered events
	// after a terminal transition.
	return !s.rootParked && !s.rootFinished, nil
}

// resumeDeferredRootTerminal commits the root boundary held back while children
// were still live, once the last of them has terminalized.
//
// The executor ends a canceled subtree by cancelling the children's work and
// publishing the parent's terminal first, so a root cancellation routinely
// arrives ahead of the children it ended. Committing it on arrival would close
// the tree over live descendant rows, and the durable Run projection has no way
// to represent that: the partial unique index only keeps one non-terminal tree
// per Session, so publication order is the only thing enforcing it.
//
// Holding the root's own fact rather than stopping here is what lets the
// children report their real terminals — a child cancelled inside a provider
// call settles as canceled, where a synthesized close would have to call it
// lost. When the executor stream ends before the children report, the terminal
// synthesis in finish() remains the backstop and closes the tree itself.
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
) ExecutionFact {
	if route == nil || route.reducer == nil {
		return fact
	}
	if batch, ok := fact.(ToolResultsCommitted); ok {
		batch = batch.clone()
		for index, result := range batch.Results {
			if itemID, open := route.reducer.openToolItemID(result.CallID); open {
				batch.Results[index] = s.owner.classifyChildCancellationTool(route.runID, itemID, result)
			}
		}
		return batch
	}
	return fact
}

// recoverProjectionDefect keeps one Run's impossible state from ending every
// other Run in the process. This pump already owns "the projection cannot
// continue": aborting its routes lets the terminal synthesis that follows close
// the tree as an internal failure, instead of unwinding out of a goroutine
// nothing can recover from and taking every other Session with it.
//
// It must be deferred directly so recover() sees the panic. The stack cannot
// travel in the Run's failure and is the only thing that fixes the defect, so it
// goes to the operator here.
func (s *segmentPump) recoverProjectionDefect() {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.ErrorContext(s.ownerCtx, "runs: run projection panicked",
		"session.id", s.spec.SessionID, "run.id", s.spec.RunID,
		"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
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
		"session.id", s.spec.SessionID, "run.id", s.spec.RunID,
		"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
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
	s.owner.observation.Lock()
	defer s.owner.observation.Unlock()
	// Closing the journal is what tells subscribers the Segment ended, and
	// releasing the executor is what stops it working. Both are deferred so a
	// terminal projection that cannot complete still ends the Segment for the
	// clients waiting on it, instead of leaving a Run nobody can finish and a
	// stream nobody can leave.
	defer s.finishBoundary()
	defer s.releaseExecutorTree()
	for memberID, managed := range s.childStarts {
		if !managed.outcome.Valid() {
			s.abortPreparedChildStart(managed.prepared)
		}
		delete(s.childStarts, memberID)
	}
	if !s.rootFinished {
		s.synthesizeUnfinished()
	}
}

// releaseExecutorTree ends the executor behind every non-waiting boundary
// exactly once. The product outcome is already committed (or was synthesized);
// Release is resource ownership, not a second cancellation decision.
func (s *segmentPump) releaseExecutorTree() {
	if !s.rootParked {
		s.tearDownExecutor()
	}
}

// synthesizeUnfinished establishes durable terminal boundaries before executor
// teardown. Children close in canonical postorder; the root closes only if
// every child did, so persistence never advertises a closed tree with a live
// descendant row.
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
	if childrenClosed && !s.routes.root.segmentFinished {
		s.rootFinished = s.synthesizeRoute(ctx, s.routes.root)
		s.rootParked = false
	}
}

func (s *segmentPump) synthesizeRoute(ctx context.Context, route *executorRoute) bool {
	reductions, err := route.reducer.synthesizeTerminal()
	if err != nil {
		s.fail(err)
		return false
	}
	publication, err := s.publisher.publishTerminalAtomically(ctx, route, reductions)
	if err != nil {
		s.fail(err)
		return false
	}
	route.segmentFinished = publication.finished()
	if !publication.finished() || publication.parked() {
		s.fail(fmt.Errorf(
			"runs: synthesized terminal for run %q produced finished=%t parked=%t",
			route.runID,
			publication.finished(),
			publication.parked(),
		))
		return false
	}
	return true
}

func (s *segmentPump) tearDownExecutor() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	if err := s.coordinator.releases.Release(ctx, s.spec.executorRef()); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		s.owner.completionErr = fmt.Errorf("runs: tear down executor %q: %w", s.spec.ExecutorID, err)
		recordRunCleanupError(ctx, s.owner.completionErr)
	}
}

// finishBoundary settles what a finished Run still owes the rest of the process.
// Every obligation here is deferred so a defect in the maintenance it fences
// still reaches them on its way out: the outer recovery is willing to abandon one
// Run, and losing this boundary instead leaves a Session that can never admit
// another Run and a stream no subscriber can leave.
func (s *segmentPump) finishBoundary() {
	defer s.coordinator.registry.RemoveSegment(s.spec.RunID, s.spec.SegmentID)
	defer s.closeJournal()
	s.settleMaintenanceFence()
}

// closeJournal ends the externally observable completion boundary. The
// synchronous maintenance fence and admission claim are already gone.
func (s *segmentPump) closeJournal() {
	if err := s.owner.hub.close(); err != nil {
		s.owner.completionErr = fmt.Errorf("runs: close replay journal: %w", err)
		recordRunCleanupError(s.ownerCtx, s.owner.completionErr)
	}
}

// settleMaintenanceFence runs post-Run maintenance while this Run's admission
// still holds its Session and working tree, so a following Run cannot write into
// the snapshot this one is taking, and gives that admission back before
// returning.
func (s *segmentPump) settleMaintenanceFence() {
	releaseMaintenance, maintenanceHeld := s.coordinator.admission.BeginMaintenance(s.spec.RunID)
	if maintenanceHeld {
		defer releaseMaintenance()
	}
	entry, tracked := s.coordinator.registry.Get(s.spec.RunID)
	if tracked && !s.rootParked {
		entry.owner.stop()
	}
	if !s.rootFinished {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	if err := s.coordinator.finalizer.Finish(ctx, Finish{
		SessionID:       s.spec.SessionID,
		RunID:           s.spec.RunID,
		WorkspaceCWD:    s.spec.WorkspaceCWD,
		Parked:          s.rootParked,
		OpeningUserText: s.spec.OpeningUserText,
	}); err != nil {
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

// recordRunCleanupError reports a failure in work a finished Run still owed:
// releasing its executor, closing its journal, aborting a reserved child start,
// running post-Run maintenance. None of it can change the Run's settled outcome,
// and a span carries it only where the host configured tracing, so the
// operator's copy goes to the logging channel.
func recordRunCleanupError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	slog.ErrorContext(ctx, "runs: Run cleanup failed", "error", err)
	trace.SpanFromContext(ctx).RecordError(err)
}
