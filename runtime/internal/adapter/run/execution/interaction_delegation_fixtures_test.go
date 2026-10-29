package execution

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
)

type notifyingRunningSubtreeCanceler struct {
	inner    runs.RunningSubtreeCanceler
	accepted func()
}

func (n notifyingRunningSubtreeCanceler) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	reason string,
) error {
	if err := n.inner.CancelRunningSubtree(ctx, ref, memberID, reason); err != nil {
		return err
	}
	if n.accepted != nil {
		n.accepted()
	}
	return nil
}

type cancelableDelegateModel struct {
	defaults *chat.Options

	childStartedOnce        sync.Once
	rootStartedOnce         sync.Once
	childCallStarted        chan struct{}
	childCallReturned       chan struct{}
	rootContinuationStarted chan struct{}
	releaseRootContinuation chan struct{}
}

func newCancelableDelegateModel() *cancelableDelegateModel {
	return &cancelableDelegateModel{
		defaults:                &chat.Options{Model: "stub-cancelable-delegate"},
		childCallStarted:        make(chan struct{}),
		childCallReturned:       make(chan struct{}),
		rootContinuationStarted: make(chan struct{}),
		releaseRootContinuation: make(chan struct{}),
	}
}

func (c *cancelableDelegateModel) DefaultOptions() chat.Options { return *c.defaults }

func (c *cancelableDelegateModel) Call(
	ctx context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	switch {
	case toolResult(request.Messages, "delegate_task") != "":
		c.rootStartedOnce.Do(func() { close(c.rootContinuationStarted) })
		select {
		case <-c.releaseRootContinuation:
			return interactionUsageTextResponse("root completed after child cancellation", 2, 1), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case userMessagesContain(request.Messages, "child waits for cancellation"):
		c.childStartedOnce.Do(func() { close(c.childCallStarted) })
		<-ctx.Done()
		close(c.childCallReturned)
		return nil, ctx.Err()
	case userMessagesContain(request.Messages, "delegate cancelable work"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_cancelable", Name: "delegate_task",
			Arguments: `{"summary":"cancelable child","instructions":"child waits for cancellation"}`,
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected cancelable Delegate model context")
	}
}

func (c *cancelableDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(c.Call(ctx, request))
}

type waitingDelegateModel struct {
	mu             sync.Mutex
	calls          int
	defaults       *chat.Options
	delegateCallID string
}

func newWaitingDelegateModel() *waitingDelegateModel {
	return &waitingDelegateModel{defaults: &chat.Options{Model: "stub-waiting-delegate"}, delegateCallID: "delegate_waiting"}
}

func (w *waitingDelegateModel) DefaultOptions() chat.Options { return *w.defaults }

func (w *waitingDelegateModel) Call(
	_ context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	w.mu.Lock()
	w.calls++
	w.mu.Unlock()
	switch {
	case toolResult(request.Messages, "ask") != "":
		return interactionUsageTextResponse("child: restored value accepted", 2, 1), nil
	case toolResult(request.Messages, "delegate_task") != "":
		return interactionUsageTextResponse("root: delegated work complete", 2, 1), nil
	case userMessagesContain(request.Messages, "child needs input"):
		return interactionToolResponse(chat.ToolCall{ID: "ask_child", Name: "ask", Arguments: `{}`}, 2, 1), nil
	case userMessagesContain(request.Messages, "delegate waiting work"):
		return interactionToolResponse(chat.ToolCall{
			ID: w.delegateCallID, Name: "delegate_task",
			Arguments: `{"summary":"waiting child","instructions":"child needs input"}`,
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected waiting Delegate model context")
	}
}

func (w *waitingDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(w.Call(ctx, request))
}

func (w *waitingDelegateModel) Calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

// Observations are appended only after the real write-set commits. The fixture
// cannot grant admission or fabricate a Run transition that SQLite would reject.
type delegateProjection struct {
	*segment.Effects
	stores       *persistencefixture.Stores
	history      *runs.ConversationHistory
	sessions     *sessions.Coordinator
	admissions   *ownership.Gate
	isolation    *workspaceadapter.Isolator
	mu           sync.Mutex
	openings     []runs.OpeningCommit
	barriers     []runs.TreeBarrierCommit
	conversation []chat.Message
	reservations map[string]runs.ChildRunStartReservation
	outcomes     map[string]runs.ChildRunStartOutcome
}

type delegateContextInvalidator struct{}

func (delegateContextInvalidator) ForgetSessionContext(string)   {}
func (delegateContextInvalidator) ForgetSession(string)          {}
func (delegateContextInvalidator) ForgetWorkspace(string)        {}
func (delegateContextInvalidator) QuiesceSession(string) error   { return nil }
func (delegateContextInvalidator) QuiesceWorkspace(string) error { return nil }

func newDelegateProjection(t *testing.T, current session.Session, executor *InteractionExecutor) *delegateProjection {
	t.Helper()
	stores := testTrees(t).stores
	if err := stores.Sessions.Insert(t.Context(), current); err != nil {
		t.Fatal(err)
	}
	history, err := persistence.NewConversationStore(stores.ChatHistory)
	if err != nil {
		t.Fatal(err)
	}
	compactions, err := persistence.NewConversationCompactions(history, stores.Runs, persistence.Transactor(stores.Transactor))
	if err != nil {
		t.Fatal(err)
	}
	goalRuns, err := goals.NewRunRecorder(stores.Goals)
	if err != nil {
		t.Fatal(err)
	}
	actors, err := plugins.NewRunActorAdmission(stores.Installations)
	if err != nil {
		t.Fatal(err)
	}
	effects, err := segment.New(segment.Config{
		Interrupts: stores.Interrupts, Sessions: stores.Sessions,
		Transcript: stores.Transcript, ItemReplacer: stores.Transcript,
		ModelInvocations: stores.ModelInvocations, ToolInvocations: stores.ToolInvocations,
		Conversation: history, Compactions: compactions, ContextState: delegateContextInvalidator{},
		State: stores.Runs, RunProgress: stores.Runs,
		ExecutorCheckpoints: stores.ExecutorCheckpoints, ExecutionTrees: stores.ExecutorCheckpoints,
		ChildRunStarts: stores.ChildRunStarts, Tx: segment.Transactor(stores.Transactor),
		CommandAcceptances: stores.CommandAcceptances, ActorAuthority: actors,
		GoalRuns: goalRuns, ToolResults: stores.ToolResults,
		RememberedRules: stores.ApprovalRules, AcceptedContinuations: stores.AcceptedContinuations,
		PluginBindings: func(context.Context, string, runs.ExecutorCheckpoint, func(context.Context) error) error {
			return errors.New("Delegate fixture has no installed plugin host")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := runs.NewConversationHistory(history)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := persistence.NewSessionStores(persistence.SessionStoresConfig{
		ResourceRetirements: stores.ResourceRetirements,
		CommandAcceptances:  stores.CommandAcceptances, Sessions: stores.Sessions,
		Transcript: stores.Transcript, Interrupts: stores.Interrupts, Runs: stores.Runs,
		ExecutorCheckpoints: stores.ExecutorCheckpoints, History: messages, Plan: stores.Plan,
		ApprovalRules: stores.ApprovalRules, ApprovalModes: stores.ApprovalModes,
		ToolResults: stores.ToolResults, ChildRunStarts: stores.ChildRunStarts,
		Goals: stores.Goals, GoalRuns: goalRuns,
		Tx: persistence.Transactor(stores.Transactor), ReadSnapshot: persistence.Transactor(stores.ReadSnapshot),
	})
	if err != nil {
		t.Fatal(err)
	}
	plans, err := sessions.NewPlanCoordinator(sessions.PlanDependencies{Store: stores.Plan})
	if err != nil {
		t.Fatal(err)
	}
	admissions, err := ownership.NewGate(ownership.GateConfig{ResolveTree: filesystem.CanonicalPath, RetirementPending: stores.ResourceRetirements.Pending})
	if err != nil {
		t.Fatal(err)
	}
	isolation, err := workspaceadapter.NewIsolator(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := isolation.Close(); err != nil {
			t.Error(err)
		}
	})
	sessionCoordinator, err := sessions.New(sessions.Dependencies{
		Sessions: stores.Sessions, Interrupts: stores.Interrupts, Transcript: stores.Transcript, Runs: stores.Runs,
		Snapshots: writes, MaterialSnapshots: writes, Writes: writes,
		TransientState: delegateContextInvalidator{}, ExecutionReleaser: executor,
		Paths: workspaceadapter.Resolver{}, Models: acceptAllRunModelInputs{},
		DefaultModelSelection: testDefaultSelection(), Admissions: admissions,
		Plan:      sessions.PlanServices{Boundaries: stores.Plan, Replacements: plans},
		Mutations: stores.WorkspaceMutations, Retirements: stores.ResourceRetirements, Goals: goals.NewSessionMutations(), Sandbox: isolation,
		NewID:     func() string { return "session_" + uuid.NewString() },
		NewRunID:  func() string { return "run_" + uuid.NewString() },
		NewItemID: func() string { return "item_" + uuid.NewString() },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &delegateProjection{
		Effects: effects, stores: stores, history: messages, sessions: sessionCoordinator,
		admissions: admissions, isolation: isolation,
		reservations: make(map[string]runs.ChildRunStartReservation),
		outcomes:     make(map[string]runs.ChildRunStartOutcome),
	}
}

func (d *delegateProjection) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if err := d.Effects.CommitOpening(ctx, opening); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recordOpening(opening)
	return nil
}

func (d *delegateProjection) ReserveChildRunStart(ctx context.Context, reservation runs.ChildRunStartReservation) error {
	if err := d.Effects.ReserveChildRunStart(ctx, reservation); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reservations[reservation.Member.MemberID] = reservation
	return nil
}

func (d *delegateProjection) AbortChildRunStart(ctx context.Context, reservation runs.ChildRunStartReservation) error {
	if err := d.Effects.AbortChildRunStart(ctx, reservation); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.outcomes[reservation.Member.MemberID] = runs.ChildRunStartAborted
	return nil
}

func (d *delegateProjection) CommitEvent(ctx context.Context, commit runs.EventCommit) error {
	if err := d.Effects.CommitEvent(ctx, commit); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recordEvent(commit)
	return nil
}

func (d *delegateProjection) CommitTreeBarrier(ctx context.Context, barrier runs.TreeBarrierCommit) error {
	if err := d.Effects.CommitTreeBarrier(ctx, barrier); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, commit := range barrier.Runs() {
		d.recordEvent(commit)
	}
	d.barriers = append(d.barriers, barrier)
	return nil
}

func (*delegateProjection) Nudge(string, []string)                    {}
func (*delegateProjection) Finish(context.Context, runs.Finish) error { return nil }

func (d *delegateProjection) ListRuns(ctx context.Context, sessionID string) ([]run.Run, error) {
	return d.stores.Runs.ListRuns(ctx, sessionID)
}
func (d *delegateProjection) Run(ctx context.Context, runID string) (run.Run, bool, error) {
	return d.stores.Runs.Run(ctx, runID)
}
func (d *delegateProjection) Tree(ctx context.Context, runID string) ([]run.Run, error) {
	return d.stores.Runs.Tree(ctx, runID)
}
func (d *delegateProjection) Item(ctx context.Context, itemID string) (transcript.Item, bool, error) {
	return d.stores.Transcript.Item(ctx, itemID)
}

func (d *delegateProjection) recordOpening(opening runs.OpeningCommit) {
	d.openings = append(d.openings, opening)
	for _, commit := range opening.Events() {
		d.recordEvent(commit)
	}
}

func (d *delegateProjection) recordEvent(commit runs.EventCommit) {
	for _, message := range commit.ConversationMessages {
		d.conversation = append(d.conversation, message.Clone())
	}
}

func (d *delegateProjection) CommitExecutionTree(ctx context.Context, tree runs.ExecutionTreeCommit) error {
	if err := d.Effects.CommitExecutionTree(ctx, tree); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, started := range tree.StartedChildren {
		d.outcomes[started.Reservation().Member.MemberID] = runs.ChildRunStarted
		d.recordOpening(started.Opening())
	}
	for _, aborted := range tree.AbortedChildren {
		d.outcomes[aborted.Member.MemberID] = runs.ChildRunStartAborted
	}
	for _, commit := range tree.Events {
		d.recordEvent(commit)
	}
	return nil
}

// The Run pump receives the original receipt; observations never acknowledge it.
type recordingExecutionObserver struct {
	source    runs.ExecutionObserver
	completed chan []runs.ExecutorEvent
}

func newRecordingExecutionObserver(source runs.ExecutionObserver) *recordingExecutionObserver {
	return &recordingExecutionObserver{source: source, completed: make(chan []runs.ExecutorEvent, 1)}
}

func (o *recordingExecutionObserver) Observe(ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	sequence, err := o.source.Observe(ctx, ref)
	if err != nil {
		return nil, err
	}
	return func(yield func(runs.ExecutorEvent) bool) {
		var observed []runs.ExecutorEvent
		defer func() { o.completed <- observed }()
		for event := range sequence {
			projection := event
			if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
				projection.Payload = commit.Fact()
			}
			if tree, ok := projection.Payload.(runs.ExecutionTreeSettled); ok {
				observed = append(observed, tree.Facts...)
			} else {
				observed = append(observed, projection)
			}
			if !yield(event) {
				return
			}
		}
	}, nil
}

type observingWaitingCancellationPreparer struct {
	source         runs.WaitingSubtreeCancellationPreparer
	beforeContinue func()
}

func (p *observingWaitingCancellationPreparer) PrepareWaitingSubtreeCancellation(ctx context.Context, request runs.WaitingSubtreeCancellationRequest) (runs.PreparedWaitingSubtreeCancellation, error) {
	prepared, err := p.source.PrepareWaitingSubtreeCancellation(ctx, request)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	return runs.NewPreparedWaitingSubtreeCancellation(prepared.CanceledMemberIDs(), prepared.PendingInterruptions(), prepared.Checkpoint(), observedWaitingCancellation{prepared: prepared, beforeContinue: p.beforeContinue})
}

type observedWaitingCancellation struct {
	prepared       runs.PreparedWaitingSubtreeCancellation
	beforeContinue func()
}

func (c observedWaitingCancellation) Apply(disposition runs.WaitingSubtreeDisposition) error {
	return c.prepared.Apply(disposition)
}
func (c observedWaitingCancellation) Discard() error { return c.prepared.Discard() }
func (c observedWaitingCancellation) Continue(ctx context.Context) error {
	if c.beforeContinue != nil {
		c.beforeContinue()
	}
	return c.prepared.Continue(ctx)
}
