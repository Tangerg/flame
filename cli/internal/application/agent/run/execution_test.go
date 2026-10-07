package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

func unavailableReplayPolicy(t testing.TB) mutation.ReplayPolicy {
	t.Helper()
	policy, err := mutation.UnavailableReplayPolicy(time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

type invalidOpeningRuntime struct{ *runtimefixture.Runtime }

type observedOpeningRuntime struct {
	*runtimefixture.Runtime
	observation context.Context
}

func (r *observedOpeningRuntime) StartRun(ctx context.Context, input prompt.StartRun) (conversation.SegmentStream, error) {
	r.observation = ctx
	return r.Runtime.StartRun(ctx, input)
}

type treeReconnectRuntime struct {
	*runtimefixture.Runtime
	initial       conversation.SegmentStream
	rebound       conversation.SegmentStream
	subscriptions []conversation.SubscribeRun
}

func (t *treeReconnectRuntime) StartRun(context.Context, prompt.StartRun) (conversation.SegmentStream, error) {
	return t.initial, nil
}

func (t *treeReconnectRuntime) SubscribeRun(_ context.Context, input conversation.SubscribeRun) (conversation.SegmentStream, error) {
	t.subscriptions = append(t.subscriptions, input)
	return t.rebound, nil
}

type treeInterruptRuntime struct {
	treeReconnectRuntime
	continued   conversation.SegmentStream
	resumptions []conversation.ResumeRun
}

func (t *treeInterruptRuntime) ResumeRun(_ context.Context, input conversation.ResumeRun) (conversation.SegmentStream, error) {
	t.resumptions = append(t.resumptions, input.Clone())
	return t.continued, nil
}

type refusingCancellationRuntime struct {
	*runtimefixture.Runtime

	failure  error
	mu       sync.Mutex
	attempts []conversation.CancelRun
}

type misdirectedCancellationRuntime struct{ *runtimefixture.Runtime }

type uncertainAcknowledgementRuntime struct {
	*runtimefixture.Runtime

	mu             sync.Mutex
	startAttempts  []prompt.StartRun
	resumeAttempts []conversation.ResumeRun
	cancelAttempts []conversation.CancelRun
	startStream    conversation.SegmentStream
	resumeStream   conversation.SegmentStream
	cancelResult   conversation.RunCancellation
}

func (u *uncertainAcknowledgementRuntime) StartRun(ctx context.Context, input prompt.StartRun) (conversation.SegmentStream, error) {
	u.mu.Lock()
	u.startAttempts = append(u.startAttempts, input.Clone())
	attempt := len(u.startAttempts)
	cached := u.startStream
	u.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	opened, err := u.Runtime.StartRun(ctx, input)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	u.mu.Lock()
	u.startStream = opened
	u.mu.Unlock()
	return conversation.SegmentStream{}, fmt.Errorf("start acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (u *uncertainAcknowledgementRuntime) ResumeRun(ctx context.Context, input conversation.ResumeRun) (conversation.SegmentStream, error) {
	u.mu.Lock()
	u.resumeAttempts = append(u.resumeAttempts, input.Clone())
	attempt := len(u.resumeAttempts)
	cached := u.resumeStream
	u.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	continued, err := u.Runtime.ResumeRun(ctx, input)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	u.mu.Lock()
	u.resumeStream = continued
	u.mu.Unlock()
	return conversation.SegmentStream{}, fmt.Errorf("resume acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (u *uncertainAcknowledgementRuntime) CancelRun(
	ctx context.Context,
	input conversation.CancelRun,
) (conversation.RunCancellation, error) {
	u.mu.Lock()
	u.cancelAttempts = append(u.cancelAttempts, input)
	attempt := len(u.cancelAttempts)
	cached := u.cancelResult
	u.mu.Unlock()
	if attempt > 1 {
		return cached, nil
	}
	result, err := u.Runtime.CancelRun(ctx, input)
	if err != nil {
		return conversation.RunCancellation{}, err
	}
	u.mu.Lock()
	u.cancelResult = result
	u.mu.Unlock()
	return conversation.RunCancellation{}, fmt.Errorf("cancel acknowledgement timed out: %w", context.DeadlineExceeded)
}

func (u *uncertainAcknowledgementRuntime) attempts() ([]prompt.StartRun, []conversation.ResumeRun) {
	u.mu.Lock()
	defer u.mu.Unlock()
	starts := make([]prompt.StartRun, len(u.startAttempts))
	for index, attempt := range u.startAttempts {
		starts[index] = attempt.Clone()
	}
	resumes := make([]conversation.ResumeRun, len(u.resumeAttempts))
	for index, attempt := range u.resumeAttempts {
		resumes[index] = attempt.Clone()
	}
	return starts, resumes
}

func (u *uncertainAcknowledgementRuntime) cancellationAttempts() []conversation.CancelRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.cancelAttempts)
}

func (i invalidOpeningRuntime) StartRun(ctx context.Context, input prompt.StartRun) (conversation.SegmentStream, error) {
	opened, err := i.Runtime.StartRun(ctx, input)
	if err == nil {
		opened.Events = nil
	}
	return opened, err
}

func (r *refusingCancellationRuntime) CancelRun(
	_ context.Context,
	input conversation.CancelRun,
) (conversation.RunCancellation, error) {
	r.mu.Lock()
	r.attempts = append(r.attempts, input)
	r.mu.Unlock()
	return conversation.RunCancellation{}, r.failure
}

func (r *refusingCancellationRuntime) cancellationAttempts() []conversation.CancelRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.attempts)
}

func (m misdirectedCancellationRuntime) CancelRun(
	ctx context.Context,
	input conversation.CancelRun,
) (conversation.RunCancellation, error) {
	result, err := m.Runtime.CancelRun(ctx, input)
	if err == nil {
		result.Canceled.ID = "run_misdirected"
		result.Root.ID = "run_misdirected"
	}
	return result, err
}

type recordingRenderer struct {
	events []conversation.RunEvent
	err    error
}

func (r *recordingRenderer) Begin(conversation.Run, prompt.RunOptions) error { return r.err }

func (r *recordingRenderer) Render(event conversation.RunEvent) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event.Clone())
	return nil
}

func (r *recordingRenderer) Reconcile(snapshot conversation.SessionSnapshot) error {
	if r.err != nil {
		return r.err
	}
	for _, block := range snapshot.Transcript {
		r.events = append(r.events, conversation.RunEvent{EventID: "snapshot:" + block.ID, RunID: "snapshot", SegmentID: "snapshot", Event: conversation.BlockCompleted{Block: block.Clone()}})
	}
	return nil
}

func (*recordingRenderer) Close() error { return nil }

func advertisedReplayPolicy(t *testing.T) mutation.ReplayPolicy {
	t.Helper()
	capability, err := replay.NewCapability("runtime-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := mutation.NewReplayPolicy(capability, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func testRunStart(sessionID, text string) prompt.StartRun {
	return prompt.StartRun{
		SessionID: sessionID, Message: prompt.Message{Text: text},
		Options: prompt.RunOptions{},
	}
}

func TestOpenRunChecksReplayAdmissionBeforeEveryAttempt(t *testing.T) {
	base := runtimefixture.New()
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	admissions := 0
	_, err := openRun(t.Context(), runtime, prompt.StartRun{
		CommandID: "cli_dddddddddddddddddddddddddddddddd", SessionID: "ses_demo_1",
		Message: prompt.Message{Text: "admit every attempt"}, Options: prompt.RunOptions{},
	}, func() error {
		admissions++
		if admissions > 1 {
			return mutation.ErrReplayGuaranteeUnavailable
		}
		return nil
	})
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("open run error = %v", err)
	}
	starts, _ := runtime.attempts()
	if admissions != 2 || len(starts) != 1 {
		t.Fatalf("open run admissions=%d attempts=%d", admissions, len(starts))
	}
}

func TestExecuteDrivesApprovalAcrossSegments(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Instant = true
	session, _ := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	renderer := new(recordingRenderer)
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: renderer,
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "fix it"),
		ApproveAll:   true})
	if err != nil {
		t.Fatal(err)
	}
	segments := make(map[string]struct{})
	for _, event := range renderer.events {
		segments[event.SegmentID] = struct{}{}
	}
	if len(segments) != 2 {
		t.Fatalf("segments = %v", segments)
	}
}

func TestExecuteConfirmsTimedOutMutationsWithoutChangingIdentity(t *testing.T) {
	base := runtimefixture.New()
	base.Instant = true
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		ReplayPolicy: advertisedReplayPolicy(t),
		Start:        testRunStart(session.ID, "confirm every mutation"),
		ApproveAll:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	starts, resumes := runtime.attempts()
	if len(starts) != 2 || starts[0].CommandID == "" || starts[0].CommandID != starts[1].CommandID {
		t.Fatalf("start confirmation attempts = %+v", starts)
	}
	if len(resumes) != 2 || resumes[0].CommandID == "" || resumes[0].CommandID != resumes[1].CommandID {
		t.Fatalf("resume confirmation attempts = %+v", resumes)
	}
}

func TestExecuteDoesNotRetryATimedOutStartWithoutRuntimeReplayCapability(t *testing.T) {
	base := runtimefixture.New()
	base.Instant = true
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "do not guess at replay"),
	})
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("one-shot error = %v", err)
	}
	starts, _ := runtime.attempts()
	if len(starts) != 1 || starts[0].CommandID == "" {
		t.Fatalf("unprotected one-shot starts = %+v", starts)
	}
}

func TestExecuteLeavesQuestionsParked(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Instant = true
	runtime.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{
			Interrupts: []conversation.Interrupt{conversation.Question{
				ItemID: "q_1", Title: "Target",
				Fields: []conversation.QuestionField{{Prompt: "Target", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
			}},
			Continue: func([]conversation.InterruptAnswer) []runtimefixture.Step {
				return []runtimefixture.Step{{Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}}}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "ask"),
	})
	if _, ok := errors.AsType[*interruptRequiredError](err); !ok {
		t.Fatalf("error = %v", err)
	}
	snapshot, snapshotErr := runtime.GetSession(t.Context(), session.ID)
	active, activeOK := snapshot.ActiveRun()
	if snapshotErr != nil || !activeOK || active.Status != protocol.RunStatusWaiting {
		t.Fatalf("snapshot = %+v, %v", snapshot, snapshotErr)
	}
}

func TestExecuteReconnectsOnlyTheCurrentSegment(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Faults = []runtimefixture.SubscriptionFault{{Kind: runtimefixture.FaultDisconnect, After: 1}}
	runtime.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Delay: 30 * time.Millisecond, Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant, Text: "done"}}},
			{Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	session, _ := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: new(recordingRenderer),
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "fix"),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteReconnectsWhenAChildFinishesBeforeTheStreamDisconnects(t *testing.T) {
	base := runtimefixture.New()
	session, err := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	root := conversation.Run{
		ID: "run_root", SessionID: session.ID, Lineage: conversation.RootRunLineage(),
		Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_root",
	}
	lineage, err := conversation.NewChildRunLineage("run_child", "item_delegate", root.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := conversation.Run{
		ID: "run_child", SessionID: session.ID, Lineage: lineage,
		Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_child",
	}
	event := func(id, runID, segmentID string, payload conversation.Event) conversation.RunEvent {
		return conversation.RunEvent{
			EventID: id, RunID: runID, SegmentID: segmentID, StreamSegmentID: root.ActiveSegmentID,
			At: time.Unix(1, 0), Event: payload,
		}
	}
	initialEvents := []conversation.RunEvent{
		event("event_root_started", root.ID, root.ActiveSegmentID, conversation.SegmentStarted{Run: root}),
		event("event_child_started", child.ID, child.ActiveSegmentID, conversation.SegmentStarted{Run: child}),
		event("event_child_finished", child.ID, child.ActiveSegmentID, finishedSegment(child, conversation.Outcome{Status: protocol.OutcomeCanceled, Detail: "child canceled"})),
	}
	rootFinished := event("event_root_finished", root.ID, root.ActiveSegmentID, finishedSegment(root, conversation.Outcome{Status: protocol.OutcomeCompleted}))
	stream := func(events []conversation.RunEvent, terminal error) conversation.EventStream {
		return func(yield func(conversation.RunEvent, error) bool) {
			for _, item := range events {
				if !yield(item, nil) {
					return
				}
			}
			if terminal != nil {
				yield(conversation.RunEvent{}, terminal)
			}
		}
	}
	runtime := &treeReconnectRuntime{
		Runtime: base,
		initial: conversation.SegmentStream{
			RunID: root.ID, SegmentID: root.ActiveSegmentID, UserItemID: "item_user",
			Events: stream(initialEvents, conversation.ErrDisconnected),
		},
		rebound: conversation.SegmentStream{
			RunID: root.ID, SegmentID: root.ActiveSegmentID,
			Events: stream([]conversation.RunEvent{rootFinished}, nil),
		},
	}
	renderer := new(recordingRenderer)
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: renderer,
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "delegate then continue"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.subscriptions) != 1 || runtime.subscriptions[0].AfterEventID != "event_child_finished" {
		t.Fatalf("subscriptions = %+v", runtime.subscriptions)
	}
	if !slices.ContainsFunc(renderer.events, func(item conversation.RunEvent) bool {
		return item.EventID == rootFinished.EventID
	}) {
		t.Fatalf("root terminal event was not rendered: %+v", renderer.events)
	}
}

func TestExecuteResumesTheCompleteTreeAfterItsRootSuspends(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnect_before_root_suspends=%t", disconnect), func(t *testing.T) {
			base := runtimefixture.New()
			session, err := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			root := conversation.Run{
				ID: "run_root", SessionID: session.ID, Lineage: conversation.RootRunLineage(),
				Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_root",
			}
			event := func(id string, run conversation.Run, streamSegment string, payload conversation.Event) conversation.RunEvent {
				return conversation.RunEvent{
					EventID: id, RunID: run.ID, SegmentID: run.ActiveSegmentID, StreamSegmentID: streamSegment,
					At: time.Unix(1, 0), Event: payload,
				}
			}
			initial := []conversation.RunEvent{event("event_root_started", root, root.ActiveSegmentID, conversation.SegmentStarted{Run: root})}
			resumedRoot := root
			resumedRoot.ActiveSegmentID = "seg_root_resumed"
			continued := []conversation.RunEvent{event("event_root_resumed", resumedRoot, resumedRoot.ActiveSegmentID, conversation.SegmentStarted{Run: resumedRoot})}
			var wantAnswers []conversation.InterruptAnswer
			for _, suffix := range []string{"a", "b"} {
				lineage, err := conversation.NewChildRunLineage("run_child_"+suffix, "item_delegate_"+suffix, root.ID, root.ID)
				if err != nil {
					t.Fatal(err)
				}
				child := conversation.Run{
					ID: "run_child_" + suffix, SessionID: session.ID, Lineage: lineage,
					Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_child_" + suffix,
				}
				tool := &conversation.ToolCall{Kind: conversation.ToolRead, Name: "read", Status: conversation.ToolRunning}
				block := conversation.Block{ID: "item_approval_" + suffix, RunID: child.ID, Status: conversation.BlockStatusRunning, Kind: conversation.BlockTool, Tool: tool}
				approval := conversation.Approval{RunID: child.ID, ItemID: block.ID, Title: "Inspect " + suffix, Tool: tool}
				initial = append(initial,
					event("event_child_started_"+suffix, child, root.ActiveSegmentID, conversation.SegmentStarted{Run: child}),
					event("event_approval_started_"+suffix, child, root.ActiveSegmentID, conversation.BlockStarted{Block: block}),
					event("event_child_waiting_"+suffix, child, root.ActiveSegmentID, parkedSegment(child, approval)),
				)
				wantAnswers = append(wantAnswers, conversation.InterruptAnswer{ItemID: block.ID, Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove}})
				child.ActiveSegmentID += "_resumed"
				block = block.Clone()
				block.Status, block.Tool.Status = conversation.BlockStatusCompleted, conversation.ToolOK
				continued = append(continued,
					event("event_child_resumed_"+suffix, child, resumedRoot.ActiveSegmentID, conversation.SegmentStarted{Run: child}),
					event("event_approval_completed_"+suffix, child, resumedRoot.ActiveSegmentID, conversation.BlockCompleted{Block: block}),
					event("event_child_finished_"+suffix, child, resumedRoot.ActiveSegmentID, finishedSegment(child, conversation.Outcome{Status: protocol.OutcomeCompleted})),
				)
			}
			rootSuspended := event("event_root_suspended", root, root.ActiveSegmentID, parkedSegment(root))
			continued = append(continued, event("event_root_finished", resumedRoot, resumedRoot.ActiveSegmentID, finishedSegment(resumedRoot, conversation.Outcome{Status: protocol.OutcomeCompleted})))
			stream := func(events []conversation.RunEvent, terminal error) conversation.EventStream {
				return func(yield func(conversation.RunEvent, error) bool) {
					for _, item := range events {
						if !yield(item, nil) {
							return
						}
					}
					if terminal != nil {
						yield(conversation.RunEvent{}, terminal)
					}
				}
			}
			var terminal error
			if disconnect {
				terminal = conversation.ErrDisconnected
			} else {
				initial = append(initial, rootSuspended)
			}
			runtime := &treeInterruptRuntime{
				treeReconnectRuntime: treeReconnectRuntime{
					Runtime: base,
					initial: conversation.SegmentStream{RunID: root.ID, SegmentID: root.ActiveSegmentID, UserItemID: "item_user", Events: stream(initial, terminal)},
					rebound: conversation.SegmentStream{RunID: root.ID, SegmentID: root.ActiveSegmentID, Events: stream([]conversation.RunEvent{rootSuspended}, nil)},
				},
				continued: conversation.SegmentStream{RunID: root.ID, SegmentID: resumedRoot.ActiveSegmentID, Events: stream(continued, nil)},
			}
			if err := Execute(t.Context(), Invocation{
				Runtime: runtime, Renderer: new(recordingRenderer), ReplayPolicy: unavailableReplayPolicy(t),
				Start: testRunStart(session.ID, "approve the delegated tree"), ApproveAll: true}); err != nil {
				t.Fatal(err)
			}
			if len(runtime.resumptions) != 1 {
				t.Fatalf("resumptions = %+v", runtime.resumptions)
			}
			resumed := runtime.resumptions[0]
			if !resumed.Equal(conversation.ResumeRun{CommandID: resumed.CommandID, RunID: root.ID, Answers: wantAnswers}) {
				t.Fatalf("resume lost part of the tree's pending set: %+v", resumed)
			}
			if disconnect && (len(runtime.subscriptions) != 1 || runtime.subscriptions[0].AfterEventID != "event_child_waiting_b") {
				t.Fatalf("subscriptions = %+v", runtime.subscriptions)
			}
		})
	}
}

func TestExecutePropagatesRendererFailure(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Instant = true
	session, _ := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	want := errors.New("write failed")
	err := Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: &recordingRenderer{err: want},
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "fix"),
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteReleasesAnUnconsumedOpeningWithoutCancelingItsRun(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	runtime := &observedOpeningRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("output unavailable")
	err = Execute(t.Context(), Invocation{
		Runtime: runtime, Renderer: &recordingRenderer{err: want},
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "keep executing after observation fails"),
	})
	if !errors.Is(err, want) {
		t.Fatalf("execute = %v", err)
	}
	if runtime.observation.Err() == nil {
		t.Fatal("execute retained the unconsumed opening observation")
	}
	if t.Context().Err() != nil {
		t.Fatal("releasing observation canceled its caller")
	}
	snapshot, err := base.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := snapshot.LatestRun()
	if !ok || run.Status != protocol.RunStatusRunning {
		t.Fatalf("releasing observation canceled accepted execution: %+v", run)
	}
	if _, err := base.CancelRun(t.Context(), conversation.CancelRun{
		CommandID: mutation.NewCommandID(), RunID: run.ID, Reason: "test cleanup",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteReportsExplicitCancellationFailure(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	cleanupFailure := errors.New("cancellation refused")
	runtime := &refusingCancellationRuntime{Runtime: base, failure: cleanupFailure}
	session, err := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = Execute(ctx, Invocation{
		Runtime: runtime, Renderer: &cancelingRenderer{recordingRenderer: recordingRenderer{err: renderFailure}, cancel: cancel},
		ReplayPolicy: advertisedReplayPolicy(t),
		Start:        testRunStart(session.ID, "fail and clean up"),
	})
	if !errors.Is(err, renderFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("joined execution failure = %v", err)
	}
	attempts := runtime.cancellationAttempts()
	if len(attempts) != 1 || attempts[0].CommandID == "" || attempts[0].RunID == "" ||
		attempts[0].Reason != "CLI execution canceled" {
		t.Fatalf("abandoned run cleanup attempts = %+v", attempts)
	}
	commandID := mutation.NewCommandID()
	if _, cancelErr := base.CancelRun(t.Context(), conversation.CancelRun{
		CommandID: commandID, RunID: attempts[0].RunID, Reason: "test cleanup",
	}); cancelErr != nil {
		t.Fatal(cancelErr)
	}
}

func TestExecuteConfirmsTimedOutCleanupWithoutChangingIdentity(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	runtime := &uncertainAcknowledgementRuntime{Runtime: base}
	session, err := runtime.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = Execute(ctx, Invocation{
		Runtime: runtime, Renderer: &cancelingRenderer{recordingRenderer: recordingRenderer{err: renderFailure}, cancel: cancel},
		ReplayPolicy: advertisedReplayPolicy(t),
		Start:        testRunStart(session.ID, "confirm cleanup"),
	})
	if !errors.Is(err, renderFailure) {
		t.Fatalf("execution error = %v", err)
	}
	attempts := runtime.cancellationAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[0].RunID != attempts[1].RunID {
		t.Fatalf("cleanup confirmation attempts = %+v", attempts)
	}
}

func TestExecuteRejectsAMisdirectedExplicitCancellation(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	session, err := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	renderFailure := errors.New("output unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = Execute(ctx, Invocation{
		Runtime: misdirectedCancellationRuntime{Runtime: base}, Renderer: &cancelingRenderer{recordingRenderer: recordingRenderer{err: renderFailure}, cancel: cancel},
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "validate cleanup receipt"),
	})
	if !errors.Is(err, renderFailure) || !strings.Contains(err.Error(), "cancel requested run") ||
		!strings.Contains(err.Error(), "run_misdirected") {
		t.Fatalf("misdirected cleanup failure = %v", err)
	}
}

func TestExecutePreservesRunWhenOpeningObservationIsInvalid(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}}}}
	}
	session, _ := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	err := Execute(t.Context(), Invocation{
		Runtime: invalidOpeningRuntime{Runtime: base}, Renderer: new(recordingRenderer),
		ReplayPolicy: unavailableReplayPolicy(t),
		Start:        testRunStart(session.ID, "start"),
	})
	if err == nil {
		t.Fatal("invalid opening stream was accepted")
	}
	snapshot, snapshotErr := base.GetSession(t.Context(), session.ID)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	latest, ok := snapshot.LatestRun()
	if !ok || latest.Status != protocol.RunStatusRunning {
		t.Fatalf("invalid opening canceled run: %+v", snapshot.Runs)
	}
}

type cancelingRenderer struct {
	recordingRenderer
	cancel context.CancelFunc
}

func (r *cancelingRenderer) Begin(run conversation.Run, options prompt.RunOptions) error {
	r.cancel()
	return r.recordingRenderer.Begin(run, options)
}

type outageRuntime struct {
	*refusingCancellationRuntime
	remaining  int
	subscribed int
	failure    error
}

func (r *outageRuntime) SubscribeRun(ctx context.Context, command conversation.SubscribeRun) (conversation.SegmentStream, error) {
	r.subscribed++
	if r.remaining > 0 {
		r.remaining--
		return conversation.SegmentStream{}, r.failure
	}
	return r.Runtime.SubscribeRun(ctx, command)
}

func TestExecuteRecoversAfterProlongedOutageWithoutCancelingRun(t *testing.T) {
	workspace := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		base := runtimefixture.New()
		base.Faults = []runtimefixture.SubscriptionFault{{Kind: runtimefixture.FaultDisconnect, After: 1}}
		base.Script = func(string) runtimefixture.Script {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: 3 * time.Minute, Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant, Text: "done"}}},
				{Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		runtime := &outageRuntime{refusingCancellationRuntime: &refusingCancellationRuntime{Runtime: base, failure: errors.New("unexpected cancel")}, remaining: 100, failure: conversation.ErrDisconnected}
		session, err := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: workspace})
		if err != nil {
			t.Fatal(err)
		}
		renderer := new(recordingRenderer)
		err = Execute(t.Context(), Invocation{Runtime: runtime, Renderer: renderer, Start: testRunStart(session.ID, "continue"), ReplayPolicy: unavailableReplayPolicy(t)})
		if err != nil {
			t.Fatal(err)
		}
		if runtime.subscribed != 101 {
			t.Fatalf("subscriptions=%d", runtime.subscribed)
		}
		if attempts := runtime.cancellationAttempts(); len(attempts) != 0 {
			t.Fatalf("observation canceled Run: %+v", attempts)
		}
		snapshot, err := base.GetSession(t.Context(), session.ID)
		latest, found := snapshot.LatestRun()
		if err != nil || !found || latest.Outcome.Status != protocol.OutcomeCompleted {
			t.Fatalf("terminal: %+v, %v", latest, err)
		}
	})
}

func TestExecutePreservesRunAfterPermanentObservationFailure(t *testing.T) {
	base := runtimefixture.New()
	base.Faults = []runtimefixture.SubscriptionFault{{Kind: runtimefixture.FaultDisconnect, After: 1}}
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}}}}
	}
	runtime := &outageRuntime{refusingCancellationRuntime: &refusingCancellationRuntime{Runtime: base}, remaining: 1, failure: conversation.ErrEventConflict}
	session, err := base.CreateSession(t.Context(), conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = Execute(t.Context(), Invocation{Runtime: runtime, Renderer: new(recordingRenderer), Start: testRunStart(session.ID, "continue"), ReplayPolicy: unavailableReplayPolicy(t)})
	if !errors.Is(err, conversation.ErrEventConflict) {
		t.Fatalf("observation error: %v", err)
	}
	if attempts := runtime.cancellationAttempts(); len(attempts) != 0 {
		t.Fatalf("observation canceled Run: %+v", attempts)
	}
	snapshot, err := base.GetSession(t.Context(), session.ID)
	active, found := snapshot.ActiveRun()
	if err != nil || !found {
		t.Fatalf("lost active Run: %+v, %v", snapshot, err)
	}
	if _, err := base.CancelRun(t.Context(), conversation.CancelRun{CommandID: mutation.NewCommandID(), RunID: active.ID, Reason: "test cleanup"}); err != nil {
		t.Fatal(err)
	}
}

// parkedSegment ends run's segment as Runtime would park it: waiting, with the
// interrupts it raised, or suspended when it raised none.
func parkedSegment(run conversation.Run, interrupts ...conversation.Interrupt) conversation.SegmentFinished {
	run = run.Clone()
	run.Status, run.ActiveSegmentID = protocol.RunStatusWaiting, ""
	return conversation.SegmentFinished{Run: run, Interrupts: interrupts}
}

func finishedSegment(run conversation.Run, outcome conversation.Outcome) conversation.SegmentFinished {
	run = run.Clone()
	run.Status, run.ActiveSegmentID, run.Outcome = protocol.RunStatusFinished, "", outcome
	return conversation.SegmentFinished{Run: run}
}
