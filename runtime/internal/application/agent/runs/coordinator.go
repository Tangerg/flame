package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ErrClosed is returned by [Coordinator.Start] once the Coordinator is closing:
// it accepts no new run segments.
var ErrClosed = errors.New("runs: coordinator closed")

// Coordinator owns the live run segments, the per-run event [journal], the
// segment pump that drives a run from Start to terminal, and the
// request-detached task group that keeps runs alive across client disconnects
// and cancels + joins them during shutdown.
//
// Reading Start and the pump explains a Run end to end through canonical
// application events.
type Coordinator struct {
	rootStarts                         RootExecutionStarter
	rootCancellation                   RunningRootCancellationRequester
	conversation                       ConversationReader
	workingContexts                    WorkingContextComposer
	models                             ModelAdmitter
	continuation                       WaitingExecutionContinuer
	waitingRestorer                    WaitingExecutionRestorer
	steering                           RunningExecutionSteerer
	runningSubtreeCanceler             RunningSubtreeCanceler
	waitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	sessionReader                      SessionReader
	sessionCreator                     SessionCreator
	activeRuns                         ActiveRunReader
	interrupts                         PendingInterruptReader
	terminations                       TerminationCommitter
	childStarts                        ChildRunStartCommitter
	resumeClaims                       ResumeClaimCommitter
	checkpoints                        WaitingCheckpointReader
	waitingSubtreeCancellations        WaitingSubtreeCancellationCommitter
	isolation                          IsolationProvider // resolves an isolated session's sandbox copy; nil = isolation off
	newRunID                           func() string
	newSegmentID                       func() string
	// runs reads the durable run record. A subscribe or a steer that cannot be
	// served has to say WHY — waiting, finished, a child, or a segment that has
	// been replaced — and only the durable projection knows: the live registry
	// holds running segments, so every one of those looks identical there.
	runs Projection
	// items resolves the exact parent tool projection a waiting child
	// cancellation replaces.
	items        ItemProjection
	observations ExecutionObserver
	releases     ExecutionReleaser
	finalizer    SegmentFinalizer
	retention    Retention
	epoch        replayEpoch
	tasks        taskgroup.Group
	registry     registry
	admission    *ownership.Gate
	// publications owns durable-write-before-notify ordering.
	publications runPublications
}

// AdmitSelection exposes the same model policy Start applies before any Goal
// can freeze that selection durably.
func (c *Coordinator) AdmitSelection(selection modelref.Selection) error {
	if err := c.models.AdmitSelection(selection); err != nil {
		return fmt.Errorf("%w: %w", ErrUnsupportedModelSelection, err)
	}
	return nil
}

// Dependencies is the complete collaborator set for the user-visible run use
// cases and the segment supervisor they own.
type Dependencies struct {
	RootStarts                         RootExecutionStarter
	Observations                       ExecutionObserver
	Releases                           ExecutionReleaser
	RootCancellation                   RunningRootCancellationRequester
	Conversation                       ConversationReader
	WorkingContexts                    WorkingContextComposer
	Models                             ModelAdmitter
	Continuation                       WaitingExecutionContinuer
	WaitingRestorer                    WaitingExecutionRestorer
	Steering                           RunningExecutionSteerer
	RunningSubtreeCanceler             RunningSubtreeCanceler
	WaitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	Session                            SessionPorts
	Projection                         ProjectionPorts
	Runs                               Projection
	Items                              ItemProjection
	Admissions                         *ownership.Gate
	Isolation                          IsolationProvider // nil disables isolated sessions (their start is refused)
	Now                                func() time.Time
	// Retention bounds every segment's replay window. Zero takes
	// [DefaultRetention], which is also what discovery advertises.
	Retention    Retention
	NewRunID     func() string
	NewSegmentID func() string
	// Invalidations publishes run/session/interrupt/state invalidations for clients that
	// are not following the run. nil disables them (no runtime change stream wired).
	Invalidations invalidation.Publish
}

// NewCoordinator builds the single owner of run use cases and live segments.
func NewCoordinator(deps Dependencies) (*Coordinator, error) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Retention == (Retention{}) {
		deps.Retention = DefaultRetention()
	}
	if deps.Retention.MaxEvents <= 0 || deps.Retention.MaxBytes <= 0 {
		return nil, errors.New("runs: replay retention budgets must be positive")
	}
	required := []struct {
		name  string
		value any
	}{
		{"root execution starter", deps.RootStarts},
		{"execution observer", deps.Observations},
		{"execution releaser", deps.Releases},
		{"root cancellation requester", deps.RootCancellation},
		{"conversation reader", deps.Conversation},
		{"working context composer", deps.WorkingContexts},
		{"model admitter", deps.Models},
		{"waiting execution continuer", deps.Continuation},
		{"waiting execution restorer", deps.WaitingRestorer},
		{"running execution steerer", deps.Steering},
		{"running subtree canceler", deps.RunningSubtreeCanceler},
		{"waiting subtree cancellation preparer", deps.WaitingSubtreeCancellationPreparer},
		{"session reader", deps.Session.Reader},
		{"session creator", deps.Session.Creator},
		{"active run reader", deps.Session.ActiveRuns},
		{"pending interrupt reader", deps.Session.Interrupts},
		{"run termination committer", deps.Session.Terminations},
		{"opening committer", deps.Projection.Openings},
		{"child run start committer", deps.Projection.ChildStarts},
		{"resume claim committer", deps.Projection.ResumeClaims},
		{"event committer", deps.Projection.Events},
		{"tree barrier committer", deps.Projection.Barriers},
		{"waiting checkpoint reader", deps.Projection.Checkpoints},
		{"waiting subtree cancellation committer", deps.Projection.WaitingSubtreeCancellations},
		{"workspace change notifier", deps.Projection.Workspace},
		{"segment finalizer", deps.Projection.Finalizer},
		{"run projection", deps.Runs},
		{"item projection", deps.Items},
		{"admission gate", deps.Admissions},
		{"run id generator", deps.NewRunID},
		{"segment id generator", deps.NewSegmentID},
	}
	for _, required := range required {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("runs: %s is required", required.name)
		}
	}
	if deps.Isolation != nil && dependency.Missing(deps.Isolation) {
		return nil, errors.New("runs: isolation provider must not be typed nil")
	}
	return &Coordinator{
		rootStarts:                         deps.RootStarts,
		rootCancellation:                   deps.RootCancellation,
		conversation:                       deps.Conversation,
		workingContexts:                    deps.WorkingContexts,
		models:                             deps.Models,
		continuation:                       deps.Continuation,
		waitingRestorer:                    deps.WaitingRestorer,
		steering:                           deps.Steering,
		runningSubtreeCanceler:             deps.RunningSubtreeCanceler,
		waitingSubtreeCancellationPreparer: deps.WaitingSubtreeCancellationPreparer,
		sessionReader:                      deps.Session.Reader,
		sessionCreator:                     deps.Session.Creator,
		activeRuns:                         deps.Session.ActiveRuns,
		interrupts:                         deps.Session.Interrupts,
		terminations:                       deps.Session.Terminations,
		childStarts:                        deps.Projection.ChildStarts,
		resumeClaims:                       deps.Projection.ResumeClaims,
		checkpoints:                        deps.Projection.Checkpoints,
		waitingSubtreeCancellations:        deps.Projection.WaitingSubtreeCancellations,
		runs:                               deps.Runs,
		items:                              deps.Items,
		isolation:                          deps.Isolation,
		newRunID:                           deps.NewRunID,
		newSegmentID:                       deps.NewSegmentID,
		observations:                       deps.Observations,
		releases:                           deps.Releases,
		finalizer:                          deps.Projection.Finalizer,
		retention:                          deps.Retention,
		epoch:                              newReplayEpoch(),
		admission:                          deps.Admissions,
		publications: newRunPublications(
			deps.Projection,
			deps.Invalidations,
			deps.Now,
		),
	}, nil
}

// ReplayRetention is the window this Coordinator enforces. Discovery publishes
// it from here rather than from a constant of its own: a number the client is
// told and a number the runtime evicts by must be the same number.
func (c *Coordinator) ReplayRetention() Retention { return c.retention }

// WaitSessionStartable lets an application-owned continuation wait until both
// the live admission gate and the durable Session Run projection are
// free before attempting its own Start. It does not reserve either resource;
// Start remains the authority that acquires them.
func (c *Coordinator) WaitSessionStartable(ctx context.Context, sessionID string) error {
	sess, err := c.sessionReader.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	for {
		if err := c.admission.WaitRunStartable(ctx, sess.ID(), sess.Workspace().Path()); err != nil {
			return err
		}
		startable, err := c.awaitSessionRunSettled(ctx, sess.ID())
		if err != nil || startable {
			return err
		}
	}
}

// awaitSessionRunSettled observes one durable Session Run transition. The
// observation is registered before the read so a Run that settles between them
// still wakes this caller, and it is given back on every way out of this
// function — the gate's own wait cannot see the durable projection, so a lost
// observer is a caller that waits for a change nobody will report again.
func (c *Coordinator) awaitSessionRunSettled(ctx context.Context, sessionID string) (bool, error) {
	changed, stopObserving := c.publications.changes.observe(sessionID)
	defer stopObserving()
	_, active, err := c.activeRuns.ActiveRun(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !active {
		return true, nil
	}
	select {
	case <-ctx.Done():
		return false, context.Cause(ctx)
	case <-changed:
		return false, nil
	}
}

// openSegment attaches an already-staged executor stream, atomically commits
// admission/resume plus opening projections, registers the live owner, then
// activates and pumps according to the command's explicit settlement boundary.
// A fresh caller transfers executor ownership on entry, so startup rejection
// releases it. A continuation caller retains failure ownership until the durable
// opening succeeds because its claim/change transaction defines cleanup order.
// The Run lifetime is detached
// from the request without losing its trace; request cancellation drops only
// that subscriber.
func (c *Coordinator) openSegment(reqCtx context.Context, spec segmentSpec) (iter.Seq[Event], error) {
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
	coordinator    *Coordinator
	spec           segmentSpec
	taskContext    context.Context
	runContext     context.Context
	cancelRun      context.CancelFunc
	releaseTask    func()
	executorEvents iter.Seq[ExecutorEvent]
	journal        *journal
	treeOwner      *runTreeOwner
	routes         *executorRoutes
}

func (c *Coordinator) prepareSegmentStartup(
	requestContext context.Context,
	spec segmentSpec,
) (*segmentStartup, error) {
	taskContext, releaseTask, attached := c.tasks.Attach(requestContext)
	if !attached {
		if spec.Continuation == nil {
			return nil, c.rejectUnadmittedExecution(requestContext, spec.executorRef(), ErrClosed)
		}
		return nil, ErrClosed
	}
	runContext, cancelRun := context.WithCancel(taskContext)
	startup := &segmentStartup{
		coordinator: c,
		spec:        spec,
		taskContext: taskContext,
		runContext:  runContext,
		cancelRun:   cancelRun,
		releaseTask: releaseTask,
	}
	executorEvents, err := c.observations.Observe(runContext, spec.executorRef())
	if err != nil {
		return nil, startup.abort(err)
	}
	startup.executorEvents = executorEvents
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
		cause = s.coordinator.rejectUnadmittedExecution(
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
	err := s.coordinator.registry.Open(Record{
		ID:             spec.RunID,
		SegmentID:      spec.SegmentID,
		SessionID:      spec.SessionID,
		CreatedAt:      spec.CreatedAt,
		ExecutorID:     spec.ExecutorID,
		ModelSelection: spec.ModelSelection,
		Capabilities:   spec.effectiveCapabilities(),
	}, s.treeOwner, func() error {
		var err error
		openings, err = s.coordinator.commitOpening(requestContext, spec, s.routes)
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
		s.coordinator.pump(
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
			if err := s.journal.append(s.coordinator.publications.event(
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
	segmentStartedAt := s.coordinator.publications.nowUTC()
	for _, route := range s.routes.admissionOrder {
		route.segmentStartedAt = segmentStartedAt
	}
}

func (s *segmentStartup) beginExecution() {
	canceled, _ := s.treeOwner.beginExecution(s.taskContext, func(ctx context.Context) error {
		if s.spec.BeginExecution == nil {
			return nil
		}
		err := s.spec.BeginExecution(ctx)
		if err != nil {
			cause := fmt.Errorf("runs: begin execution: %w", err)
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

type routeOpening struct {
	route *executorRoute
	batch reductionBatch
}

func (c *Coordinator) commitOpening(ctx context.Context, spec segmentSpec, routes *executorRoutes) ([]routeOpening, error) {
	ordered, err := routes.unfinishedInPostorder()
	if err != nil {
		return nil, fmt.Errorf("runs: order opening tree: %w", err)
	}
	commitID := newRunCommitID()
	var admission *rundomain.Draft
	var resume *rundomain.TreeResumeDraft
	if spec.Continuation != nil {
		resume = &rundomain.TreeResumeDraft{
			RootRunID: spec.RunID,
			SessionID: spec.SessionID,
			ResumedAt: c.publications.nowUTC(),
			Runs:      make([]rundomain.ResumeDraft, 0, len(ordered)),
		}
	} else {
		admission = &rundomain.Draft{
			RunID:             spec.RunID,
			SessionID:         spec.SessionID,
			SegmentID:         spec.SegmentID,
			ModelSelection:    spec.ModelSelection,
			GoalIncarnationID: spec.GoalIncarnationID,
			Capabilities:      spec.Capabilities,
			CreatedAt:         spec.CreatedAt,
		}
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
				commit, err := NewEventCommit(*reduced.Commit)
				if err != nil {
					return nil, err
				}
				events = append(events, commit)
			}
		}
		if resume != nil {
			resume.Runs = append(resume.Runs, rundomain.ResumeDraft{
				RunID:     route.runID,
				SegmentID: route.segmentID,
			})
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
	if admission != nil {
		opening, err = NewAdmissionOpeningCommit(
			commitID, *admission, spec.InitialSession, spec.SessionReplacement,
			spec.ScheduleFiring, spec.ManualScheduleRun, events,
		)
	} else {
		opening, err = NewResumeOpeningCommit(commitID, *resume, events)
	}
	if err != nil {
		return nil, err
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
func (c *Coordinator) rejectUnadmittedExecution(ctx context.Context, ref ExecutorRef, cause error) error {
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
func (c *Coordinator) Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error) {
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
func (c *Coordinator) SubscribeSnapshot(ctx context.Context, req SubscribeRequest, read func(context.Context, string) error) (Subscription, error) {
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
	if _, err := c.addressLiveSegment(ctx, req.RunID, req.SegmentID); err != nil {
		return Subscription{}, err
	}
	if err := read(ctx, live.record.SessionID); err != nil {
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

func (c *Coordinator) addressSubscription(ctx context.Context, req SubscribeRequest) (liveSegment, error) {
	live, err := c.addressLiveSegment(ctx, req.RunID, req.SegmentID)
	if err != nil {
		return liveSegment{}, err
	}
	if gap := live.record.Capabilities.MissingFrom(req.CallerCapabilities); !gap.IsEmpty() {
		return liveSegment{}, &rundomain.InsufficientCapabilitiesError{RunID: req.RunID, Missing: gap}
	}
	return live, nil
}

// addressLiveSegment resolves the run and segment a control command addresses,
// refusing with the reason the caller can act on.
//
// The durable record is the authority for the refusal, not the live registry:
// the registry holds only running segments, so a run that is waiting, finished,
// or a child all look the same there — one indistinguishable "not found" for
// three situations whose remedies are answering an interrupt, reading the
// transcript, and following rootRunId.
//
// Both entry points into an executing run (subscribe and steer) resolve here, so
// they cannot come to different conclusions about the same run.
func (c *Coordinator) addressLiveSegment(ctx context.Context, runID, segmentID string) (liveSegment, error) {
	// Capture the owner before reading its durable state. Retirement may remove
	// that entry after the read, but cannot invalidate the captured journal.
	// Exclude openings until both reads finish so a newly committed Segment
	// cannot be paired with an absent or predecessor owner.
	c.registry.opening.RLock()
	defer c.registry.opening.RUnlock()
	live, liveFound := c.registry.Get(runID)
	run, ok, err := c.runs.Run(ctx, runID)
	if err != nil {
		return liveSegment{}, fmt.Errorf("runs: read run %q: %w", runID, err)
	}
	if !ok {
		return liveSegment{}, ErrRunNotFound
	}
	if run.Lineage().IsChild() {
		return liveSegment{}, fmt.Errorf("%w: %q", transcript.ErrNotRoot, runID)
	}
	switch status := run.State().Status(); status {
	case rundomain.StatusWaiting:
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunWaiting, runID)
	case rundomain.StatusFinished:
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunFinished, runID)
	case rundomain.StatusRunning:
	default:
		return liveSegment{}, fmt.Errorf("runs: run %q has unknown status %q", runID, status)
	}
	if run.ActiveSegmentID() != segmentID {
		return liveSegment{}, fmt.Errorf("%w: run %q is executing %q", ErrStaleSegment, runID, run.ActiveSegmentID())
	}
	if !liveFound {
		// A Running record whose segment this process does not own. Restart recovery
		// terminalizes orphans before the runtime serves, so this is a broken
		// invariant rather than a state a client can act on — reporting it as one
		// would teach the client a lie about the run.
		return liveSegment{}, fmt.Errorf("runs: run %q is running segment %q with no live stream", runID, segmentID)
	}
	if live.record.SegmentID != segmentID {
		// Opening and owner publication share the fence above; a disagreement is
		// an ownership fault, not a reason to retarget the caller's Segment.
		return liveSegment{}, fmt.Errorf(
			"runs: run %q is executing segment %q but its live owner names %q",
			runID,
			segmentID,
			live.record.SegmentID,
		)
	}
	return live, nil
}

// BeginShutdown prevents new runs and cancels every in-flight pump.
func (c *Coordinator) BeginShutdown() { c.tasks.Cancel() }

// AwaitShutdown joins every in-flight pump after [BeginShutdown].
func (c *Coordinator) AwaitShutdown(ctx context.Context) error {
	return c.tasks.Wait(ctx)
}

func (c *Coordinator) newJournal(runID, segmentID string) (*journal, error) {
	scope, err := newStreamScope(c.epoch, runID, segmentID)
	if err != nil {
		return nil, err
	}
	return newJournal(scope, c.retention)
}
