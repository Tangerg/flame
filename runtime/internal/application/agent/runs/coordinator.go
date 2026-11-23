package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

var errClosed = errors.New("runs: coordinator closed")

type Coordinator struct {
	commandAcceptances    commandAcceptanceReader
	continuation          ExecutionContinuer
	approvalRules         approvalRulePreparer
	acceptedContinuations acceptedContinuationStore
	steering              RunningExecutionSteerer
	sessionReader         sessionReader
	interrupts            pendingInterruptReader
	checkpoints           WaitingCheckpointReader
	runs                  Projection
	items                 ItemProjection
	admission             *ownership.Gate
	segments              *Segments
	newItemID             func() identity.ItemID
}

type Dependencies struct {
	CommandAcceptances    commandAcceptanceReader
	Continuation          ExecutionContinuer
	ApprovalRules         approvalRulePreparer
	AcceptedContinuations acceptedContinuationStore
	Steering              RunningExecutionSteerer
	Sessions              sessionReader
	Interrupts            pendingInterruptReader
	Checkpoints           WaitingCheckpointReader
	Segments              *Segments
	Runs                  Projection
	Items                 ItemProjection
	Admissions            *ownership.Gate
	NewItemID             func() identity.ItemID
}

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
		{"session reader", deps.Sessions},
		{"pending interrupt reader", deps.Interrupts},
		{"waiting checkpoint reader", deps.Checkpoints},
		{"segment supervisor", deps.Segments},
		{"run projection", deps.Runs},
		{"item projection", deps.Items},
		{"admission gate", deps.Admissions},
		{"item id generator", deps.NewItemID},
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
		sessionReader:         deps.Sessions,
		interrupts:            deps.Interrupts,
		checkpoints:           deps.Checkpoints,
		runs:                  deps.Runs,
		items:                 deps.Items,
		admission:             deps.Admissions,
		segments:              deps.Segments,
		newItemID:             deps.NewItemID,
	}, nil
}

func (c *Segments) ReplayRetention() Retention { return c.retention }

func (c *Segments) openSegment(reqCtx context.Context, spec segmentSpec) (iter.Seq[Event], error) {
	startup, err := c.prepareSegmentStartup(reqCtx, spec)
	if err != nil {
		return nil, err
	}
	return startup.activate(reqCtx)
}

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
	if spec.BeginExecution == nil {
		return nil, errors.New("runs: Segment startup requires an execution start")
	}
	if resumption, ok := spec.Opening.(treeResumption); ok && resumption.restoresSegments(spec.RunID) {
		attempts, err := c.toolAttempts.ListOpenToolInvocations(requestContext)
		if err != nil {
			return nil, spec.staged.abandon(requestContext, err, "read restored Segment Tool attempts")
		}
		resumption.toolAttempts = attempts
		spec.Opening = resumption
	}
	taskContext, releaseTask, attached := c.tasks.Attach(requestContext)
	if !attached {
		return nil, spec.staged.abandon(requestContext, errClosed, "unadmitted executor")
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
	if _, deferred := spec.stageExecution(); !deferred {
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
	startup.routes, err = c.openingRoutes(spec)
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
	cause = s.spec.staged.abandon(s.taskContext, cause, "unadmitted executor")
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
	if spec.staged != nil {
		spec.staged.transfer()
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
		if err := s.treeOwner.awaitActivation(context.WithoutCancel(requestContext)); err != nil {
			subscription.Cancel()
			return nil, err
		}
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
	if stage, deferred := s.spec.stageExecution(); deferred {
		if err := stage(ctx); err != nil {
			return err
		}
		events, err := s.segments.observations.Observe(s.runContext, s.spec.executorRef())
		if err != nil {
			return err
		}
		s.observationReady <- events
	}
	return s.spec.BeginExecution(ctx)
}

type routeOpening struct {
	route *executorRoute
	batch reductionBatch
}

func (c *Segments) commitOpening(ctx context.Context, spec segmentSpec, routes *executorRoutes) ([]routeOpening, error) {
	if spec.restoresSegments() {
		return nil, nil
	}
	ordered, err := routes.unfinishedInPostorder()
	if err != nil {
		return nil, fmt.Errorf("runs: order opening tree: %w", err)
	}
	openings, events, err := reduceOpenings(spec.SessionID, ordered)
	if err != nil {
		return nil, err
	}
	opening, commit, err := c.openingCommit(spec, ordered, events)
	if err != nil {
		return nil, err
	}
	if spec.CommandAcceptance != nil {
		acceptance, err := AcceptCommand(spec.CommandAcceptance.Key, spec.CommandAcceptance.Fingerprint, opening)
		if err != nil {
			return nil, err
		}
		opening, err = opening.WithCommandAcceptance(acceptance)
		if err != nil {
			return nil, err
		}
	}
	if err := commit(ctx, opening); err != nil {
		return nil, err
	}
	return openings, nil
}

func reduceOpenings(sessionID identity.SessionID, ordered []*executorRoute) ([]routeOpening, []EventCommit, error) {
	openings := make([]routeOpening, 0, len(ordered))
	events := make([]EventCommit, 0, len(ordered))
	for _, route := range ordered {
		projected, err := route.reducer.open()
		if err != nil {
			return nil, nil, fmt.Errorf("runs: reduce opening for Run %q: %w", route.runID, err)
		}
		if len(projected.events) == 0 {
			return nil, nil, fmt.Errorf("runs: reducer for Run %q produced no opening events", route.runID)
		}
		if projected.parkCommit != nil {
			return nil, nil, fmt.Errorf("runs: opening for Run %q cannot park", route.runID)
		}
		if err := validateRouteReductionBatch(route, sessionID, projected); err != nil {
			return nil, nil, err
		}
		for _, reduced := range projected.events {
			if reduced.Event.Terminal() || reduced.Nudge != nil {
				return nil, nil, fmt.Errorf("runs: invalid opening event for Run %q", route.runID)
			}
			if reduced.Commit != nil {
				events = append(events, *reduced.Commit)
			}
		}
		openings = append(openings, routeOpening{route: route, batch: projected})
	}
	return openings, events, nil
}

func (c *Segments) openingCommit(spec segmentSpec, ordered []*executorRoute, events []EventCommit) (OpeningCommit, func(context.Context, OpeningCommit) error, error) {
	commitID := newRunCommitID()
	switch value := spec.Opening.(type) {
	case rootOpening:
		opening, err := NewAdmissionOpeningCommit(
			commitID, value.draft(spec), value.SessionWrite, value.Actor, events,
		)
		return opening, c.publications.openings.CommitOpening, err
	case treeResumption:
		resume := rundomain.TreeResumeDraft{Runs: make([]rundomain.Replacement, 0, len(ordered))}
		for _, route := range ordered {
			if route.resumed == nil {
				return OpeningCommit{}, nil, fmt.Errorf("runs: resumed tree route for Run %q has no resume", route.runID)
			}
			resume.Runs = append(resume.Runs, *route.resumed)
		}
		opening, err := NewResumeOpeningCommit(commitID, resume, events)
		if err == nil && value.ResumeAcceptance != nil {
			opening, err = opening.WithResumeAcceptance(*value.ResumeAcceptance)
		}
		commit := c.publications.openings.CommitOpening
		if value.CommitOpening != nil {
			commit = value.CommitOpening
		}
		return opening, commit, err
	default:
		return OpeningCommit{}, nil, fmt.Errorf("runs: unknown Segment opening %T", spec.Opening)
	}
}

func (c *Segments) Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error) {
	req = req.clone()
	live, err := c.addressSubscription(ctx, req)
	if err != nil {
		return Subscription{}, err
	}
	return subscribeLiveSegment(ctx, live, req)
}

func (c *Segments) SubscribeSnapshot(ctx context.Context, req SubscribeRequest, read func(context.Context, identity.SessionID) error) (Subscription, error) {
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

// Live addressing and the durable recording of a steer both ask the Run the
// same question, so they share one answer.
func RequireRunningSegment(current rundomain.Run, segmentID identity.SegmentID) error {
	if current.Lineage().IsChild() {
		return fmt.Errorf("%w: %q", transcript.ErrNotRoot, current.ID())
	}
	switch status := current.State(); status {
	case rundomain.Waiting:
		return fmt.Errorf("%w: %q", ErrRunWaiting, current.ID())
	case rundomain.Finished:
		return fmt.Errorf("%w: %q", ErrRunFinished, current.ID())
	case rundomain.Running:
	default:
		return fmt.Errorf("runs: run %q has unknown status %q", current.ID(), status)
	}
	if current.ActiveSegmentID() != segmentID {
		return fmt.Errorf("%w: run %q is executing %q", ErrStaleSegment, current.ID(), current.ActiveSegmentID())
	}
	return nil
}

func (c *Segments) addressLiveSegment(ctx context.Context, runID identity.RunID, segmentID identity.SegmentID) (liveSegment, rundomain.Run, error) {
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
	if err := RequireRunningSegment(run, segmentID); err != nil {
		return liveSegment{}, rundomain.Run{}, err
	}
	if !liveFound {
		return liveSegment{}, rundomain.Run{}, fmt.Errorf("runs: run %q is running segment %q with no live stream", runID, segmentID)
	}
	if live.record.SegmentID != segmentID {
		return liveSegment{}, rundomain.Run{}, fmt.Errorf(
			"runs: run %q is executing segment %q but its live owner names %q",
			runID,
			segmentID,
			live.record.SegmentID,
		)
	}
	return live, run, nil
}

func (c *Segments) BeginShutdown() { c.tasks.Cancel() }

func (c *Segments) AwaitShutdown(ctx context.Context) error {
	return c.tasks.Wait(ctx)
}

func (c *Segments) newJournal(runID identity.RunID, segmentID identity.SegmentID) (*journal, error) {
	scope, err := newStreamScope(c.epoch, runID, segmentID)
	if err != nil {
		return nil, err
	}
	return newJournal(scope, c.retention)
}
