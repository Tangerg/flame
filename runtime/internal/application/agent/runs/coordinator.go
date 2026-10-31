package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

// errClosed refuses a new segment once Segments is shutting down.
var errClosed = errors.New("runs: coordinator closed")

// Coordinator continues existing Runs: it resumes a waiting Run with its
// answers, revives accepted and uncertain continuations after a restart, steers
// a running Run and reports a command's prior acceptance. [Openings] starts
// Runs, [Cancellations] stops them and [Segments] runs every segment they open.
type Coordinator struct {
	commandAcceptances    commandAcceptanceReader
	continuation          ExecutionContinuer
	approvalRules         approvalRulePreparer
	acceptedContinuations acceptedContinuationStore
	steering              RunningExecutionSteerer
	sessionReader         sessionReader
	interrupts            pendingInterruptReader
	checkpoints           WaitingCheckpointReader
	// runs reads the durable run record. A subscribe or a steer that cannot be
	// served has to say WHY — waiting, finished, a child, or a segment that has
	// been replaced — and only the durable projection knows: the live registry
	// holds running segments, so every one of those looks identical there.
	runs Projection
	// items reads the Items a waiting hand-off names.
	items     ItemProjection
	admission *ownership.Gate
	segments  *Segments
}

// Dependencies are the collaborators continuing an existing Run requires.
type Dependencies struct {
	CommandAcceptances    commandAcceptanceReader
	Continuation          ExecutionContinuer
	ApprovalRules         approvalRulePreparer
	AcceptedContinuations acceptedContinuationStore
	Steering              RunningExecutionSteerer
	Session               SessionPorts
	Checkpoints           WaitingCheckpointReader
	// Segments runs every segment these use cases open.
	Segments   *Segments
	Runs       Projection
	Items      ItemProjection
	Admissions *ownership.Gate
}

// NewCoordinator builds the run use cases over the segment supervisor that runs them.
func NewCoordinator(deps Dependencies) (*Coordinator, error) {
	required := []struct {
		name  string
		value any
	}{
		{"command acceptance reader", deps.CommandAcceptances},
		{"waiting execution continuer", deps.Continuation},
		{"approval rule preparer", deps.ApprovalRules},
		{"accepted continuation store", deps.AcceptedContinuations},
		{"running execution steerer", deps.Steering},
		{"session reader", deps.Session.Reader},
		{"pending interrupt reader", deps.Session.Interrupts},
		{"waiting checkpoint reader", deps.Checkpoints},
		{"segment supervisor", deps.Segments},
		{"run projection", deps.Runs},
		{"item projection", deps.Items},
		{"admission gate", deps.Admissions},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("runs: %s is required", required.name)
		}
	}
	return &Coordinator{
		commandAcceptances:    deps.CommandAcceptances,
		continuation:          deps.Continuation,
		approvalRules:         deps.ApprovalRules,
		acceptedContinuations: deps.AcceptedContinuations,
		steering:              deps.Steering,
		sessionReader:         deps.Session.Reader,
		interrupts:            deps.Session.Interrupts,
		checkpoints:           deps.Checkpoints,
		runs:                  deps.Runs,
		items:                 deps.Items,
		admission:             deps.Admissions,
		segments:              deps.Segments,
	}, nil
}

// ReplayRetention is the window this Coordinator enforces. Discovery publishes
// it from here rather than from a constant of its own: a number the client is
// told and a number the runtime evicts by must be the same number.
func (c *Segments) ReplayRetention() Retention { return c.retention }

// openSegment attaches an already-staged executor stream, atomically commits
// admission/resume plus opening projections, registers the live owner, then
// activates and pumps according to the command's explicit settlement boundary.
// A fresh caller transfers executor ownership on entry, so startup rejection
// releases it. A continuation caller retains failure ownership until the durable
// opening succeeds because its claim/change transaction defines cleanup order.
// The Run lifetime is detached
// from the request without losing its trace; request cancellation drops only
// that subscriber.
func (c *Segments) openSegment(reqCtx context.Context, spec segmentSpec) (iter.Seq[Event], error) {
	startup, err := c.prepareSegmentStartup(reqCtx, spec)
	if err != nil {
		return nil, err
	}
	return startup.activate(reqCtx)
}

// segmentStartup owns the reversible process-local resources between executor
// staging and durable Run opening. It also owns fresh-executor rejection; a
// continuation remains borrowed until its opening commits. Once activate
// registers the tree owner and launches the lifecycle task, that task owns
// activation, pumping, executor cleanup and task release instead.
type segmentStartup struct {
	segments         *Segments
	spec             segmentSpec
	taskContext      context.Context
	runContext       context.Context
	cancelRun        context.CancelFunc
	releaseTask      func()
	executorEvents   iter.Seq[ExecutorEvent]
	observationReady chan iter.Seq[ExecutorEvent]
	journal          *journal
	treeOwner        *runTreeOwner
	routes           *executorRoutes
}

func (c *Segments) prepareSegmentStartup(
	requestContext context.Context,
	spec segmentSpec,
) (*segmentStartup, error) {
	taskContext, releaseTask, attached := c.tasks.Attach(requestContext)
	if !attached {
		if spec.Continuation == nil {
			return nil, c.rejectUnadmittedExecution(requestContext, spec.executorRef(), errClosed)
		}
		return nil, errClosed
	}
	runContext, cancelRun := context.WithCancel(taskContext)
	startup := &segmentStartup{
		segments:    c,
		spec:        spec,
		taskContext: taskContext,
		runContext:  runContext,
		cancelRun:   cancelRun,
		releaseTask: releaseTask,
	}
	var err error
	if spec.StageExecution == nil {
		startup.executorEvents, err = c.observations.Observe(runContext, spec.executorRef())
		if err != nil {
			return nil, startup.abort(err)
		}
	} else {
		startup.observationReady = make(chan iter.Seq[ExecutorEvent], 1)
		startup.executorEvents = func(yield func(ExecutorEvent) bool) {
			select {
			case events := <-startup.observationReady:
				events(yield)
			case <-runContext.Done():
			}
		}
	}
	startup.journal, err = c.newJournal(spec.RunID, spec.SegmentID)
	if err != nil {
		return nil, startup.abort(err)
	}
	startup.treeOwner = newRunTreeOwner(cancelRun, taskContext, startup.journal)
	startup.routes, err = c.openingRoutes(spec, startup.treeOwner.CancelReasonFor)
	if err != nil {
		return nil, startup.abort(err)
	}
	if err := startup.bindExecutorMembers(); err != nil {
		return nil, startup.abort(err)
	}
	return startup, nil
}

func (s *segmentStartup) bindExecutorMembers() error {
	for _, route := range s.routes.admissionOrder {
		if route.member.MemberID == "" {
			continue
		}
		if err := s.treeOwner.bindExecutorMember(route.runID, route.member.MemberID); err != nil {
			return err
		}
	}
	return nil
}

func (s *segmentStartup) abort(cause error) error {
	s.cancelRun()
	if s.spec.Continuation == nil {
		cause = s.segments.rejectUnadmittedExecution(
			s.taskContext,
			s.spec.executorRef(),
			cause,
		)
	}
	s.releaseTask()
	return cause
}

func (s *segmentStartup) activate(requestContext context.Context) (iter.Seq[Event], error) {
	spec := s.spec
	s.treeOwner.observation.Lock()
	var openings []routeOpening
	err := s.segments.registry.Open(Record{
		ID:         spec.RunID,
		SegmentID:  spec.SegmentID,
		ExecutorID: spec.ExecutorID,
	}, s.treeOwner, func() error {
		var err error
		openings, err = s.segments.commitOpening(requestContext, spec, s.routes)
		if err != nil {
			return err
		}
		if spec.admission != nil && !spec.admission.Admit(spec.RunID) {
			panic("runs: committed opening without a pending admission")
		}
		return nil
	})
	if err != nil {
		s.treeOwner.observation.Unlock()
		return nil, s.abort(err)
	}
	s.markSegmentsStarted()
	// Attach before publishing so this opening stream has no gap; a fresh journal
	// has no earlier events to replay.
	subscription := s.journal.tail()
	publicationErr := s.publishOpenings(openings)
	s.treeOwner.observation.Unlock()
	if publicationErr != nil {
		subscription.Cancel()
		publicationErr = errors.Join(publicationErr, s.treeOwner.rejectActivation(publicationErr))
	}
	// Durable activation may synchronously await a fact receipt from the pump.
	// Consume events during activation; the owner gate still orders cancellation.
	if publicationErr == nil {
		go s.beginExecution()
	}
	go func() {
		defer s.releaseTask()
		s.segments.pump(
			s.runContext,
			s.taskContext,
			s.spec,
			s.executorEvents,
			s.treeOwner,
			s.routes,
			publicationErr,
		)
	}()
	if publicationErr != nil {
		return nil, publicationErr
	}
	if !s.spec.DetachActivation {
		<-s.treeOwner.activation.done
	}
	if s.spec.restoresSegments() {
		subscription.Cancel()
		return nil, nil
	}
	return openingStream(requestContext, subscription), nil
}

func openingStream(requestContext context.Context, subscription subscription) iter.Seq[Event] {
	stopUnsubscribe := context.AfterFunc(requestContext, subscription.Cancel)
	return func(yield func(Event) bool) {
		defer stopUnsubscribe()
		subscription.Events(yield)
	}
}

func (s *segmentStartup) publishOpenings(openings []routeOpening) error {
	for _, opening := range openings {
		for _, reduced := range opening.batch.events {
			if err := s.journal.append(s.segments.publications.event(
				opening.route.runID,
				opening.route.segmentID,
				reduced,
			)); err != nil {
				return fmt.Errorf("runs: publish opening replay position: %w", err)
			}
		}
	}
	return nil
}

func (s *segmentStartup) markSegmentsStarted() {
	segmentStartedAt := s.segments.publications.nowUTC()
	for _, route := range s.routes.admissionOrder {
		route.segmentStartedAt = segmentStartedAt
	}
}

func (s *segmentStartup) beginExecution() {
	canceled, _ := s.treeOwner.beginExecution(s.taskContext, func(ctx context.Context) error {
		err := s.activateExecutor(ctx)
		if err != nil {
			cause := fmt.Errorf("runs: activate execution: %w", err)
			trace.SpanFromContext(s.taskContext).RecordError(cause)
			s.treeOwner.observation.Lock()
			s.routes.abortUnfinished(cause)
			s.treeOwner.observation.Unlock()
			s.cancelRun()
		}
		return err
	})
	if canceled {
		s.cancelRun()
	}
}

func (s *segmentStartup) activateExecutor(ctx context.Context) error {
	if s.spec.StageExecution != nil {
		if err := s.spec.StageExecution(ctx); err != nil {
			return err
		}
		events, err := s.segments.observations.Observe(s.runContext, s.spec.executorRef())
		if err != nil {
			return err
		}
		// The pump must consume while BeginExecution awaits durable fact receipts.
		s.observationReady <- events
	}
	if s.spec.BeginExecution == nil {
		return nil
	}
	return s.spec.BeginExecution(ctx)
}

type routeOpening struct {
	route *executorRoute
	batch reductionBatch
}

func (c *Segments) commitOpening(ctx context.Context, spec segmentSpec, routes *executorRoutes) ([]routeOpening, error) {
	if spec.restoresSegments() {
		// Acceptance already committed these exact Segments and Items. Revival
		// installs their process-local owners without replaying an opening write.
		return nil, nil
	}
	ordered, err := routes.unfinishedInPostorder()
	if err != nil {
		return nil, fmt.Errorf("runs: order opening tree: %w", err)
	}
	commitID := newRunCommitID()
	var resume *rundomain.TreeResumeDraft
	if routes.admission == nil {
		resume = &rundomain.TreeResumeDraft{Runs: make([]rundomain.Replacement, 0, len(ordered))}
	}
	openings := make([]routeOpening, 0, len(ordered))
	events := make([]EventCommit, 0, len(ordered))
	for _, route := range ordered {
		projected, err := route.reducer.open()
		if err != nil {
			return nil, fmt.Errorf("runs: reduce opening for Run %q: %w", route.runID, err)
		}
		if len(projected.events) == 0 {
			return nil, fmt.Errorf("runs: reducer for Run %q produced no opening events", route.runID)
		}
		if projected.parkCommit != nil {
			return nil, fmt.Errorf("runs: opening for Run %q cannot park", route.runID)
		}
		if err := validateRouteReductionBatch(route, spec.SessionID, projected); err != nil {
			return nil, err
		}
		for _, reduced := range projected.events {
			if reduced.Event.Terminal() || reduced.Nudge != nil {
				return nil, fmt.Errorf("runs: invalid opening event for Run %q", route.runID)
			}
			if reduced.Commit != nil {
				events = append(events, *reduced.Commit)
			}
		}
		if resume != nil {
			if route.resumed == nil {
				return nil, fmt.Errorf("runs: resumed tree route for Run %q has no resume", route.runID)
			}
			resume.Runs = append(resume.Runs, *route.resumed)
		}
		openings = append(openings, routeOpening{route: route, batch: projected})
	}
	// An opening may carry no item commits at all — a resumed segment that only
	// delivers an approval has nothing to append. Its durable projection is the
	// admission or resume above, which is what makes the Run exist.
	commitOpening := c.publications.commitOpening
	if spec.CommitOpening != nil {
		commitOpening = spec.CommitOpening
	}
	var opening OpeningCommit
	if routes.admission != nil {
		opening, err = NewAdmissionOpeningCommit(
			commitID, *routes.admission, spec.InitialSession, spec.SessionReplacement,
			spec.Actor, events,
		)
	} else {
		opening, err = NewResumeOpeningCommit(commitID, *resume, events)
	}
	if err != nil {
		return nil, err
	}
	if spec.ResumeAcceptance != nil {
		opening, err = opening.WithResumeAcceptance(*spec.ResumeAcceptance)
		if err != nil {
			return nil, err
		}
	}
	if spec.CommandAcceptance != nil {
		acceptance, acceptErr := AcceptCommand(
			spec.CommandAcceptance.Key, spec.CommandAcceptance.Fingerprint,
			opening,
		)
		if acceptErr != nil {
			return nil, acceptErr
		}
		opening, err = opening.WithCommandAcceptance(acceptance)
		if err != nil {
			return nil, err
		}
	}
	if err := commitOpening(ctx, opening); err != nil {
		return nil, err
	}
	return openings, nil
}

// rejectUnadmittedExecution tears down a fresh execution that failed before its opening
// write-set committed. The rejection cause and teardown failure are both
// preserved: hiding the latter would report a clean rejection while leaking an
// executor the application never admitted.
func (c *Segments) rejectUnadmittedExecution(ctx context.Context, ref ExecutorRef, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.releases.Release(cleanupCtx, ref); err != nil {
		cleanupErr := fmt.Errorf("runs: release unadmitted executor %q: %w", ref.ExecutorID, err)
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

// Subscribe attaches to the Segment the request names. The record and journal
// are captured from the same registry entry, so a caller cannot receive a
// Segment id from one entry and subscribe to a replacement or removed entry. The
// subscription is dropped when ctx ends or the consumer stops ranging.
//
// Refusals name what the caller should do instead: [ErrRunNotFound],
// [transcript.ErrNotRoot], [ErrRunWaiting], [ErrRunFinished],
// [ErrStaleSegment], [ErrReplayCursorInvalid], [ErrReplayUnavailable], and
// [rundomain.InsufficientCapabilitiesError] when the caller could not follow what this run
// publishes — the same rule a resume applies, kept here rather than at each
// caller so the two entry points into an existing Run cannot disagree about it.
func (c *Segments) Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error) {
	req = req.clone()
	live, err := c.addressSubscription(ctx, req)
	if err != nil {
		return Subscription{}, err
	}
	return subscribeLiveSegment(ctx, live, req)
}

// SubscribeSnapshot reads durable material and establishes its successor tail
// while the addressed tree cannot commit or publish. The reader stays with its
// own use case; this boundary owns continuity with the executing Run.
func (c *Segments) SubscribeSnapshot(ctx context.Context, req SubscribeRequest, read func(context.Context, string) error) (Subscription, error) {
	req = req.clone()
	if req.Cursor != "" {
		return Subscription{}, errors.New("runs: snapshot subscription cannot replay a cursor")
	}
	live, err := c.addressSubscription(ctx, req)
	if err != nil {
		return Subscription{}, err
	}
	live.owner.observation.Lock()
	defer live.owner.observation.Unlock()
	if err := ctx.Err(); err != nil {
		return Subscription{}, err
	}
	// Waiting for publication may cross a park/finish/resume boundary. The
	// snapshot must not describe a successor Segment while tailing the old one.
	_, durable, err := c.addressLiveSegment(ctx, req.RunID, req.SegmentID)
	if err != nil {
		return Subscription{}, err
	}
	if err := read(ctx, durable.SessionID()); err != nil {
		return Subscription{}, err
	}
	return subscribeLiveSegment(ctx, live, req)
}

func subscribeLiveSegment(ctx context.Context, live liveSegment, req SubscribeRequest) (Subscription, error) {
	attached, err := live.owner.hub.attach(req.Cursor)
	if err != nil {
		return Subscription{}, err
	}
	stopUnsubscribe := context.AfterFunc(ctx, attached.Cancel)
	return Subscription{
		Record: live.record, HeadCursor: attached.HeadCursor,
		Events: func(yield func(Event) bool) {
			defer stopUnsubscribe()
			attached.Events(yield)
		},
	}, nil
}

func (c *Segments) addressSubscription(ctx context.Context, req SubscribeRequest) (liveSegment, error) {
	live, durable, err := c.addressLiveSegment(ctx, req.RunID, req.SegmentID)
	if err != nil {
		return liveSegment{}, err
	}
	if gap := durable.Capabilities().MissingFrom(req.CallerCapabilities); !gap.IsEmpty() {
		return liveSegment{}, fmt.Errorf("runs: run %q: %w", req.RunID, &rundomain.InsufficientCapabilitiesError{Missing: gap})
	}
	return live, nil
}

// addressLiveSegment resolves the run and segment a control command addresses,
// refusing with the reason the caller can act on. It returns the durable Run,
// which owns every fact about the Run beyond its live Segment.
//
// The durable record is the authority for the refusal, not the live registry:
// the registry holds only running segments, so a run that is waiting, finished,
// or a child all look the same there — one indistinguishable "not found" for
// three situations whose remedies are answering an interrupt, reading the
// transcript, and following rootRunId.
//
// Both entry points into an executing run (subscribe and steer) resolve here, so
// they cannot come to different conclusions about the same run.
func (c *Segments) addressLiveSegment(ctx context.Context, runID, segmentID string) (liveSegment, rundomain.Run, error) {
	// Capture the owner before reading its durable state. Retirement may remove
	// that entry after the read, but cannot invalidate the captured journal.
	// Exclude openings until both reads finish so a newly committed Segment
	// cannot be paired with an absent or predecessor owner.
	c.registry.opening.RLock()
	defer c.registry.opening.RUnlock()
	live, liveFound := c.registry.Get(runID)
	run, ok, err := c.runs.Run(ctx, runID)
	if err != nil {
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("runs: read run %q: %w", runID, err)
	}
	if !ok {
		return liveSegment{}, rundomain.Run{}, ErrRunNotFound
	}
	if run.Lineage().IsChild() {
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("%w: %q", transcript.ErrNotRoot, runID)
	}
	switch status := run.State(); status {
	case rundomain.Waiting:
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("%w: %q", ErrRunWaiting, runID)
	case rundomain.Finished:
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("%w: %q", ErrRunFinished, runID)
	case rundomain.Running:
	default:
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("runs: run %q has unknown status %q", runID, status)
	}
	if run.ActiveSegmentID() != segmentID {
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("%w: run %q is executing %q", ErrStaleSegment, runID, run.ActiveSegmentID())
	}
	if !liveFound {
		// A Running record whose segment this process does not own. Restart recovery
		// terminalizes orphans before the runtime serves, so this is a broken
		// invariant rather than a state a client can act on — reporting it as one
		// would teach the client a lie about the run.
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("runs: run %q is running segment %q with no live stream", runID, segmentID)
	}
	if live.record.SegmentID != segmentID {
		// Opening and owner publication share the fence above; a disagreement is
		// an ownership fault, not a reason to retarget the caller's Segment.
		return liveSegment{}, rundomain.Run{}, fmt.Errorf(
			"runs: run %q is executing segment %q but its live owner names %q",
			runID,
			segmentID,
			live.record.SegmentID,
		)
	}
	return live, run, nil
}

// BeginShutdown prevents new runs and cancels every in-flight pump.
func (c *Segments) BeginShutdown() { c.tasks.Cancel() }

// AwaitShutdown joins every in-flight pump after [BeginShutdown].
func (c *Segments) AwaitShutdown(ctx context.Context) error {
	return c.tasks.Wait(ctx)
}

func (c *Segments) newJournal(runID, segmentID string) (*journal, error) {
	scope, err := newStreamScope(c.epoch, runID, segmentID)
	if err != nil {
		return nil, err
	}
	return newJournal(scope, c.retention)
}
